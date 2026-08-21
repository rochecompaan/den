package configdir

import (
	"os"
	"path/filepath"
	"strings"
)

func pathsOverlap(left, right string) bool {
	return pathsOverlapWithCase(left, right, prospectiveCaseInsensitive())
}

func pathsOverlapWithCase(left, right string, foldCase bool) bool {
	if pathContainsWithCase(left, right, foldCase) || pathContainsWithCase(right, left, foldCase) {
		return true
	}
	return pathsOverlapByIdentity(left, right, foldCase)
}

func pathContainsWithCase(parent, child string, foldCase bool) bool {
	if !foldCase {
		return pathContains(parent, child)
	}
	parentVolume, parentComponents := absolutePathComponents(parent)
	childVolume, childComponents := absolutePathComponents(child)
	return strings.EqualFold(parentVolume, childVolume) && componentsContain(parentComponents, childComponents, true)
}

func absolutePathComponents(path string) (string, []string) {
	clean := filepath.Clean(path)
	volume := filepath.VolumeName(clean)
	remainder := strings.TrimPrefix(clean[len(volume):], string(os.PathSeparator))
	return volume, splitPathComponents(remainder)
}

type pathCheckpoint struct {
	info      os.FileInfo
	remaining []string
}

func pathsOverlapByIdentity(left, right string, foldCase bool) bool {
	leftCheckpoints := pathCheckpoints(left)
	rightCheckpoints := pathCheckpoints(right)
	for _, leftCheckpoint := range leftCheckpoints {
		for _, rightCheckpoint := range rightCheckpoints {
			if !os.SameFile(leftCheckpoint.info, rightCheckpoint.info) {
				continue
			}
			if componentsContain(leftCheckpoint.remaining, rightCheckpoint.remaining, foldCase) ||
				componentsContain(rightCheckpoint.remaining, leftCheckpoint.remaining, foldCase) {
				return true
			}
		}
	}
	return false
}

func pathCheckpoints(path string) []pathCheckpoint {
	clean := filepath.Clean(path)
	volume := filepath.VolumeName(clean)
	root := volume + string(os.PathSeparator)
	remainder := strings.TrimPrefix(clean[len(volume):], string(os.PathSeparator))
	components := splitPathComponents(remainder)
	checkpoints := make([]pathCheckpoint, 0, len(components)+1)
	current := root
	for index := 0; ; index++ {
		info, err := os.Stat(current)
		if err != nil {
			return checkpoints
		}
		checkpoints = append(checkpoints, pathCheckpoint{info: info, remaining: components[index:]})
		if index == len(components) {
			return checkpoints
		}
		current = filepath.Join(current, components[index])
	}
}

func componentsContain(parent, child []string, foldCase bool) bool {
	if len(parent) > len(child) {
		return false
	}
	for index := range parent {
		if parent[index] == child[index] {
			continue
		}
		if !foldCase || !strings.EqualFold(parent[index], child[index]) {
			return false
		}
	}
	return true
}

func splitPathComponents(path string) []string {
	if path == "" || path == "." {
		return nil
	}
	return strings.Split(path, string(os.PathSeparator))
}

func pathContains(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return relative == "." || relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator))
}
