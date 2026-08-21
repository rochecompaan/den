//go:build darwin

package configdir

import (
	"errors"
	"strings"
	"unicode"
)

func parseACL(output []byte, ownerName, ownerID string) (aclAccess, []byte, error) {
	var access aclAccess
	var canonical strings.Builder
	for _, rawLine := range strings.Split(string(output), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || !darwinACLLine(line) {
			continue
		}
		canonical.WriteString(line)
		canonical.WriteByte('\n')
		fields := strings.Fields(line)
		principalIndex := -1
		allowIndex := -1
		deny := false
		for index, field := range fields {
			if strings.HasPrefix(field, "user:") || strings.HasPrefix(field, "group:") {
				principalIndex = index
			}
			if field == "allow" {
				allowIndex = index
			}
			if field == "deny" {
				deny = true
			}
		}
		if deny {
			continue
		}
		if principalIndex < 0 || allowIndex < 0 || allowIndex+1 >= len(fields) {
			return aclAccess{}, nil, errors.New("invalid ACL entry")
		}
		principal := fields[principalIndex]
		nonOwner := true
		if strings.HasPrefix(principal, "user:") {
			name := strings.TrimPrefix(principal, "user:")
			nonOwner = name != ownerName && name != ownerID
		}
		if !nonOwner {
			continue
		}
		permissions := strings.Join(fields[allowIndex+1:], "")
		if permissions != "" {
			access.nonOwnerAny = true
			if darwinWritePermission(permissions) {
				access.nonOwnerWrite = true
			}
		}
	}
	return access, []byte(canonical.String()), nil
}

func darwinACLLine(line string) bool {
	index := strings.IndexByte(line, ':')
	if index <= 0 {
		return false
	}
	for _, character := range line[:index] {
		if !unicode.IsDigit(character) {
			return false
		}
	}
	return true
}
