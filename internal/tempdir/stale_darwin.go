//go:build darwin

package tempdir

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
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
	snapshot, err := os.Lstat(path)
	if err != nil {
		return err
	}
	parentFD, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer unix.Close(parentFD)
	opened, err := descriptorInfo(parentFD)
	if err != nil {
		return err
	}
	current, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !privateOwnedDirectory(snapshot, uid) || !privateOwnedDirectory(opened, uid) || !privateOwnedDirectory(current, uid) ||
		!sameIdentity(snapshot, opened) || !sameIdentity(snapshot, current) {
		return errors.New("Den temporary directory is unsafe")
	}
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
	duplicate, err := unix.FcntlInt(uintptr(fd), unix.F_DUPFD_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(duplicate), "temporary directory")
	defer file.Close()
	return file.ReadDir(-1)
}

func removeIfStale(parentFD int, name string, uid int, cutoff time.Time) error {
	childFD, err := unix.Openat(parentFD, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		if unsafeOrGone(err) {
			return nil
		}
		return err
	}
	defer unix.Close(childFD)
	info, err := descriptorInfo(childFD)
	if err != nil {
		return err
	}
	if !privateOwnedDirectory(info, uid) || !info.ModTime().Before(cutoff) {
		return nil
	}
	leaseFD, err := unix.Openat(childFD, leaseName, unix.O_RDWR|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		if unsafeOrGone(err) {
			return nil
		}
		return err
	}
	defer unix.Close(leaseFD)
	if err := unix.Flock(leaseFD, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return nil
		}
		return err
	}

	var tombstone string
	for range 4 {
		tombstone, err = removalName()
		if err != nil {
			return err
		}
		err = unix.RenameatxNp(parentFD, name, parentFD, tombstone, unix.RENAME_EXCL)
		if err == nil {
			break
		}
		if !errors.Is(err, unix.EEXIST) {
			if unsafeOrGone(err) {
				return nil
			}
			return err
		}
	}
	if err != nil {
		return err
	}
	movedFD, err := unix.Openat(parentFD, tombstone, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		if unsafeOrGone(err) {
			return nil
		}
		return err
	}
	defer unix.Close(movedFD)
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
	duplicate, err := unix.FcntlInt(uintptr(fd), unix.F_DUPFD_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(duplicate), "temporary directory")
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

func unsafeOrGone(err error) bool {
	return errors.Is(err, unix.ENOENT) || errors.Is(err, unix.ELOOP) || errors.Is(err, unix.ENOTDIR)
}
