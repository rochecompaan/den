package configdir

import "os"

func directoryWritable(path string) bool {
	return directoryWritableWith(path, os.CreateTemp)
}

func directoryWritableWith(path string, create func(string, string) (*os.File, error)) bool {
	probe, err := create(path, ".den-write-probe-*")
	if err != nil {
		return false
	}
	name := probe.Name()
	removeErr := os.Remove(name)
	closeErr := probe.Close()
	if removeErr != nil {
		_ = os.Remove(name)
	}
	return removeErr == nil && closeErr == nil
}
