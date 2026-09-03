//go:build darwin

package configdir

import (
	"io/fs"
	"syscall"
)

func identityFromFileInfo(info fs.FileInfo) (fileIdentity, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat == nil {
		return fileIdentity{}, false
	}
	return fileIdentity{
		device: uint64(stat.Dev),
		inode:  stat.Ino,
		uid:    stat.Uid,
		mode:   info.Mode(),
	}, true
}
