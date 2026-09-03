//go:build linux

package tempdir

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveOpenedDirectoryContentsPreservesReplacementPath(t *testing.T) {
	root := t.TempDir()
	original := filepath.Join(root, "original")
	moved := filepath.Join(root, "moved")
	if err := os.Mkdir(original, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(original, "stale"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	directory, err := os.Open(original)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	if err := os.Rename(original, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(original, 0o700); err != nil {
		t.Fatal(err)
	}
	fresh := filepath.Join(original, "fresh")
	if err := os.WriteFile(fresh, []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}

	stablePath := openedDirectoryPath(directory)
	if err := removeOpenedDirectoryContents(directory, stablePath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(moved, "stale")); !os.IsNotExist(err) {
		t.Fatalf("stale content remains: %v", err)
	}
	contents, err := os.ReadFile(fresh)
	if err != nil || string(contents) != "preserve" {
		t.Fatalf("replacement changed: %q, %v", contents, err)
	}
}
