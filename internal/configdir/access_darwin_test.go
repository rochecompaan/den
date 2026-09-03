//go:build darwin

package configdir

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDirectoryHandleWritableUsesOpenedDirectoryAfterPathReplacement(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses ordinary directory write permissions")
	}

	root := t.TempDir()
	original := filepath.Join(root, "original")
	moved := filepath.Join(root, "moved")
	if err := os.Mkdir(original, 0o700); err != nil {
		t.Fatal(err)
	}
	file, err := openDirectoryHandle(original)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	if err := os.Rename(original, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(original, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(original, 0o500); err != nil {
		t.Fatal(err)
	}
	if probe, err := os.CreateTemp(original, ".den-write-probe-*"); err == nil {
		_ = probe.Close()
		_ = os.Remove(probe.Name())
		t.Fatal("pathname-based probe unexpectedly created a file in the replacement directory")
	}
	if !directoryHandleWritable(file) {
		t.Fatal("directoryHandleWritable() = false for the opened directory")
	}

	for _, path := range []string{original, moved} {
		entries, err := os.ReadDir(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".den-write-probe-") {
				t.Fatalf("directoryHandleWritable() left artifact %q in %q", entry.Name(), path)
			}
		}
	}
}
