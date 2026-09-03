//go:build darwin

package tempdir

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestRemoveStaleDarwinRejectsLeaseSymlink(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "den-"+itoa(os.Getuid()))
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	candidate := filepath.Join(parent, "scratch-lease-symlink")
	if err := os.Mkdir(candidate, 0o700); err != nil {
		t.Fatal(err)
	}
	lease, err := acquireLease(candidate)
	if err != nil {
		t.Fatal(err)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(candidate, leaseName)); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "lease-target")
	if err := os.WriteFile(target, []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(candidate, leaseName)); err != nil {
		t.Fatal(err)
	}
	then := time.Now().Add(-25 * time.Hour)
	if err := os.Chtimes(candidate, then, then); err != nil {
		t.Fatal(err)
	}

	if err := removeStale(root, os.Getuid(), 24*time.Hour, time.Now, func(string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(candidate); err != nil {
		t.Fatalf("candidate removed: %v", err)
	}
}

func TestRemoveStaleDarwinPreservesTemporaryNameSymlinkTarget(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "den-"+itoa(os.Getuid()))
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	contents := filepath.Join(target, "contents")
	if err := os.WriteFile(contents, []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	candidate := filepath.Join(parent, "scratch-symlink")
	if err := os.Symlink(target, candidate); err != nil {
		t.Fatal(err)
	}

	if err := removeStale(root, os.Getuid(), 0, time.Now, func(string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(candidate)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("temporary-name symlink changed: %v, %v", info, err)
	}
	got, err := os.ReadFile(contents)
	if err != nil || string(got) != "preserve" {
		t.Fatalf("symlink target contents changed: %q, %v", got, err)
	}
}

func TestRemoveTreeAtDarwinPreservesReplacementAtTombstone(t *testing.T) {
	root := t.TempDir()
	parentFD, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(parentFD)

	tombstone := ".den-removing-0123456789abcdef01234567"
	original := filepath.Join(root, tombstone)
	if err := os.Mkdir(original, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(original, "stale"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	childFD, err := unix.Openat(parentFD, tombstone, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(childFD)
	if err := os.Rename(original, filepath.Join(root, "moved")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(original, 0o700); err != nil {
		t.Fatal(err)
	}
	fresh := filepath.Join(original, "fresh")
	if err := os.WriteFile(fresh, []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := removeTreeAt(parentFD, tombstone, childFD); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(root, "moved", "stale")); !os.IsNotExist(err) {
		t.Fatalf("opened directory content remains: %v", err)
	}
	got, err := os.ReadFile(fresh)
	if err != nil || string(got) != "preserve" {
		t.Fatalf("replacement changed: %q, %v", got, err)
	}
}
