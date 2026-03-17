package tests

import (
	"os"
	"path/filepath"
	"testing"

	"darkorbit-resource-downloader/internal/discovery"
)

func TestDiscoverSeedsFindsExpectedFiles(t *testing.T) {
	root := t.TempDir()

	mustWrite := func(rel string) {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	mustWrite("spacemap/templates/en/resource_chat.xml")
	mustWrite("unityApi/events/spaceSearch.php")
	mustWrite("flashAPI/brNews.php")
	mustWrite("resources/resource_command_center.xml")
	mustWrite("spacemap_decompiled/ignore.xml")
	mustWrite("index.php")

	seeds, err := discovery.DiscoverSeeds(root)
	if err != nil {
		t.Fatal(err)
	}

	seen := map[string]string{}
	for _, seed := range seeds {
		seen[seed.RelativePath] = seed.Category
	}

	if seen["spacemap/templates/en/resource_chat.xml"] != "templates" {
		t.Fatalf("expected template seed, got %q", seen["spacemap/templates/en/resource_chat.xml"])
	}
	if seen["unityApi/events/spaceSearch.php"] != "unityApi" {
		t.Fatalf("expected unityApi seed, got %q", seen["unityApi/events/spaceSearch.php"])
	}
	if seen["flashAPI/brNews.php"] != "flashAPI" {
		t.Fatalf("expected flashAPI seed, got %q", seen["flashAPI/brNews.php"])
	}
	if seen["resources/resource_command_center.xml"] != "resources" {
		t.Fatalf("expected resources seed, got %q", seen["resources/resource_command_center.xml"])
	}
	if _, ok := seen["spacemap_decompiled/ignore.xml"]; ok {
		t.Fatal("did not expect decompiled files to be discovered as seeds")
	}
	if _, ok := seen["index.php"]; ok {
		t.Fatal("did not expect index.php to be discovered as a seed")
	}
}

func TestMatchesLanguagePath(t *testing.T) {
	languages := map[string]bool{"en": true}

	tests := []struct {
		path string
		want bool
	}{
		{path: "spacemap/templates/language_en.xml", want: true},
		{path: "spacemap/templates/language_tr.xml", want: false},
		{path: "spacemap/templates/en/resource_chat.xml", want: true},
		{path: "spacemap/templates/es/resource_chat.xml", want: false},
		{path: "do_img/en/xml/resource_localized.xml", want: true},
		{path: "do_img/es/xml/resource_localized.xml", want: false},
		{path: "do_img/global/xml/resource_items.xml", want: true},
		{path: "spacemap/xml/resources.xml", want: true},
	}

	for _, tt := range tests {
		if got := discovery.MatchesLanguagePath(tt.path, languages); got != tt.want {
			t.Fatalf("MatchesLanguagePath(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestMatchesLanguagePathAllowsAllLanguages(t *testing.T) {
	languages := map[string]bool{"all": true}

	if !discovery.MatchesLanguagePath("spacemap/templates/tr/resource_chat.xml", languages) {
		t.Fatal("expected all-language mode to allow template files")
	}
	if !discovery.MatchesLanguagePath("do_img/es/xml/resource_localized.xml", languages) {
		t.Fatal("expected all-language mode to allow do_img localized files")
	}
}
