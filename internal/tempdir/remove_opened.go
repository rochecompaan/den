package tempdir

import (
	"os"
	"path/filepath"
)

// removeOpenedDirectoryContents removes children through a stable descriptor
// path. It never resolves children through the original directory name.
func removeOpenedDirectoryContents(directory *os.File, descriptorPath string) error {
	entries, err := directory.ReadDir(-1)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(descriptorPath, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}
