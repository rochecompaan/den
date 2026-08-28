package configdir

import (
	"crypto/sha256"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"
)

type aclProbe struct {
	executable string
	arguments  []string
}

type aclAccess struct {
	nonOwnerAny      bool
	nonOwnerWrite    bool
	ownerWriteDenied bool
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

func parseDarwinACL(output []byte, ownerName, ownerID string) (aclAccess, []byte, error) {
	var access aclAccess
	var canonical strings.Builder
	ownerDecisions := make(map[string]bool)
	for _, rawLine := range strings.Split(string(output), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || !darwinACLLine(line) {
			continue
		}
		canonical.WriteString(line)
		canonical.WriteByte('\n')
		fields := strings.Fields(line)
		principal, action, permissions, err := darwinACE(fields)
		if err != nil {
			return aclAccess{}, nil, err
		}
		owner := strings.HasPrefix(principal, "user:") &&
			(strings.TrimPrefix(principal, "user:") == ownerName || strings.TrimPrefix(principal, "user:") == ownerID)
		if owner {
			for _, permission := range permissions {
				if !darwinWritePermission(permission) {
					continue
				}
				if _, decided := ownerDecisions[permission]; !decided {
					ownerDecisions[permission] = action == "deny"
				}
			}
			continue
		}
		if action == "allow" && len(permissions) > 0 {
			access.nonOwnerAny = true
			for _, permission := range permissions {
				if darwinWritePermission(permission) {
					access.nonOwnerWrite = true
				}
			}
		}
	}
	for _, denied := range ownerDecisions {
		access.ownerWriteDenied = access.ownerWriteDenied || denied
	}
	return access, []byte(canonical.String()), nil
}

func darwinACE(fields []string) (string, string, []string, error) {
	principal := ""
	actionIndex := -1
	for index, field := range fields {
		if strings.HasPrefix(field, "user:") || strings.HasPrefix(field, "group:") {
			principal = field
		}
		if field == "allow" || field == "deny" {
			actionIndex = index
		}
	}
	if principal == "" || actionIndex < 0 || actionIndex+1 >= len(fields) {
		return "", "", nil, errors.New("invalid ACL entry")
	}
	permissions := strings.Split(strings.Join(fields[actionIndex+1:], ""), ",")
	return principal, fields[actionIndex], permissions, nil
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

func snapshotACLProbe(arguments []string) (aclProbe, error) {
	if len(arguments) == 0 || !filepath.IsAbs(arguments[0]) || strings.IndexByte(arguments[0], 0) >= 0 {
		return aclProbe{}, errACL
	}
	probe := aclProbe{executable: arguments[0], arguments: append([]string(nil), arguments[1:]...)}
	for _, argument := range probe.arguments {
		if argument == "" || strings.IndexByte(argument, 0) >= 0 {
			return aclProbe{}, errACL
		}
	}
	return probe, nil
}

func inspectACL(path, ownerName, ownerID string, probe aclProbe) ([sha256.Size]byte, aclAccess, error) {
	return runACLProbe(path, path, ownerName, ownerID, probe)
}

func runACLProbe(probePath, originalPath, ownerName, ownerID string, probe aclProbe) ([sha256.Size]byte, aclAccess, error) {
	arguments := append([]string{}, probe.arguments...)
	arguments = append(arguments, probePath)
	command := exec.Command(probe.executable, arguments...)
	command.Env = aclProbeEnvironment(originalPath)
	output, err := command.Output()
	if err != nil {
		return [sha256.Size]byte{}, aclAccess{}, errACL
	}
	access, canonicalACL, err := parseACL(output, ownerName, ownerID)
	if err != nil {
		return [sha256.Size]byte{}, aclAccess{}, errACL
	}
	return aclDigest(canonicalACL), access, nil
}

func aclProbeEnvironment(originalPath string) []string {
	const name = "DEN_CONFIGDIR_ACL_ORIGINAL_PATH"
	environment := make([]string, 0, len(os.Environ())+1)
	for _, entry := range os.Environ() {
		entryName, _, ok := strings.Cut(entry, "=")
		if ok && entryName == name {
			continue
		}
		environment = append(environment, entry)
	}
	return append(environment, name+"="+originalPath)
}
