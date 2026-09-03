//go:build linux

package configdir

import (
	"errors"
	"strings"
)

func parseACL(output []byte, ownerName, ownerID string) (aclAccess, []byte, error) {
	var access aclAccess
	var canonical strings.Builder
	for _, rawLine := range strings.Split(string(output), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		canonical.WriteString(line)
		canonical.WriteByte('\n')
		line = strings.TrimPrefix(line, "default:")
		entry, effective, _ := strings.Cut(line, "#effective:")
		parts := strings.Split(strings.TrimSpace(entry), ":")
		if len(parts) != 3 {
			return aclAccess{}, nil, errors.New("invalid ACL entry")
		}
		kind, principal, permissions := parts[0], parts[1], strings.TrimSpace(parts[2])
		if effective != "" {
			permissions = strings.TrimSpace(effective)
		}
		if !validLinuxPermissions(permissions) {
			return aclAccess{}, nil, errors.New("invalid ACL permissions")
		}
		nonOwner := false
		switch kind {
		case "user":
			nonOwner = principal != "" && principal != ownerName && principal != ownerID
		case "group", "other":
			nonOwner = true
		case "mask":
			continue
		default:
			return aclAccess{}, nil, errors.New("invalid ACL kind")
		}
		if nonOwner && permissions != "---" {
			access.nonOwnerAny = true
			if strings.Contains(permissions, "w") {
				access.nonOwnerWrite = true
			}
		}
	}
	return access, []byte(canonical.String()), nil
}

func validLinuxPermissions(permissions string) bool {
	if len(permissions) != 3 {
		return false
	}
	return (permissions[0] == 'r' || permissions[0] == '-') &&
		(permissions[1] == 'w' || permissions[1] == '-') &&
		(permissions[2] == 'x' || permissions[2] == '-')
}
