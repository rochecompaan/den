// Package tempdir manages Den's private per-launch temporary directories.
package tempdir

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const directoryMode = 0o700

// NewPair creates separate private policy and scratch directories below the
// invoking user's validated Den parent directory.
func NewPair(root string) (policyDir, scratchDir string, cleanup func() error, err error) {
	parent, err := parent(root, os.Getuid())
	if err != nil {
		return "", "", nil, err
	}
	policyDir, err = os.MkdirTemp(parent, "policy-")
	if err != nil {
		return "", "", nil, err
	}
	if err := os.Chmod(policyDir, directoryMode); err != nil {
		_ = os.RemoveAll(policyDir)
		return "", "", nil, err
	}
	scratchDir, err = os.MkdirTemp(parent, "scratch-")
	if err != nil {
		_ = os.RemoveAll(policyDir)
		return "", "", nil, err
	}
	if err := os.Chmod(scratchDir, directoryMode); err != nil {
		_ = os.RemoveAll(policyDir)
		_ = os.RemoveAll(scratchDir)
		return "", "", nil, err
	}
	return policyDir, scratchDir, func() error {
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

// RemoveStale removes old policy and scratch children belonging to uid. It
// only scans the validated per-user parent, never the sticky scratch root.
func RemoveStale(root string, uid int, olderThan time.Duration) error {
	parent, err := parent(root, uid)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(parent)
	if err != nil {
		return err
	}
	cutoff := time.Now().Add(-olderThan)
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "policy-") && !strings.HasPrefix(entry.Name(), "scratch-") {
			continue
		}
		path := filepath.Join(parent, entry.Name())
		info, err := os.Lstat(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if !privateOwnedDirectory(info, uid) || !info.ModTime().Before(cutoff) {
			continue
		}
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}
	return nil
}

func parent(root string, uid int) (string, error) {
	if err := validateRoot(root); err != nil {
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
