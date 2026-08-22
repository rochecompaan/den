//go:build darwin

package tempdir

import (
	"fmt"
	"os"
)

func openedDirectoryPath(directory *os.File) string {
	return fmt.Sprintf("/dev/fd/%d", directory.Fd())
}
