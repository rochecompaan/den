package tempdir

import "strings"

const removalPrefix = ".den-removing-"

func temporaryName(name string) bool {
	return strings.HasPrefix(name, "policy-") || strings.HasPrefix(name, "scratch-") || validRemovalName(name)
}

func validRemovalName(name string) bool {
	suffix, ok := strings.CutPrefix(name, removalPrefix)
	if !ok || len(suffix) != 24 {
		return false
	}
	for _, character := range suffix {
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f')) {
			return false
		}
	}
	return true
}
