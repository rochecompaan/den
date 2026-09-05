package manifest

import (
	"os"
	"strings"
	"testing"
)

const validManifest = `{
 "version":2,"platform":"linux","fenceExecutable":"/nix/store/fence/bin/fence","repoWolfClientDir":"/nix/store/repowolf","basePolicy":"/nix/store/policy.json","closurePathsFile":"/nix/store/closures","scratchRoot":"/tmp","aclProbe":["/usr/bin/getfacl"],"protectedPathPatterns":["~/.ssh/id_*"],"pathEntries":["/nix/store/bin"],
 "agent":{"name":"claude","executable":"/nix/store/claude/bin/claude","commandName":"claude","argumentPolicy":"claude","mandatoryArgs":["--safe"],"resourceArgs":[],"reservedFlags":["--safe"],"reservedCommands":[],"environment":{"scrub":[],"set":{}},"packageDirectory":null,"securityAdapter":null},
 "stateBindings":[{"name":"config","explicitPath":null,"inheritedEnvironment":"CLAUDE_CONFIG_DIR","defaultPath":"","defaultWritablePaths":[],"exports":[{"kind":"environment","name":"CLAUDE_CONFIG_DIR","exportDefault":false}]}],
 "docker":{"enable":false,"socketPath":null,"hostPorts":[],"clientPrograms":[]},"podman":{"enable":false,"socketPath":null,"hostPorts":[],"clientPrograms":[]}
}`

func TestLoadVersion2Manifest(t *testing.T) {
	got, err := Load(writeManifest(t, validManifest))
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != CurrentVersion || got.Agent.CommandName != "claude" || len(got.StateBindings) != 1 {
		t.Fatalf("manifest = %#v", got)
	}
}

func TestLoadVersion2RejectsVersionOneAndUnknown(t *testing.T) {
	for _, content := range []string{strings.Replace(validManifest, `"version":2`, `"version":1`, 1), strings.Replace(validManifest, `"version":2`, `"version":3`, 1)} {
		if _, err := Load(writeManifest(t, content)); err == nil || !strings.Contains(err.Error(), "manifest version") {
			t.Fatalf("Load() error = %v, want version rejection", err)
		}
	}
}

func TestValidateStateBinding(t *testing.T) {
	cases := []struct{ name, old, new, field string }{
		{"duplicate names", `"stateBindings":[{`, `"stateBindings":[{"name":"config","explicitPath":null,"inheritedEnvironment":"OTHER","defaultPath":"","defaultWritablePaths":[],"exports":[{"kind":"environment","name":"OTHER","exportDefault":false}]},{`, "stateBindings"},
		{"no exports", `"exports":[{"kind":"environment","name":"CLAUDE_CONFIG_DIR","exportDefault":false}]`, `"exports":[]`, "stateBindings"},
		{"unknown export", `"kind":"environment"`, `"kind":"file"`, "stateBindings.exports"},
		{"unsafe export name", `"CLAUDE_CONFIG_DIR"`, `"BAD-NAME"`, "stateBindings"},
		{"newline path", `"/nix/store/policy.json"`, `"/nix/store/policy\n.json"`, "basePolicy"},
		{"relative explicit", `"explicitPath":null`, `"explicitPath":"relative"`, "explicitPath"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := Load(writeManifest(t, strings.Replace(validManifest, test.old, test.new, 1)))
			if err == nil || !strings.Contains(err.Error(), test.field) {
				t.Fatalf("Load() error = %v, want %q", err, test.field)
			}
		})
	}
}

func TestLoadRejectsUnsafeAgentFields(t *testing.T) {
	for _, test := range []struct{ old, new string }{
		{`"commandName":"claude"`, `"commandName":"bad/name"`}, {`"argumentPolicy":"claude"`, `"argumentPolicy":""`}, {`"--safe"`, `""`}, {`"securityAdapter":null`, `"securityAdapter":{"kind":"settings","path":"relative","arguments":[]}`},
	} {
		if _, err := Load(writeManifest(t, strings.Replace(validManifest, test.old, test.new, 1))); err == nil {
			t.Fatalf("Load accepted %s", test.new)
		}
	}
}

func writeManifest(t *testing.T, content string) string {
	t.Helper()
	path := t.TempDir() + "/manifest.json"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
