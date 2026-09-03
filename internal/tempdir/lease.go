package tempdir

import (
	"os"
	"path/filepath"
	"syscall"
)

const leaseName = ".den-lease"

func acquireLease(directory string) (*os.File, error) {
	lease, err := os.OpenFile(filepath.Join(directory, leaseName), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(lease.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = lease.Close()
		return nil, err
	}
	return lease, nil
}
