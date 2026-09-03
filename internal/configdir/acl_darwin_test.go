//go:build darwin

package configdir

import (
	"fmt"
	"testing"
)

func TestSelectRejectsDarwinOwnerWriteDeny(t *testing.T) {
	root := t.TempDir()
	home := privateDir(t, root, "home")
	path := privateDir(t, root, "config")
	ownerDeny := fmt.Sprintf("0: user:%s deny write\n", currentUserName(t))

	if _, err := Select(&path, nil, home, nil, targetedDependencies(t, path, ownerDeny, safeACL(t))); err == nil {
		t.Fatal("Select() accepted an owner write-deny ACL")
	}
}
