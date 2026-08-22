package configdir

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func claudeDefaultPaths(home string) []string {
	return []string{
		directoryPolicyPath(filepath.Join(home, ".claude")),
		filepath.Join(home, ".claude.json"),
		directoryPolicyPath(filepath.Join(home, ".config", "claude")),
	}
}

func directoryPolicyPath(path string) string {
	return path + string(os.PathSeparator)
}

func canonicalFinal(path string) (string, bool, error) {
	clean := filepath.Clean(path)
	info, err := os.Lstat(clean)
	if err == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", false, errInvalid
	}
	if err != nil && !os.IsNotExist(err) {
		return "", false, err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(clean))
	if err != nil {
		return "", false, err
	}
	parentInfo, err := os.Lstat(parent)
	if err != nil || !parentInfo.IsDir() || parentInfo.Mode()&os.ModeSymlink != 0 {
		return "", false, errInvalid
	}
	canonical := filepath.Join(parent, filepath.Base(clean))
	canonicalInfo, canonicalErr := os.Lstat(canonical)
	if canonicalErr == nil && canonicalInfo.Mode()&os.ModeSymlink != 0 {
		return "", false, errInvalid
	}
	if canonicalErr != nil && !os.IsNotExist(canonicalErr) {
		return "", false, canonicalErr
	}
	return canonical, canonicalErr == nil, nil
}

func validateProtectedOverlap(candidate, home string, protected []string) error {
	return validateProtectedOverlapForHome(candidate, home, home, protected)
}

func validateProtectedOverlapForHome(candidate, runtimeHome, protectedHome string, protected []string) error {
	entries := []string{
		filepath.Join(runtimeHome, ".claude"),
		filepath.Join(runtimeHome, ".claude.json"),
		filepath.Join(runtimeHome, ".config", "claude"),
	}
	for _, pattern := range protected {
		root, err := protectedRoot(pattern, protectedHome)
		if err != nil {
			return errInvalid
		}
		entries = append(entries, root)
	}
	for _, entry := range entries {
		canonical, err := canonicalProspective(entry)
		if err != nil {
			if !os.IsPermission(err) {
				return errInvalid
			}
			// An inaccessible protected root cannot alias an accessible,
			// already-canonical candidate. Keep its absolute lexical prefix
			// protected instead of making an otherwise valid launch unusable.
			canonical = filepath.Clean(entry)
		}
		if pathsOverlap(candidate, canonical) {
			return errOverlap
		}
	}
	return nil
}

func expandProtectedPatterns(patterns, homes []string) ([]string, error) {
	result := make([]string, 0, len(patterns)*len(homes))
	seen := make(map[string]struct{})
	for _, pattern := range patterns {
		if filepath.IsAbs(pattern) {
			result = appendUniquePath(result, seen, filepath.Clean(pattern))
			continue
		}
		if pattern != "~" && !strings.HasPrefix(pattern, "~/") {
			return nil, errors.New("relative protected pattern")
		}
		for _, home := range homes {
			if home == "" || !filepath.IsAbs(home) || filepath.Clean(home) != home {
				return nil, errors.New("invalid protected home")
			}
			expanded := home
			if pattern != "~" {
				expanded = filepath.Join(home, strings.TrimPrefix(pattern, "~/"))
			}
			result = appendUniquePath(result, seen, expanded)
		}
	}
	return result, nil
}

func appendUniquePath(paths []string, seen map[string]struct{}, path string) []string {
	if _, exists := seen[path]; exists {
		return paths
	}
	seen[path] = struct{}{}
	return append(paths, path)
}

func protectedRoot(pattern, home string) (string, error) {
	switch {
	case pattern == "~":
		return filepath.Clean(home), nil
	case strings.HasPrefix(pattern, "~/"):
		complete := completePatternComponents(strings.TrimPrefix(pattern, "~/"))
		if len(complete) == 0 {
			return filepath.Clean(home), nil
		}
		return filepath.Clean(filepath.Join(append([]string{home}, complete...)...)), nil
	case !filepath.IsAbs(pattern):
		return "", errors.New("relative protected pattern")
	}

	volume := filepath.VolumeName(pattern)
	remainder := strings.TrimPrefix(pattern[len(volume):], string(os.PathSeparator))
	complete := completePatternComponents(remainder)
	if len(complete) == 0 {
		return "", errors.New("protected glob has no complete root")
	}
	root := volume + string(os.PathSeparator) + filepath.Join(complete...)
	return filepath.Clean(root), nil
}

func completePatternComponents(pattern string) []string {
	components := strings.Split(pattern, string(os.PathSeparator))
	complete := make([]string, 0, len(components))
	for _, component := range components {
		if strings.ContainsAny(component, "*?[") {
			break
		}
		complete = append(complete, component)
	}
	return complete
}

func canonicalProspective(path string) (string, error) {
	clean := filepath.Clean(path)
	missing := make([]string, 0)
	current := clean
	for {
		_, err := os.Lstat(current)
		if err == nil {
			resolved, err := filepath.EvalSymlinks(current)
			if err != nil {
				return "", err
			}
			for index := len(missing) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, missing[index])
			}
			return filepath.Clean(resolved), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", err
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}
