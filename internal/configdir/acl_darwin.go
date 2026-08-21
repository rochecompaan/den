//go:build darwin

package configdir

func parseACL(output []byte, ownerName, ownerID string) (aclAccess, []byte, error) {
	return parseDarwinACL(output, ownerName, ownerID)
}
