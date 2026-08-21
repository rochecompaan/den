//go:build !linux

package tempdir

import (
	"os"
	"path/filepath"
	"strconv"
)

// Non-Linux hosts use the descriptor-backed /dev/fd namespace after stale.go
// atomically moves and revalidates the candidate under its opened parent.
func removeTreeAt(parentFD int, name string, _ int) error {
	return os.RemoveAll(filepath.Join("/dev/fd", strconv.Itoa(parentFD), name))
}
