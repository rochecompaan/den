package configdir

import (
	"crypto/sha256"
	"os"
	"path/filepath"
)

func captureAncestors(start string, ownerUID uint32, ownerName, ownerID string, probe aclProbe) ([]pathSnapshot, error) {
	paths := ancestorPaths(start)
	snapshots := make([]pathSnapshot, len(paths))
	trust := ancestorTrust{ownerUID: ownerUID}
	// Check parents before children: a private directory cannot protect its
	// own name from replacement through an unsafe parent.
	for index := len(paths) - 1; index >= 0; index-- {
		path := paths[index]
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
		if err := trust.check(identity, access); err != nil {
			return nil, err
		}
		// Retain every snapshot, including writable ancestors below a boundary,
		// so revalidation still detects permission, ACL, and identity changes.
		snapshots[index] = pathSnapshot{path: path, identity: identity, acl: acl}
	}
	return snapshots, nil
}

// ancestorTrust checks permissions in root-to-leaf order, independently of
// filesystem inspection. A private boundary protects only its descendants.
type ancestorTrust struct {
	ownerUID        uint32
	privateBoundary bool
	foreignParent   bool
}

func (trust *ancestorTrust) check(identity fileIdentity, access aclAccess) error {
	stickyOwner := identity.mode&os.ModeSticky != 0 && (identity.uid == 0 || identity.uid == trust.ownerUID)
	if !trust.privateBoundary && (identity.mode.Perm()&0o022 != 0 || access.nonOwnerWrite) && !stickyOwner {
		return errPrivate
	}
	// A foreign parent owner can change permissions or replace descendants.
	trust.foreignParent = trust.foreignParent || (identity.uid != 0 && identity.uid != trust.ownerUID)
	if !trust.foreignParent && identity.uid == trust.ownerUID && privateDirectoryMode(identity.mode) && !access.nonOwnerAny {
		trust.privateBoundary = true
	}
	return nil
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
