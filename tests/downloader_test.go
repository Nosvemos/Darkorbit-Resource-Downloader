package tests

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"darkorbit-resource-downloader/internal/downloader"
	"darkorbit-resource-downloader/internal/model"
)

func TestFetchToFilePersistsXMLAndPHPResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manifest.xml":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte("<filecollection></filecollection>"))
		case "/endpoint.php":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("<?php echo 'ok';"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := downloader.New()
	dir := t.TempDir()

	xmlDest := filepath.Join(dir, "spacemap", "xml", "resources.xml")
	if err := client.FetchToFile(context.Background(), server.URL+"/manifest.xml", xmlDest); err != nil {
		t.Fatal(err)
	}
	xmlData, err := os.ReadFile(xmlDest)
	if err != nil {
		t.Fatal(err)
	}
	if string(xmlData) != "<filecollection></filecollection>" {
		t.Fatalf("unexpected xml content: %q", string(xmlData))
	}

	phpDest := filepath.Join(dir, "unityApi", "events", "spaceSearch.php")
	if err := client.FetchToFile(context.Background(), server.URL+"/endpoint.php", phpDest); err != nil {
		t.Fatal(err)
	}
	phpData, err := os.ReadFile(phpDest)
	if err != nil {
		t.Fatal(err)
	}
	if string(phpData) != "<?php echo 'ok';" {
		t.Fatalf("unexpected php content: %q", string(phpData))
	}
}

func TestFetchToFileRetries503AndSucceeds(t *testing.T) {
	var attempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := attempts.Add(1)
		if current < 3 {
			w.Header().Set("Retry-After", "1")
			http.Error(w, "busy", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	client := downloader.New()
	dest := filepath.Join(t.TempDir(), "retry.txt")

	if err := client.FetchToFile(context.Background(), server.URL+"/retry", dest); err != nil {
		t.Fatal(err)
	}
	if attempts.Load() != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts.Load())
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "ok" {
		t.Fatalf("unexpected downloaded content: %q", string(data))
	}
}

func TestDownloadAllPacesConcurrentRequests(t *testing.T) {
	var lastStart atomic.Int64

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		now := time.Now().UnixNano()
		prev := lastStart.Swap(now)
		if prev != 0 && time.Duration(now-prev) < 45*time.Millisecond {
			http.Error(w, "too fast", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	client := downloader.NewWithConfig(downloader.Config{
		Timeout:         10 * time.Second,
		RequestInterval: 60 * time.Millisecond,
		MaxCooldown:     2 * time.Second,
	})

	dir := t.TempDir()
	items := []model.DownloadItem{
		{Resource: model.Resource{RelativePath: "one.bin", URL: server.URL + "/one"}},
		{Resource: model.Resource{RelativePath: "two.bin", URL: server.URL + "/two"}},
		{Resource: model.Resource{RelativePath: "three.bin", URL: server.URL + "/three"}},
	}

	for result := range client.DownloadAll(context.Background(), dir, items, 3) {
		if result.Err != nil {
			t.Fatalf("unexpected download error for %s: %v", result.Item.Resource.RelativePath, result.Err)
		}
	}

	for _, item := range items {
		if _, err := os.Stat(filepath.Join(dir, item.Resource.RelativePath)); err != nil {
			t.Fatalf("expected %s to be written: %v", item.Resource.RelativePath, err)
		}
	}
}

func TestDownloadAllAutoTunesConcurrencyAfter503(t *testing.T) {
	var inFlight atomic.Int32
	var maxObserved atomic.Int32
	var throttles atomic.Int32
	var mu sync.Mutex
	attemptsByPath := map[string]int{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := inFlight.Add(1)
		defer inFlight.Add(-1)

		for {
			previous := maxObserved.Load()
			if current <= previous || maxObserved.CompareAndSwap(previous, current) {
				break
			}
		}

		mu.Lock()
		attemptsByPath[r.URL.Path]++
		attempt := attemptsByPath[r.URL.Path]
		mu.Unlock()

		time.Sleep(40 * time.Millisecond)
		if current > 2 && attempt <= 2 {
			throttles.Add(1)
			http.Error(w, "busy", http.StatusServiceUnavailable)
			return
		}

		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	client := downloader.NewWithConfig(downloader.Config{
		Timeout:             10 * time.Second,
		RequestInterval:     0,
		MaxCooldown:         2 * time.Second,
		MaxConcurrency:      4,
		MinConcurrency:      1,
		AutoTuneConcurrency: true,
	})

	dir := t.TempDir()
	items := []model.DownloadItem{
		{Resource: model.Resource{RelativePath: "one.bin", URL: server.URL + "/one"}},
		{Resource: model.Resource{RelativePath: "two.bin", URL: server.URL + "/two"}},
		{Resource: model.Resource{RelativePath: "three.bin", URL: server.URL + "/three"}},
		{Resource: model.Resource{RelativePath: "four.bin", URL: server.URL + "/four"}},
		{Resource: model.Resource{RelativePath: "five.bin", URL: server.URL + "/five"}},
		{Resource: model.Resource{RelativePath: "six.bin", URL: server.URL + "/six"}},
	}

	for result := range client.DownloadAll(context.Background(), dir, items, 4) {
		if result.Err != nil {
			t.Fatalf("unexpected download error for %s: %v", result.Item.Resource.RelativePath, result.Err)
		}
	}

	if throttles.Load() == 0 {
		t.Fatal("expected the test server to issue at least one throttle response")
	}
	if maxObserved.Load() < 3 {
		t.Fatalf("expected initial concurrency pressure above 2, got %d", maxObserved.Load())
	}

	for _, item := range items {
		if _, err := os.Stat(filepath.Join(dir, item.Resource.RelativePath)); err != nil {
			t.Fatalf("expected %s to be written: %v", item.Resource.RelativePath, err)
		}
	}
}
