package configdir

import (
	"crypto/sha256"
	"os/exec"
	"strings"
)

type aclAccess struct {
	nonOwnerAny   bool
	nonOwnerWrite bool
}

func darwinWritePermission(permissions string) bool {
	permissions = strings.ToLower(permissions)
	for _, permission := range strings.Split(permissions, ",") {
		permission = strings.TrimSpace(permission)
		if strings.HasPrefix(permission, "write") || strings.HasPrefix(permission, "add_") ||
			permission == "append" || permission == "delete" || permission == "delete_child" || permission == "chown" {
			return true
		}
	}
	return false
}

func inspectACL(path, ownerName, ownerID string, deps Dependencies) ([sha256.Size]byte, aclAccess, error) {
	if len(deps.ACLProbe) == 0 {
		return [sha256.Size]byte{}, aclAccess{}, errACL
	}
	arguments := append([]string{}, deps.ACLProbe[1:]...)
	arguments = append(arguments, path)
	output, err := exec.Command(deps.ACLProbe[0], arguments...).Output()
	if err != nil {
		return [sha256.Size]byte{}, aclAccess{}, errACL
	}
	access, canonicalACL, err := parseACL(output, ownerName, ownerID)
	if err != nil {
		return [sha256.Size]byte{}, aclAccess{}, errACL
	}
	return aclDigest(canonicalACL), access, nil
}
