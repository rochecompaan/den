//go:build linux

package tempdir

import (
	"fmt"
	"os"
)

func openedDirectoryPath(directory *os.File) string {
	return fmt.Sprintf("/proc/self/fd/%d", directory.Fd())
}
