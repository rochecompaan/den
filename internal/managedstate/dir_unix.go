//go:build unix

package managedstate

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

type anchoredDir struct {
	fd  int
	uid uint32
}

type leafOperations struct {
	renameat func(int, string, int, string) error
	unlinkat func(int, string, int) error
}

var systemLeafOperations = leafOperations{
	renameat: unix.Renameat,
	unlinkat: unix.Unlinkat,
}

func openRoot(root Root, uid uint32) (*anchoredDir, error) {
	fd, err := unix.Open(root.Path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open state root: %w", err)
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		unix.Close(fd)
		return nil, err
	}
	if uint64(stat.Dev) != root.Device || stat.Ino != root.Inode {
		unix.Close(fd)
		return nil, errors.New("state root identity changed")
	}
	if err := validateDirectory(stat, uid); err != nil {
		unix.Close(fd)
		return nil, err
	}
	return &anchoredDir{fd: fd, uid: uid}, nil
}

func (d *anchoredDir) close() error {
	return unix.Close(d.fd)
}

func (d *anchoredDir) restore(destination, source string, deps dependencies) (mutated bool, err error) {
	components := strings.Split(destination, string(filepath.Separator))
	parentFD := d.fd
	children := make([]int, 0, len(components)-1)
	defer func() {
		for index := len(children) - 1; index >= 0; index-- {
			_ = unix.Close(children[index])
		}
	}()
	for _, component := range components[:len(components)-1] {
		childFD, changed, openErr := openOrCreateParent(parentFD, component, d.uid)
		if changed {
			mutated = true
		}
		if openErr != nil {
			return mutated, openErr
		}
		children = append(children, childFD)
		parentFD = childFD
	}
	changed, err := replaceLeaf(parentFD, components[len(components)-1], source, deps.tempName, deps.leafOperations)
	return mutated || changed, err
}

func openOrCreateParent(parentFD int, name string, uid uint32) (fd int, mutated bool, err error) {
	fd, err = unix.Openat(parentFD, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if errors.Is(err, unix.ENOENT) {
		if err = unix.Mkdirat(parentFD, name, 0o700); err == nil {
			mutated = true
		}
		if err != nil && !errors.Is(err, unix.EEXIST) {
			return -1, false, fmt.Errorf("create parent: %w", err)
		}
		fd, err = unix.Openat(parentFD, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	}
	if err != nil {
		if errors.Is(err, unix.ELOOP) || parentIsSymlink(parentFD, name) {
			return -1, mutated, errors.New("parent is a symbolic link")
		}
		if errors.Is(err, unix.ENOTDIR) {
			return -1, mutated, errors.New("parent is not a directory")
		}
		return -1, mutated, fmt.Errorf("open parent: %w", err)
	}
	changed, err := normalizeDirectory(fd, uid)
	if err != nil {
		unix.Close(fd)
		return -1, mutated, err
	}
	return fd, mutated || changed, nil
}

func parentIsSymlink(parentFD int, name string) bool {
	var stat unix.Stat_t
	return unix.Fstatat(parentFD, name, &stat, unix.AT_SYMLINK_NOFOLLOW) == nil && stat.Mode&unix.S_IFMT == unix.S_IFLNK
}

func normalizeDirectory(fd int, uid uint32) (bool, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return false, err
	}
	if err := validateDirectory(stat, uid); err != nil {
		return false, err
	}
	if stat.Mode&0o7777 == 0o700 {
		return false, nil
	}
	if err := unix.Fchmod(fd, 0o700); err != nil {
		return false, fmt.Errorf("parent permissions cannot be normalized: %w", err)
	}
	return true, nil
}

func validateDirectory(stat unix.Stat_t, uid uint32) error {
	if stat.Mode&unix.S_IFMT != unix.S_IFDIR {
		return errors.New("parent is not a directory")
	}
	if stat.Uid != uid {
		return errors.New("parent is not owned by the runtime user")
	}
	return nil
}

func replaceLeaf(parentFD int, leaf, source string, tempName func() (string, error), operations leafOperations) (bool, error) {
	var stat unix.Stat_t
	if err := unix.Fstatat(parentFD, leaf, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil && !errors.Is(err, unix.ENOENT) {
		return false, fmt.Errorf("inspect managed leaf: %w", err)
	} else if err == nil {
		switch stat.Mode & unix.S_IFMT {
		case unix.S_IFREG, unix.S_IFLNK:
		case unix.S_IFDIR:
			return false, errors.New("managed leaf is a directory")
		default:
			return false, errors.New("managed leaf is not a regular file or symbolic link")
		}
	}

	var temporary string
	for range 16 {
		name, err := tempName()
		if err != nil {
			return false, fmt.Errorf("generate temporary name: %w", err)
		}
		if !validTempName(name) {
			return false, errors.New("temporary name is invalid")
		}
		err = unix.Symlinkat(source, parentFD, name)
		if err == nil {
			temporary = name
			break
		}
		if !errors.Is(err, unix.EEXIST) {
			return false, fmt.Errorf("create temporary link: %w", err)
		}
	}
	if temporary == "" {
		return false, errors.New("temporary name collision limit reached")
	}
	if err := operations.renameat(parentFD, temporary, parentFD, leaf); err != nil {
		renameErr := fmt.Errorf("replace managed leaf: %w", err)
		if cleanupErr := operations.unlinkat(parentFD, temporary, 0); cleanupErr != nil {
			return true, errors.Join(renameErr, fmt.Errorf("cleanup temporary managed link: %w", cleanupErr))
		}
		return false, renameErr
	}
	return true, nil
}

func validTempName(name string) bool {
	return strings.HasPrefix(name, ".den-managed-") && filepath.Base(name) == name && name != ".den-managed-"
}
