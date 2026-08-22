//go:build !linux

package tempdir

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// RemoveStale removes only old, unlocked Den temporary directories. Linux uses
// descriptor-relative deletion; this fallback retains lease and identity checks
// for platforms whose syscall package does not expose openat.
func RemoveStale(root string, uid int, olderThan time.Duration) error {
	return removeStale(root, uid, olderThan, time.Now, validateRoot)
}

func removeStale(root string, uid int, olderThan time.Duration, now func() time.Time, validate rootValidator) error {
	parentPath, err := parent(root, uid, validate)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(parentPath)
	if err != nil {
		return err
	}
	cutoff := now().Add(-olderThan)
	for _, entry := range entries {
		if !temporaryName(entry.Name()) {
			continue
		}
		path := filepath.Join(parentPath, entry.Name())
		info, err := os.Lstat(path)
		if err != nil || !privateOwnedDirectory(info, uid) || !info.ModTime().Before(cutoff) {
			continue
		}
		lease, err := os.OpenFile(filepath.Join(path, leaseName), os.O_RDWR, 0)
		if err != nil {
			continue
		}
		if err := syscall.Flock(int(lease.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
			_ = lease.Close()
			if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
				continue
			}
			return err
		}
		current, err := os.Lstat(path)
		if err == nil && os.SameFile(info, current) {
			err = os.RemoveAll(path)
		}
		_ = lease.Close()
		if err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
