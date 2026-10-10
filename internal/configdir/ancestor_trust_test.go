package configdir

import (
	"os"
	"testing"
)

func TestAncestorTrustChecksPrivateBoundaryPermissions(t *testing.T) {
	const ownerUID = 1000
	type ancestor struct {
		identity fileIdentity
		access   aclAccess
	}
	directory := func(uid uint32, mode os.FileMode) ancestor {
		return ancestor{identity: fileIdentity{uid: uid, mode: os.ModeDir | mode}}
	}
	withACL := func(entry ancestor, access aclAccess) ancestor {
		entry.access = access
		return entry
	}
	root := directory(0, 0o755)
	private := directory(ownerUID, 0o700)
	public := directory(ownerUID, 0o755)
	groupWritable := directory(ownerUID, 0o775)
	worldWritable := directory(ownerUID, 0o777)
	readACL := aclAccess{nonOwnerAny: true}
	writeACL := aclAccess{nonOwnerAny: true, nonOwnerWrite: true}

	for _, test := range []struct {
		name      string
		ancestors []ancestor
		reject    bool
	}{
		{"private boundary permits group writes", []ancestor{root, private, groupWritable}, false},
		{"private boundary permits world writes", []ancestor{root, private, worldWritable}, false},
		{"private boundary permits ACL writes", []ancestor{root, private, withACL(public, writeACL)}, false},
		{"unprotected group writes", []ancestor{root, public, groupWritable}, true},
		{"unprotected ACL writes", []ancestor{root, public, withACL(public, writeACL)}, true},
		{"unsafe parent above private boundary", []ancestor{root, groupWritable, private}, true},
		{"writable ACL above private boundary", []ancestor{root, withACL(public, writeACL), private}, true},
		{"non-owner ACL access prevents boundary", []ancestor{root, withACL(private, readACL), groupWritable}, true},
		{"group access prevents boundary", []ancestor{root, directory(ownerUID, 0o750), groupWritable}, true},
		{"other execute access prevents boundary", []ancestor{root, directory(ownerUID, 0o701), groupWritable}, true},
		{"special bits prevent boundary", []ancestor{root, directory(ownerUID, os.ModeSetgid|0o700), groupWritable}, true},
		{"root-owned private directory is not user boundary", []ancestor{root, directory(0, 0o700), groupWritable}, true},
		{"foreign-owned private directory is not boundary", []ancestor{root, directory(2000, 0o700), groupWritable}, true},
		{"foreign parent can replace boundary", []ancestor{root, directory(2000, 0o755), private, groupWritable}, true},
		{"readonly foreign parent can change its permissions", []ancestor{root, directory(2000, 0o555), private, groupWritable}, true},
		{"foreign namespace root cannot anchor boundary", []ancestor{directory(65534, 0o750), private, groupWritable}, true},
		{"foreign namespace preserves strictly protected paths", []ancestor{directory(65534, 0o750), private, private}, false},
		{"root-owned sticky parent protects boundary", []ancestor{root, directory(0, os.ModeSticky|0o777), private, groupWritable}, false},
		{"user-owned sticky parent protects boundary", []ancestor{root, directory(ownerUID, os.ModeSticky|0o777), private, groupWritable}, false},
		{"foreign-owned sticky parent is unsafe", []ancestor{root, directory(2000, os.ModeSticky|0o777), private}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			trust := ancestorTrust{ownerUID: ownerUID}
			var err error
			for _, entry := range test.ancestors {
				err = trust.check(entry.identity, entry.access)
				if err != nil {
					break
				}
			}
			var want error
			if test.reject {
				want = errPrivate
			}
			if err != want {
				t.Fatalf("ancestor permission check error = %v, want %v", err, want)
			}
		})
	}
}
