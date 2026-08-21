// Package tempdir manages Den's private per-launch temporary directories.
package tempdir

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

const directoryMode = 0o700

type rootValidator func(string) error

// NewPair creates separate private policy and scratch directories below the
// invoking user's validated Den parent directory.
func NewPair(root string) (policyDir, scratchDir string, cleanup func() error, err error) {
	return newPair(root, os.Getuid(), validateRoot)
}

func newPair(root string, uid int, validate rootValidator) (policyDir, scratchDir string, cleanup func() error, err error) {
	parent, err := parent(root, uid, validate)
	if err != nil {
		return "", "", nil, err
	}
	policyDir, err = os.MkdirTemp(parent, "policy-")
	if err != nil {
		return "", "", nil, err
	}
	policyLease, err := acquireLease(policyDir)
	if err != nil {
		_ = os.RemoveAll(policyDir)
		return "", "", nil, err
	}
	scratchDir, err = os.MkdirTemp(parent, "scratch-")
	if err != nil {
		_ = policyLease.Close()
		_ = os.RemoveAll(policyDir)
		return "", "", nil, err
	}
	scratchLease, err := acquireLease(scratchDir)
	if err != nil {
		_ = policyLease.Close()
		_ = os.RemoveAll(policyDir)
		_ = os.RemoveAll(scratchDir)
		return "", "", nil, err
	}
	return policyDir, scratchDir, func() error {
		_ = policyLease.Close()
		_ = scratchLease.Close()
		var result error
		if err := os.RemoveAll(policyDir); err != nil {
			result = err
		}
		if err := os.RemoveAll(scratchDir); err != nil && result == nil {
			result = err
		}
		return result
	}, nil
}

func parent(root string, uid int, validate rootValidator) (string, error) {
	if err := validate(root); err != nil {
		return "", err
	}
	path := filepath.Join(root, "den-"+strconv.Itoa(uid))
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		if err := os.Mkdir(path, directoryMode); err != nil && !os.IsExist(err) {
			return "", err
		}
		info, err = os.Lstat(path)
	}
	if err != nil || !privateOwnedDirectory(info, uid) {
		return "", errors.New("Den temporary directory is unsafe")
	}
	return path, nil
}

func validateRoot(root string) error {
	clean := filepath.Clean(root)
	resolved, err := filepath.EvalSymlinks(clean)
	if err != nil || resolved != clean {
		return errors.New("Den scratch root is unsafe")
	}
	info, err := os.Lstat(clean)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeSticky == 0 {
		return errors.New("Den scratch root is unsafe")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != 0 {
		return errors.New("Den scratch root is unsafe")
	}
	return nil
}

func privateOwnedDirectory(info fs.FileInfo, uid int) bool {
	if info == nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != directoryMode {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(stat.Uid) == uid
}
