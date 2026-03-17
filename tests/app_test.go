package tests

import (
	"os"
	"path/filepath"
	"testing"

	"darkorbit-resource-downloader/internal/app"
	"darkorbit-resource-downloader/internal/integrity"
	"darkorbit-resource-downloader/internal/model"
)

func TestBuildDownloadPlan(t *testing.T) {
	dir := t.TempDir()

	matchPath := filepath.Join(dir, "spacemap", "graphics", "match.swf")
	mismatchPath := filepath.Join(dir, "spacemap", "graphics", "mismatch.swf")
	if err := os.MkdirAll(filepath.Dir(matchPath), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(matchPath, []byte("expected"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mismatchPath, []byte("old-data"), 0o644); err != nil {
		t.Fatal(err)
	}

	matchHash, err := integrity.NormalizedFileMD5(matchPath)
	if err != nil {
		t.Fatal(err)
	}

	expectedMismatchFile := filepath.Join(dir, "expected-mismatch.bin")
	if err := os.WriteFile(expectedMismatchFile, []byte("new-data"), 0o644); err != nil {
		t.Fatal(err)
	}
	mismatchHash, err := integrity.NormalizedFileMD5(expectedMismatchFile)
	if err != nil {
		t.Fatal(err)
	}

	resources := []model.Resource{
		{RelativePath: "spacemap/graphics/missing.swf", URL: "https://example.com/missing.swf", Hash: matchHash, Category: "spacemap"},
		{RelativePath: "spacemap/graphics/match.swf", URL: "https://example.com/match.swf", Hash: matchHash, Category: "spacemap"},
		{RelativePath: "spacemap/graphics/mismatch.swf", URL: "https://example.com/mismatch.swf", Hash: mismatchHash, Category: "spacemap"},
	}

	plan, skipped, _, err := app.BuildDownloadPlan(dir, resources, false)
	if err != nil {
		t.Fatal(err)
	}

	if skipped != 1 {
		t.Fatalf("expected 1 skipped resource, got %d", skipped)
	}
	if len(plan) != 2 {
		t.Fatalf("expected 2 planned downloads, got %d", len(plan))
	}

	reasons := map[string]string{}
	for _, item := range plan {
		reasons[item.Resource.RelativePath] = item.Reason
	}

	if reasons["spacemap/graphics/missing.swf"] != "missing" {
		t.Fatalf("expected missing.swf to be marked missing, got %q", reasons["spacemap/graphics/missing.swf"])
	}
	if reasons["spacemap/graphics/mismatch.swf"] != "hash-mismatch" {
		t.Fatalf("expected mismatch.swf to be marked hash-mismatch, got %q", reasons["spacemap/graphics/mismatch.swf"])
	}
}
