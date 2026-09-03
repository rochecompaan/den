//go:build linux

package tempdir

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"syscall"
	"time"
)

// RemoveStale removes old, unlocked policy and scratch children belonging to
// uid. It only scans the validated per-user parent, never the sticky root.
func RemoveStale(root string, uid int, olderThan time.Duration) error {
	return removeStale(root, uid, olderThan, time.Now, validateRoot)
}

func removeStale(root string, uid int, olderThan time.Duration, now func() time.Time, validate rootValidator) error {
	path, err := parent(root, uid, validate)
	if err != nil {
		return err
	}
	parentFD, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer syscall.Close(parentFD)
	entries, err := readDirectory(parentFD)
	if err != nil {
		return err
	}
	cutoff := now().Add(-olderThan)
	for _, entry := range entries {
		if !temporaryName(entry.Name()) {
			continue
		}
		if err := removeIfStale(parentFD, entry.Name(), uid, cutoff); err != nil {
			return err
		}
	}
	return nil
}

func readDirectory(fd int) ([]os.DirEntry, error) {
	duplicate, err := syscall.Dup(fd)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(duplicate), "temporary parent")
	defer file.Close()
	return file.ReadDir(-1)
}

func removeIfStale(parentFD int, name string, uid int, cutoff time.Time) error {
	childFD, err := syscall.Openat(parentFD, name, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		if errors.Is(err, syscall.ENOENT) || errors.Is(err, syscall.ELOOP) || errors.Is(err, syscall.ENOTDIR) {
			return nil
		}
		return err
	}
	defer syscall.Close(childFD)
	info, err := descriptorInfo(childFD)
	if err != nil {
		return err
	}
	if !privateOwnedDirectory(info, uid) || !info.ModTime().Before(cutoff) {
		return nil
	}
	leaseFD, err := syscall.Openat(childFD, leaseName, syscall.O_RDWR|syscall.O_NOFOLLOW, 0)
	if err != nil {
		if errors.Is(err, syscall.ENOENT) || errors.Is(err, syscall.ELOOP) {
			return nil
		}
		return err
	}
	defer syscall.Close(leaseFD)
	if err := syscall.Flock(leaseFD, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil
		}
		return err
	}
	tombstone, err := removalName()
	if err != nil {
		return err
	}
	if err := syscall.Renameat(parentFD, name, parentFD, tombstone); err != nil {
		if errors.Is(err, syscall.ENOENT) {
			return nil
		}
		return err
	}
	movedFD, err := syscall.Openat(parentFD, tombstone, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer syscall.Close(movedFD)
	moved, err := descriptorInfo(movedFD)
	if err != nil {
		return err
	}
	if !sameIdentity(info, moved) {
		return nil
	}
	return removeTreeAt(parentFD, tombstone, movedFD)
}

func descriptorInfo(fd int) (os.FileInfo, error) {
	duplicate, err := syscall.Dup(fd)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(duplicate), "temporary child")
	defer file.Close()
	return file.Stat()
}

func removalName() (string, error) {
	bytes := make([]byte, 12)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return removalPrefix + hex.EncodeToString(bytes), nil
}

func sameIdentity(left, right os.FileInfo) bool {
	leftStat, leftOK := left.Sys().(*syscall.Stat_t)
	rightStat, rightOK := right.Sys().(*syscall.Stat_t)
	return leftOK && rightOK && leftStat.Dev == rightStat.Dev && leftStat.Ino == rightStat.Ino
}
