//go:build linux

package tempdir

import (
	"errors"
	"os"
	"syscall"
	"unsafe"
)

const atRemovedir = 0x200

// removeTreeAt removes a checked child through parentFD, never by rebuilding a
// pathname from the ambient working directory.
func removeTreeAt(parentFD int, name string, childFD int) error {
	if err := removeContents(childFD); err != nil {
		return err
	}
	return removeDirectoryAt(parentFD, name)
}

func removeContents(directoryFD int) error {
	entries, err := readDirectory(directoryFD)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		childFD, err := syscall.Openat(directoryFD, name, syscall.O_RDONLY|syscall.O_NOFOLLOW, 0)
		if err == nil {
			info, statErr := descriptorInfo(childFD)
			syscall.Close(childFD)
			if statErr != nil {
				return statErr
			}
			if info.IsDir() {
				childDirectoryFD, err := syscall.Openat(directoryFD, name, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW, 0)
				if err != nil {
					return err
				}
				if err := removeTreeAt(directoryFD, name, childDirectoryFD); err != nil {
					syscall.Close(childDirectoryFD)
					return err
				}
				syscall.Close(childDirectoryFD)
				continue
			}
		}
		if err := syscall.Unlinkat(directoryFD, name); err != nil {
			if errors.Is(err, syscall.ENOENT) {
				continue
			}
			return err
		}
	}
	return nil
}

func removeDirectoryAt(parentFD int, name string) error {
	path, err := syscall.BytePtrFromString(name)
	if err != nil {
		return err
	}
	_, _, errno := syscall.Syscall6(syscall.SYS_UNLINKAT, uintptr(parentFD), uintptr(unsafe.Pointer(path)), atRemovedir, 0, 0, 0)
	if errno != 0 {
		return os.NewSyscallError("unlinkat", errno)
	}
	return nil
}
