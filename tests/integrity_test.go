package tests

import (
	"os"
	"path/filepath"
	"testing"

	"darkorbit-resource-downloader/internal/integrity"
)

func TestNormalizedFileMD5AndMatch(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "sample.bin")
	if err := os.WriteFile(filePath, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	hash, err := integrity.NormalizedFileMD5(filePath)
	if err != nil {
		t.Fatal(err)
	}

	const expected = "5d41402abc4b2a76b9719d911017c500"
	if hash != expected {
		t.Fatalf("unexpected normalized hash: got %s want %s", hash, expected)
	}

	match, err := integrity.FileMatchesManifestHash(filePath, expected)
	if err != nil {
		t.Fatal(err)
	}
	if !match {
		t.Fatal("expected file to match manifest hash")
	}
}
