//go:build darwin

package configdir

import (
	"crypto/rand"
	"encoding/hex"
	"os"

	"golang.org/x/sys/unix"
)

func directoryHandleWritable(file *os.File) bool {
	for range 4 {
		bytes := make([]byte, 12)
		if _, err := rand.Read(bytes); err != nil {
			return false
		}
		name := ".den-write-probe-" + hex.EncodeToString(bytes)
		fd, err := unix.Openat(int(file.Fd()), name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
		if err == unix.EEXIST {
			continue
		}
		if err != nil {
			return false
		}
		unlinkErr := unix.Unlinkat(int(file.Fd()), name, 0)
		closeErr := unix.Close(fd)
		return unlinkErr == nil && closeErr == nil
	}
	return false
}
