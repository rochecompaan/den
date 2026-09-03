package configdir

import (
	"crypto/sha256"
	"os"
	"path/filepath"
)

func captureAncestors(start string, ownerUID uint32, ownerName, ownerID string, probe aclProbe) ([]pathSnapshot, error) {
	paths := ancestorPaths(start)
	snapshots := make([]pathSnapshot, 0, len(paths))
	for _, path := range paths {
		info, err := os.Lstat(path)
		if err != nil || info == nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, errInvalid
		}
		identity, ok := identityFromFileInfo(info)
		if !ok {
			return nil, errInvalid
		}
		acl, access, err := inspectACL(path, ownerName, ownerID, probe)
		if err != nil {
			return nil, err
		}
		stickyOwner := identity.mode&os.ModeSticky != 0 && (identity.uid == 0 || identity.uid == ownerUID)
		if (identity.mode.Perm()&0o022 != 0 || access.nonOwnerWrite) && !stickyOwner {
			return nil, errPrivate
		}
		snapshots = append(snapshots, pathSnapshot{path: path, identity: identity, acl: acl})
	}
	return snapshots, nil
}

func ancestorPaths(start string) []string {
	paths := make([]string, 0)
	for current := filepath.Clean(start); ; current = filepath.Dir(current) {
		paths = append(paths, current)
		parent := filepath.Dir(current)
		if parent == current {
			return paths
		}
	}
}

func aclDigest(output []byte) [sha256.Size]byte {
	return sha256.Sum256(output)
}
