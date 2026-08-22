package launch

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

var errCAPreparation = errors.New("RepoWolf CA preparation failed")

func prepareCAFile(source, policyDir string) (string, error) {
	fd, err := syscall.Open(source, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC|syscall.O_NONBLOCK, 0)
	if err != nil {
		return "", errCAPreparation
	}
	input := os.NewFile(uintptr(fd), "repowolf-ca-input")
	if input == nil {
		_ = syscall.Close(fd)
		return "", errCAPreparation
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil || info == nil || !info.Mode().IsRegular() {
		return "", errCAPreparation
	}

	destination := filepath.Join(policyDir, "repowolf-ca.pem")
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o400)
	if err != nil {
		return "", errCAPreparation
	}
	if err := output.Chmod(0o400); err != nil {
		_ = output.Close()
		_ = os.Remove(destination)
		return "", errCAPreparation
	}
	ok := false
	defer func() {
		_ = output.Close()
		if !ok {
			_ = os.Remove(destination)
		}
	}()
	if _, err := io.Copy(output, input); err != nil {
		return "", errCAPreparation
	}
	if err := output.Sync(); err != nil {
		return "", errCAPreparation
	}
	if err := output.Close(); err != nil {
		return "", errCAPreparation
	}
	ok = true
	return destination, nil
}
