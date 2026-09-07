package manifest

import (
	"os"
	"strings"
	"testing"
)

const validManifest = `{
 "version":2,"platform":"linux","fenceExecutable":"/nix/store/fence/bin/fence","repoWolfClientDir":"/nix/store/repowolf","basePolicy":"/nix/store/policy.json","closurePathsFile":"/nix/store/closures","scratchRoot":"/tmp","aclProbe":["/usr/bin/getfacl"],"protectedPathPatterns":["~/.ssh/id_*"],"pathEntries":["/nix/store/bin"],
 "agent":{"name":"claude","executable":"/nix/store/claude/bin/claude","commandName":"claude","argumentPolicy":"claude","mandatoryArgs":["--safe"],"resourceArgs":[],"reservedFlags":["--settings","--permission-mode","--dangerously-skip-permissions"],"reservedCommands":[],"environment":{"scrub":[],"set":{}},"packageDirectory":null,"securityAdapter":null},
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

func TestLoadVersion2RequiresBindingAndAllowsRelativeDefault(t *testing.T) {
	withoutBindings := strings.Replace(validManifest, `"stateBindings":[{"name":"config","explicitPath":null,"inheritedEnvironment":"CLAUDE_CONFIG_DIR","defaultPath":"","defaultWritablePaths":[],"exports":[{"kind":"environment","name":"CLAUDE_CONFIG_DIR","exportDefault":false}]}],`, `"stateBindings":[],`, 1)
	if _, err := Load(writeManifest(t, withoutBindings)); err == nil {
		t.Fatal("Load() accepted empty stateBindings")
	}
	relativeDefault := strings.Replace(validManifest, `"defaultPath":""`, `"defaultPath":".local/state/den/pi/agent"`, 1)
	if _, err := Load(writeManifest(t, relativeDefault)); err != nil {
		t.Fatalf("Load() rejected relative default: %v", err)
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
		{"unsafe inherited name", `"CLAUDE_CONFIG_DIR"`, `"CLAUDE-CONFIG"`, "stateBindings"},
		{"unsafe default path", `"defaultPath":""`, `"defaultPath":"../escape"`, "defaultPath"},
		{"newline default path", `"defaultPath":""`, `"defaultPath":"bad\npath"`, "defaultPath"},
	}
	if _, err := Load(writeManifest(t, strings.Replace(validManifest, `"version":2`, `"version":2,"unexpected":true`, 1))); err == nil {
		t.Fatal("Load accepted unknown field")
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

func TestLoadRejectsUnsafeAgentContractValues(t *testing.T) {
	// Catches validation regressions that admit malformed process controls into
	// the launcher after the manifest has passed its version and schema checks.
	for _, test := range []struct {
		name, old, new string
	}{
		{"empty agent name", `"name":"claude"`, `"name":""`},
		{"unsafe agent name", `"name":"claude"`, `"name":"bad/name"`},
		{"slash command name", `"commandName":"claude"`, `"commandName":"bad/name"`},
		{"empty command name", `"commandName":"claude"`, `"commandName":""`},
		{"newline command name", `"commandName":"claude"`, `"commandName":"bad\nname"`},
		{"empty argument policy", `"argumentPolicy":"claude"`, `"argumentPolicy":""`},
		{"unsafe environment scrub name", `"scrub":[]`, `"scrub":["BAD-NAME"]`},
		{"empty environment scrub name", `"scrub":[]`, `"scrub":[""]`},
		{"unsafe environment set name", `"set":{}`, `"set":{"BAD-NAME":"value"}`},
		{"empty environment set name", `"set":{}`, `"set":{"":"value"}`},
		{"empty environment set value", `"set":{}`, `"set":{"SAFE":""}`},
		{"unsafe environment export name", `"name":"CLAUDE_CONFIG_DIR"`, `"name":"BAD-NAME"`},
		{"empty environment export name", `"name":"CLAUDE_CONFIG_DIR"`, `"name":""`},
		{"unsafe argument export name", `"kind":"environment","name":"CLAUDE_CONFIG_DIR"`, `"kind":"argument","name":"session-dir"`},
		{"empty mandatory argument", `"mandatoryArgs":["--safe"]`, `"mandatoryArgs":[""]`},
		{"newline resource argument", `"resourceArgs":[]`, `"resourceArgs":["bad\nresource"]`},
		{"empty resource argument", `"resourceArgs":[]`, `"resourceArgs":[""]`},
		{"empty reserved flag", `"--settings"`, `""`},
		{"carriage-return reserved command", `"reservedCommands":[]`, `"reservedCommands":["bad\rcommand"]`},
		{"empty reserved command", `"reservedCommands":[]`, `"reservedCommands":[""]`},
		{"empty security adapter kind", `"securityAdapter":null`, `"securityAdapter":{"kind":"","path":"/nix/store/adapter","arguments":[]}`},
		{"unsafe security adapter kind", `"securityAdapter":null`, `"securityAdapter":{"kind":"bad/kind","path":"/nix/store/adapter","arguments":[]}`},
		{"relative security adapter path", `"securityAdapter":null`, `"securityAdapter":{"kind":"adapter","path":"relative","arguments":[]}`},
		{"empty security argument", `"securityAdapter":null`, `"securityAdapter":{"kind":"adapter","path":"/nix/store/adapter","arguments":[""]}`},
		{"newline security argument", `"securityAdapter":null`, `"securityAdapter":{"kind":"adapter","path":"/nix/store/adapter","arguments":["bad\nargument"]}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Load(writeManifest(t, strings.Replace(validManifest, test.old, test.new, 1))); err == nil {
				t.Fatalf("Load accepted malformed %s", test.name)
			}
		})
	}
}

func TestLoadRejectsArgumentPolicyTableMismatch(t *testing.T) {
	pi := strings.ReplaceAll(validManifest, `"name":"claude"`, `"name":"pi"`)
	pi = strings.ReplaceAll(pi, `"commandName":"claude","argumentPolicy":"claude","mandatoryArgs":["--safe"],"resourceArgs":[],"reservedFlags":["--settings","--permission-mode","--dangerously-skip-permissions"],"reservedCommands":[]`, `"commandName":"pi","argumentPolicy":"pi-0.84.4","mandatoryArgs":[],"resourceArgs":[],"reservedFlags":["--session-dir","--session","--fork","--export","--extension","-e","--skill","--prompt-template","--theme"],"reservedCommands":["install","remove","uninstall","update","list","config"]`)
	if _, err := Load(writeManifest(t, pi)); err != nil {
		t.Fatalf("Load() rejected Pi policy table: %v", err)
	}
	mismatch := strings.Replace(pi, `"--theme"`, `"--unknown"`, 1)
	if _, err := Load(writeManifest(t, mismatch)); err == nil {
		t.Fatal("Load() accepted mismatched Pi reserved flags")
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
