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
	root := t.TempDir()
	policyDir, scratchDir, cleanup, err := newPair(root, os.Getuid(), func(string) error { return nil })
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
	root := t.TempDir()
	parent := filepath.Join(root, "den-"+itoa(os.Getuid()))
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}

	old, err := os.MkdirTemp(parent, "scratch-")
	if err != nil {
		t.Fatal(err)
	}
	lease, err := acquireLease(old)
	if err != nil {
		t.Fatal(err)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	recent, err := os.MkdirTemp(parent, "policy-")
	if err != nil {
		t.Fatal(err)
	}
	then := time.Now().Add(-25 * time.Hour)
	if err := os.Chtimes(old, then, then); err != nil {
		t.Fatal(err)
	}
	if err := removeStale(root, os.Getuid(), 24*time.Hour, time.Now, func(string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(old); !os.IsNotExist(err) {
		t.Fatalf("old directory remains: %v", err)
	}
	if _, err := os.Lstat(recent); err != nil {
		t.Fatalf("recent directory was removed: %v", err)
	}
}

func TestRemoveStalePreservesLockedLeaseThenRemovesReleasedLease(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "den-"+itoa(os.Getuid()))
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(parent, "scratch-old")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	lease, err := acquireLease(directory)
	if err != nil {
		t.Fatal(err)
	}
	then := time.Now().Add(-25 * time.Hour)
	if err := os.Chtimes(directory, then, then); err != nil {
		t.Fatal(err)
	}
	trusted := func(string) error { return nil }
	if err := removeStale(root, os.Getuid(), 24*time.Hour, time.Now, trusted); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(directory); err != nil {
		t.Fatalf("locked directory removed: %v", err)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	if err := removeStale(root, os.Getuid(), 24*time.Hour, time.Now, trusted); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(directory); !os.IsNotExist(err) {
		t.Fatalf("released stale directory remains: %v", err)
	}
}

func TestRemoveStaleReclaimsSimulatedSIGKILLOnlyAfterThreshold(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "den-"+itoa(os.Getuid()))
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(parent, "policy-killed")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	lease, err := acquireLease(directory)
	if err != nil {
		t.Fatal(err)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	} // SIGKILL releases flock but leaves the lease file.
	started := time.Now()
	if err := os.Chtimes(directory, started, started); err != nil {
		t.Fatal(err)
	}
	trusted := func(string) error { return nil }
	if err := removeStale(root, os.Getuid(), 24*time.Hour, func() time.Time { return started.Add(23 * time.Hour) }, trusted); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(directory); err != nil {
		t.Fatalf("recent SIGKILL residue removed: %v", err)
	}
	if err := removeStale(root, os.Getuid(), 24*time.Hour, func() time.Time { return started.Add(25 * time.Hour) }, trusted); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(directory); !os.IsNotExist(err) {
		t.Fatalf("old SIGKILL residue remains: %v", err)
	}
}

func TestPrivateOwnedDirectoryRejectsForeignOwner(t *testing.T) {
	info := temporaryFileInfo{mode: os.ModeDir | 0o700, stat: &syscall.Stat_t{Uid: uint32(os.Getuid() + 1)}}
	if privateOwnedDirectory(info, os.Getuid()) {
		t.Fatal("foreign-owned directory was accepted")
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
