package piargs

import "testing"

func TestPiArgumentPolicy(t *testing.T) {
	validUUID := "123e4567-e89b-42d3-a456-426614174000"
	for _, test := range []struct {
		name string
		args []string
		want bool
	}{
		{"session directory split", []string{"--session-dir", "/tmp/sessions"}, false},
		{"session directory equals", []string{"--session-dir=/tmp/sessions"}, false},
		{"session directory missing", []string{"--session-dir"}, false},
		{"export split", []string{"--export", "session.md"}, false},
		{"export equals", []string{"--export=session.md"}, false},
		{"export missing", []string{"--export"}, false},
		{"extension split", []string{"--extension", "extension.ts"}, false},
		{"extension equals", []string{"--extension=extension.ts"}, false},
		{"extension missing", []string{"--extension"}, false},
		{"extension attached", []string{"-eextension.ts"}, false},
		{"extension combined short", []string{"-ce"}, false},
		{"skill split", []string{"--skill", "skill"}, false},
		{"skill equals", []string{"--skill=skill"}, false},
		{"skill missing", []string{"--skill"}, false},
		{"prompt template split", []string{"--prompt-template", "prompt.md"}, false},
		{"prompt template equals", []string{"--prompt-template=prompt.md"}, false},
		{"prompt template missing", []string{"--prompt-template"}, false},
		{"theme split", []string{"--theme", "theme.json"}, false},
		{"theme equals", []string{"--theme=theme.json"}, false},
		{"theme missing", []string{"--theme"}, false},
		{"session UUID", []string{"--session", validUUID}, true},
		{"session hexadecimal partial ID", []string{"--session", "deadBEEF"}, true},
		{"fork UUID", []string{"--fork", validUUID}, true},
		{"fork hexadecimal partial ID", []string{"--fork", "deadBEEF"}, true},
		{"session path", []string{"--session", "/tmp/session.json"}, false},
		{"session sibling prefix", []string{"--session", "sessions-old/one"}, false},
		{"session empty", []string{"--session", ""}, false},
		{"session malformed", []string{"--session", "not-a-hex-id"}, false},
		{"session equals", []string{"--session=" + validUUID}, false},
		{"session missing", []string{"--session"}, false},
		{"repeated session", []string{"--session", "deadbeef", "--session", "c0ffee"}, true},
		{"session ID UUID", []string{"--session-id", validUUID}, true},
		{"session ID invalid", []string{"--session-id", "deadbeef"}, false},
		{"session ID missing", []string{"--session-id"}, false},
		{"continue", []string{"--continue"}, true},
		{"continue short", []string{"-c"}, true},
		{"resume", []string{"--resume"}, true},
		{"resume short", []string{"-r"}, true},
		{"no session", []string{"--no-session"}, true},
		{"approve", []string{"--approve"}, true},
		{"approve short", []string{"-a"}, true},
		{"no approve", []string{"--no-approve"}, true},
		{"no approve short", []string{"-na"}, true},
		{"no extensions", []string{"--no-extensions"}, true},
		{"no extensions short", []string{"-ne"}, true},
		{"no skills", []string{"--no-skills"}, true},
		{"no skills short", []string{"-ns"}, true},
		{"no templates", []string{"--no-prompt-templates"}, true},
		{"no templates short", []string{"-np"}, true},
		{"no themes", []string{"--no-themes"}, true},
		{"unknown extension option", []string{"--configured-extension-option", "value"}, true},
		{"nested wrapper arguments", []string{"--mode", "rpc", "--", "install"}, true},
		{"package command install", []string{"install"}, false},
		{"package command remove", []string{"remove"}, false},
		{"package command uninstall", []string{"uninstall"}, false},
		{"package command update", []string{"update"}, false},
		{"package command list", []string{"list"}, false},
		{"package command config", []string{"config"}, false},
		{"install after terminator", []string{"--", "install"}, true},
		{"remove after terminator", []string{"--", "remove"}, true},
		{"uninstall after terminator", []string{"--", "uninstall"}, true},
		{"update after terminator", []string{"--", "update"}, true},
		{"list after terminator", []string{"--", "list"}, true},
		{"config after terminator", []string{"--", "config"}, true},
		{"reserved long option after terminator", []string{"--", "--extension"}, true},
		{"reserved session option after terminator", []string{"--", "--session-dir"}, true},
		{"reserved short option after terminator", []string{"--", "-euntrusted.ts"}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := Validate([]string{"--session-dir", "--session", "--fork", "--export", "--extension", "-e", "--skill", "--prompt-template", "--theme"}, []string{"install", "remove", "uninstall", "update", "list", "config"}, test.args)
			if (err == nil) != test.want {
				t.Fatalf("Validate(%#v) error = %v, want valid %t", test.args, err, test.want)
			}
		})
	}
}
