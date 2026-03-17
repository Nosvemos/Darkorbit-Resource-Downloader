package tests

import (
	"os"
	"path/filepath"
	"testing"

	"darkorbit-resource-downloader/internal/manifest"
)

func TestLoadResourcesFromFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "resources.xml")
	xml := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<filecollection>
  <location id="graphics" path="graphics/" />
  <file id="demo" location="graphics" name="portal" type="swf" hash="abc12300" version="1" />
</filecollection>`
	if err := os.WriteFile(filePath, []byte(xml), 0o644); err != nil {
		t.Fatal(err)
	}

	resources, ok, err := manifest.LoadResourcesFromFile(filePath, "spacemap/xml/resources.xml", "https://example.com", "spacemap")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected filecollection xml to be parsed")
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(resources))
	}

	resource := resources[0]
	if resource.RelativePath != "spacemap/graphics/portal.swf" {
		t.Fatalf("unexpected relative path: %s", resource.RelativePath)
	}
	if resource.URL != "https://example.com/spacemap/graphics/portal.swf?__cv=abc12300" {
		t.Fatalf("unexpected url: %s", resource.URL)
	}
}

func TestLoadResourcesFromLanguageFileCollectionWithoutLocationNodes(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "language_en.xml")
	xml := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<filecollection>
  <file id="resource_chat" location="en" name="resource_chat" type="xml" hash="abc12300" version="1" />
</filecollection>`
	if err := os.WriteFile(filePath, []byte(xml), 0o644); err != nil {
		t.Fatal(err)
	}

	resources, ok, err := manifest.LoadResourcesFromFile(filePath, "spacemap/templates/language_en.xml", "https://example.com", "templates")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected language filecollection xml to be parsed")
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(resources))
	}

	resource := resources[0]
	if resource.RelativePath != "spacemap/templates/en/resource_chat.xml" {
		t.Fatalf("unexpected relative path: %s", resource.RelativePath)
	}
	if resource.URL != "https://example.com/spacemap/templates/en/resource_chat.xml?__cv=abc12300" {
		t.Fatalf("unexpected url: %s", resource.URL)
	}
}
