// Package arguments dispatches adapter-specific launcher argument validation.
package arguments

import (
	"errors"
	"reflect"

	"github.com/rochecompaan/den/internal/claude"
	"github.com/rochecompaan/den/internal/piargs"
)

var claudeReservedFlags = []string{"--settings", "--permission-mode", "--dangerously-skip-permissions"}

// Validate verifies the adapter policy table and untrusted user arguments.
func Validate(policy string, reservedFlags, reservedCommands, userArgs []string) error {
	switch policy {
	case "claude":
		if !reflect.DeepEqual(reservedFlags, claudeReservedFlags) || len(reservedCommands) != 0 {
			return errors.New("manifest Claude argument policy is invalid")
		}
		return claude.ValidateArguments(userArgs)
	case "pi-0.84.4":
		return piargs.Validate(reservedFlags, reservedCommands, userArgs)
	default:
		return errors.New("manifest argument policy is invalid")
	}
}
