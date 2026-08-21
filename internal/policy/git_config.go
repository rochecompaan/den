package policy

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const maxGitPathFileSize = 4096

var errGitMetadata = errors.New("policy: resolve Git metadata")

// gitConfigDenyPaths resolves the repository-wide config and the optional
// per-worktree config for directory, symlinked-directory, and gitfile forms.
func gitConfigDenyPaths(worktree string) ([]string, error) {
	dotGit := filepath.Join(worktree, ".git")
	info, err := os.Lstat(dotGit)
	if os.IsNotExist(err) {
		return canonicalGitConfigPaths(dotGit, dotGit)
	}
	if err != nil {
		return nil, errGitMetadata
	}

	gitDirectory, err := resolveGitDirectory(dotGit, info)
	if err != nil {
		return nil, err
	}
	commonDirectory, err := resolveCommonGitDirectory(gitDirectory)
	if err != nil {
		return nil, err
	}
	return canonicalGitConfigPaths(commonDirectory, gitDirectory)
}

func resolveGitDirectory(dotGit string, info os.FileInfo) (string, error) {
	if info.Mode().IsRegular() {
		value, err := readPrefixedGitPathFile(dotGit, "gitdir: ")
		if err != nil {
			return "", err
		}
		return resolveExistingGitDirectory(filepath.Dir(dotGit), value)
	}
	if info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		resolved, err := canonicalPath("Git metadata", dotGit)
		if err != nil {
			return "", errGitMetadata
		}
		resolvedInfo, err := os.Stat(resolved)
		if err != nil || !resolvedInfo.IsDir() {
			return "", errGitMetadata
		}
		return resolved, nil
	}
	return "", errGitMetadata
}

func resolveCommonGitDirectory(gitDirectory string) (string, error) {
	commondir := filepath.Join(gitDirectory, "commondir")
	_, err := os.Lstat(commondir)
	if os.IsNotExist(err) {
		return gitDirectory, nil
	}
	if err != nil {
		return "", errGitMetadata
	}
	value, err := readGitPathFile(commondir)
	if err != nil {
		return "", err
	}
	return resolveExistingGitDirectory(gitDirectory, value)
}

func resolveExistingGitDirectory(base, value string) (string, error) {
	path := value
	if !filepath.IsAbs(path) {
		path = filepath.Join(base, path)
	}
	path = filepath.Clean(path)
	resolved, err := canonicalPath("Git metadata", path)
	if err != nil {
		return "", errGitMetadata
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", errGitMetadata
	}
	return resolved, nil
}

func canonicalGitConfigPaths(commonDirectory, gitDirectory string) ([]string, error) {
	config, err := canonicalPath("Git config", filepath.Join(commonDirectory, "config"))
	if err != nil {
		return nil, errGitMetadata
	}
	worktreeConfig, err := canonicalPath("Git worktree config", filepath.Join(gitDirectory, "config.worktree"))
	if err != nil {
		return nil, errGitMetadata
	}
	return []string{config, worktreeConfig}, nil
}

func readPrefixedGitPathFile(path, prefix string) (string, error) {
	value, err := readGitPathFile(path)
	if err != nil || !strings.HasPrefix(value, prefix) {
		return "", errGitMetadata
	}
	value = strings.TrimPrefix(value, prefix)
	if value == "" {
		return "", errGitMetadata
	}
	return value, nil
}

func readGitPathFile(path string) (string, error) {
	beforeOpen, err := os.Lstat(path)
	if err != nil || !beforeOpen.Mode().IsRegular() || beforeOpen.Size() > maxGitPathFileSize {
		return "", errGitMetadata
	}
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return "", errGitMetadata
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxGitPathFileSize || !os.SameFile(beforeOpen, info) {
		return "", errGitMetadata
	}
	data, err := io.ReadAll(io.LimitReader(file, maxGitPathFileSize+1))
	if err != nil || len(data) == 0 || len(data) > maxGitPathFileSize || strings.IndexByte(string(data), 0) >= 0 {
		return "", errGitMetadata
	}
	value := strings.TrimSuffix(string(data), "\n")
	value = strings.TrimSuffix(value, "\r")
	if value == "" || strings.ContainsAny(value, "\r\n") || strings.TrimSpace(value) != value {
		return "", errGitMetadata
	}
	return value, nil
}
