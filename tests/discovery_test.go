package tests

import (
	"context"
	"net/http"
	"net/http/httptest"
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

	if _, ok := seen["spacemap/templates/en/resource_chat.xml"]; ok {
		t.Fatal("did not expect localized template resource xml to be treated as a seed")
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

func TestAddLanguageBootstrapSeedsSupportsArbitraryLanguageCodes(t *testing.T) {
	seeds := discovery.AddLanguageBootstrapSeeds(t.TempDir(), nil, map[string]bool{"de": true, "au": true})

	seen := map[string]string{}
	for _, seed := range seeds {
		seen[seed.RelativePath] = seed.Category
	}

	if seen["spacemap/templates/language_de.xml"] != "templates" {
		t.Fatalf("expected language_de.xml bootstrap seed, got %q", seen["spacemap/templates/language_de.xml"])
	}
	if seen["spacemap/templates/language_au.xml"] != "templates" {
		t.Fatalf("expected language_au.xml bootstrap seed, got %q", seen["spacemap/templates/language_au.xml"])
	}
	if seen["do_img/de/xml/resource_localized.xml"] != "do_img" {
		t.Fatalf("expected do_img de bootstrap seed, got %q", seen["do_img/de/xml/resource_localized.xml"])
	}
	if seen["do_img/au/xml/resource_localized.xml"] != "do_img" {
		t.Fatalf("expected do_img au bootstrap seed, got %q", seen["do_img/au/xml/resource_localized.xml"])
	}
}

func TestAddLanguageBootstrapSeedsAllUsesDiscoveredLanguages(t *testing.T) {
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

	mustWrite("spacemap/templates/language_ja.xml")
	mustWrite("do_img/fr/xml/resource_localized.xml")

	seeds := discovery.AddLanguageBootstrapSeeds(root, nil, map[string]bool{"all": true})

	seen := map[string]string{}
	for _, seed := range seeds {
		seen[seed.RelativePath] = seed.Category
	}

	if seen["spacemap/templates/language_ja.xml"] != "templates" {
		t.Fatalf("expected discovered ja bootstrap seed, got %q", seen["spacemap/templates/language_ja.xml"])
	}
	if seen["spacemap/templates/language_fr.xml"] != "templates" {
		t.Fatalf("expected inferred fr template bootstrap seed, got %q", seen["spacemap/templates/language_fr.xml"])
	}
	if seen["do_img/ja/xml/resource_localized.xml"] != "do_img" {
		t.Fatalf("expected inferred ja do_img bootstrap seed, got %q", seen["do_img/ja/xml/resource_localized.xml"])
	}
	if seen["do_img/fr/xml/resource_localized.xml"] != "do_img" {
		t.Fatalf("expected discovered fr do_img bootstrap seed, got %q", seen["do_img/fr/xml/resource_localized.xml"])
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

func TestCanonicalLanguageCode(t *testing.T) {
	tests := map[string]string{
		"en":     "en",
		"PT_br":  "pt_BR",
		"es-ar":  "es_AR",
		"en_US":  "en_US",
		"ALL":    "all",
		"  tr  ": "tr",
	}

	for input, want := range tests {
		if got := discovery.CanonicalLanguageCode(input); got != want {
			t.Fatalf("CanonicalLanguageCode(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestDiscoverLiveLanguages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`
				<a href="index.es?lang=en">EN</a>
				<a href="index.es?lang=pt_BR">PTBR</a>
				<a href="index.es?lang=de">DE</a>
				<a href="index.es?lang=au">AU</a>
			`))
		case "/spacemap/templates/language_en.xml":
			_, _ = w.Write([]byte("<filecollection/>"))
		case "/spacemap/templates/language_pt_BR.xml":
			_, _ = w.Write([]byte("<filecollection/>"))
		case "/do_img/de/xml/resource_localized.xml":
			_, _ = w.Write([]byte("<filecollection/>"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	languages, err := discovery.DiscoverLiveLanguages(context.Background(), server.URL)
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]bool{}
	for _, lang := range languages {
		got[lang] = true
	}

	for _, want := range []string{"en", "pt_BR", "de"} {
		if !got[want] {
			t.Fatalf("expected %q to be discovered, got %v", want, languages)
		}
	}
	if got["au"] {
		t.Fatalf("did not expect unavailable locale au to be discovered, got %v", languages)
	}
}
