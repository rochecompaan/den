package arguments

import (
	"reflect"
	"testing"
)

func TestValidateArguments(t *testing.T) {
	piFlags := []string{"--session-dir", "--session", "--fork", "--export", "--extension", "-e", "--skill", "--prompt-template", "--theme"}
	piCommands := []string{"install", "remove", "uninstall", "update", "list", "config"}
	claudeFlags := []string{"--settings", "--permission-mode", "--dangerously-skip-permissions"}
	for _, test := range []struct {
		name, policy          string
		flags, commands, user []string
		wantErr               bool
	}{
		{"Pi policy", "pi-0.84.4", piFlags, piCommands, []string{"--continue"}, false},
		{"Claude policy preserves existing flags", "claude", claudeFlags, nil, []string{"--plugin-dir", "plugin"}, false},
		{"unknown policy is invalid", "unknown", nil, nil, nil, true},
		{"Pi manifest flags must match", "pi-0.84.4", piFlags[:len(piFlags)-1], piCommands, nil, true},
		{"Pi manifest commands must match", "pi-0.84.4", piFlags, piCommands[:len(piCommands)-1], nil, true},
		{"Claude manifest flags must match", "claude", nil, nil, nil, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			original := append([]string(nil), test.user...)
			err := Validate(test.policy, test.flags, test.commands, test.user)
			if (err != nil) != test.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %t", err, test.wantErr)
			}
			if !reflect.DeepEqual(test.user, original) {
				t.Fatalf("Validate() mutated user arguments: %#v", test.user)
			}
		})
	}
}
