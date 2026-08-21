package tempdir

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestNewPairCreatesPrivateSeparateDirectoriesAndCleansThem(t *testing.T) {
	requireTrustedScratchRoot(t)
	policyDir, scratchDir, cleanup, err := NewPair("/tmp")
	if err != nil {
		t.Fatal(err)
	}
	if policyDir == scratchDir {
		t.Fatal("policy and scratch directories must differ")
	}
	for _, path := range []string{policyDir, scratchDir} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o700 {
			t.Fatalf("%s mode = %o, want 0700", path, info.Mode().Perm())
		}
	}
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{policyDir, scratchDir} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("temporary directory remains: %s: %v", path, err)
		}
	}
}

func TestRemoveStaleRemovesOnlyOldOwnedTemporaryDirectories(t *testing.T) {
	requireTrustedScratchRoot(t)
	root := filepath.Join("/tmp", "den-"+itoa(os.Getuid()))
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })

	old, err := os.MkdirTemp(root, "scratch-")
	if err != nil {
		t.Fatal(err)
	}
	recent, err := os.MkdirTemp(root, "policy-")
	if err != nil {
		t.Fatal(err)
	}
	then := time.Now().Add(-25 * time.Hour)
	if err := os.Chtimes(old, then, then); err != nil {
		t.Fatal(err)
	}
	if err := RemoveStale("/tmp", os.Getuid(), 24*time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(old); !os.IsNotExist(err) {
		t.Fatalf("old directory remains: %v", err)
	}
	if _, err := os.Lstat(recent); err != nil {
		t.Fatalf("recent directory was removed: %v", err)
	}
}

func TestPrivateOwnedDirectoryRejectsForeignOwner(t *testing.T) {
	info := temporaryFileInfo{mode: os.ModeDir | 0o700, stat: &syscall.Stat_t{Uid: uint32(os.Getuid() + 1)}}
	if privateOwnedDirectory(info, os.Getuid()) {
		t.Fatal("foreign-owned directory was accepted")
	}
}

func requireTrustedScratchRoot(t *testing.T) {
	t.Helper()
	if err := validateRoot("/tmp"); err != nil {
		t.Skipf("sandbox does not provide a trusted /tmp: %v", err)
	}
}

func itoa(value int) string { return fmt.Sprintf("%d", value) }

type temporaryFileInfo struct {
	mode os.FileMode
	stat *syscall.Stat_t
}

func (info temporaryFileInfo) Name() string       { return "temporary" }
func (info temporaryFileInfo) Size() int64        { return 0 }
func (info temporaryFileInfo) Mode() os.FileMode  { return info.mode }
func (info temporaryFileInfo) ModTime() time.Time { return time.Time{} }
func (info temporaryFileInfo) IsDir() bool        { return info.mode.IsDir() }
func (info temporaryFileInfo) Sys() any           { return info.stat }
