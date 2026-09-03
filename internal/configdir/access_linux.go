//go:build linux

package configdir

import "os"

func directoryHandleWritable(file *os.File) bool {
	return directoryWritable(localDirectoryHandlePath(file))
}
