//go:build darwin

package tempdir

import (
	"errors"

	"golang.org/x/sys/unix"
)

// removeTreeAt removes a checked child through parentFD, never by rebuilding a
// pathname from the ambient working directory.
func removeTreeAt(parentFD int, name string, childFD int) error {
	if err := removeContents(childFD); err != nil {
		return err
	}
	return removeDirectoryAt(parentFD, name, childFD)
}

func removeContents(directoryFD int) error {
	entries, err := readDirectory(directoryFD)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		childFD, err := unix.Openat(directoryFD, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err == nil {
			err = removeTreeAt(directoryFD, name, childFD)
			closeErr := unix.Close(childFD)
			if err != nil {
				return err
			}
			if closeErr != nil {
				return closeErr
			}
			continue
		}
		if errors.Is(err, unix.ENOENT) {
			continue
		}
		if !errors.Is(err, unix.ENOTDIR) && !errors.Is(err, unix.ELOOP) {
			return err
		}
		if err := unix.Unlinkat(directoryFD, name, 0); err != nil && !errors.Is(err, unix.ENOENT) {
			return err
		}
	}
	return nil
}

func removeDirectoryAt(parentFD int, name string, childFD int) error {
	var opened unix.Stat_t
	if err := unix.Fstat(childFD, &opened); err != nil {
		return err
	}
	var named unix.Stat_t
	if err := unix.Fstatat(parentFD, name, &named, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		if errors.Is(err, unix.ENOENT) {
			return nil
		}
		return err
	}
	if named.Dev != opened.Dev || named.Ino != opened.Ino {
		return nil
	}
	if err := unix.Unlinkat(parentFD, name, unix.AT_REMOVEDIR); err != nil && !errors.Is(err, unix.ENOENT) {
		return err
	}
	return nil
}
