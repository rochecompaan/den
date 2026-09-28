// Package managedstate restores immutable managed links into an agent state directory.
package managedstate

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rochecompaan/den/internal/manifest"
)

// Root identifies the already validated state directory.
type Root struct {
	Path   string
	Device uint64
	Inode  uint64
}

// Result describes whether restoration changed the state tree.
type Result struct {
	Mutated bool
}

type dependencies struct {
	storeDir       string
	uid            uint32
	tempName       func() (string, error)
	leafOperations leafOperations
}

type preparedFile struct {
	destination string
	source      string
}

// Restore atomically restores managed links below root.
func Restore(root Root, files []manifest.ManagedStateFile) (Result, error) {
	return restore(root, files, dependencies{
		storeDir:       "/nix/store",
		uid:            uint32(os.Getuid()),
		tempName:       randomTempName,
		leafOperations: systemLeafOperations,
	})
}

func restore(root Root, files []manifest.ManagedStateFile, deps dependencies) (Result, error) {
	if len(files) == 0 {
		return Result{}, nil
	}
	prepared, err := prepareFiles(root, files, deps.storeDir)
	if err != nil {
		return Result{}, err
	}
	directory, err := openRoot(root, deps.uid)
	if err != nil {
		return Result{}, managedError(prepared[0].destination, err)
	}
	defer directory.close()

	var result Result
	for _, file := range prepared {
		mutated, err := directory.restore(file.destination, file.source, deps)
		if mutated {
			result.Mutated = true
		}
		if err != nil {
			return result, managedError(file.destination, err)
		}
	}
	return result, nil
}

func prepareFiles(root Root, files []manifest.ManagedStateFile, storeDir string) ([]preparedFile, error) {
	if !safeAbsolutePath(root.Path) || root.Device == 0 || root.Inode == 0 {
		return nil, managedError(files[0].Destination, errors.New("state root is invalid"))
	}
	if !safeAbsolutePath(storeDir) {
		return nil, managedError(files[0].Destination, errors.New("Nix store path is invalid"))
	}
	prepared := make([]preparedFile, 0, len(files))
	for _, file := range files {
		if !safeRelativePath(file.Destination) {
			return nil, managedError(file.Destination, errors.New("managed destination is invalid"))
		}
		if !safeAbsolutePath(file.Source) || !pathBelow(file.Source, storeDir) {
			return nil, managedError(file.Destination, errors.New("source is outside the Nix store"))
		}
		resolved, err := filepath.EvalSymlinks(file.Source)
		if err != nil {
			return nil, managedError(file.Destination, errors.New("source cannot be resolved"))
		}
		if !pathBelow(resolved, storeDir) {
			return nil, managedError(file.Destination, errors.New("source resolves outside the Nix store"))
		}
		if _, err := os.Stat(resolved); err != nil {
			return nil, managedError(file.Destination, errors.New("source cannot be read"))
		}
		prepared = append(prepared, preparedFile{destination: file.Destination, source: file.Source})
	}
	return prepared, nil
}

func managedError(destination string, err error) error {
	return fmt.Errorf("managed state %q: %w", destination, err)
}

func safeAbsolutePath(path string) bool {
	return path != "" && filepath.IsAbs(path) && filepath.Clean(path) == path && !strings.ContainsAny(path, "\x00\r\n")
}

func safeRelativePath(path string) bool {
	if path == "" || filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsAny(path, "\x00\r\n") {
		return false
	}
	for _, component := range strings.Split(path, string(filepath.Separator)) {
		if component == "" || component == "." || component == ".." {
			return false
		}
	}
	return true
}

func pathBelow(path, root string) bool {
	return path != root && strings.HasPrefix(path, root+string(filepath.Separator))
}

func randomTempName() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return ".den-managed-" + hex.EncodeToString(bytes), nil
}
