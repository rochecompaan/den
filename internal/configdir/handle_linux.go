//go:build linux

package configdir

import (
	"fmt"
	"os"
	"syscall"
)

func openDirectoryHandle(path string) (*os.File, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}

func localDirectoryHandlePath(file *os.File) string {
	return fmt.Sprintf("/proc/self/fd/%d", file.Fd())
}
