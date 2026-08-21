package manifest

import (
	"os"
	"strings"
	"testing"
)

const validManifest = `{
  "version": 1,
  "platform": "linux",
  "fenceExecutable": "/nix/store/fence/bin/fence",
  "repoWolfClientDir": "/nix/store/repowolf-client",
  "basePolicy": "/nix/store/policy/fence.json",
  "closurePathsFile": "/nix/store/closure-paths",
  "scratchRoot": "/tmp",
  "aclProbe": ["/usr/bin/getfacl", "-lde"],
  "protectedPathPatterns": ["/home/user/.ssh/id_*"],
  "pathEntries": ["/nix/store/fence/bin"],
  "explicitConfigDir": null,
  "agent": {
    "name": "example-agent",
    "executable": "/nix/store/example-agent/bin/example-agent",
    "mandatoryArgs": ["--safe"],
    "reservedFlags": ["--safe"],
    "configEnvironment": "EXAMPLE_CONFIG_DIR",
    "defaultStatePaths": ["/home/user/.example"]
  },
  "docker": {
    "enable": false,
    "socketPath": null,
    "hostPorts": [],
    "clientPrograms": []
  },
  "podman": {
    "enable": false,
    "socketPath": null,
    "hostPorts": [],
    "clientPrograms": []
  }
}`

func TestLoadAcceptsVersionOneManifest(t *testing.T) {
	manifest, err := Load(writeManifest(t, validManifest))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if manifest.Agent.Name != "example-agent" {
		t.Fatalf("agent name = %q, want example-agent", manifest.Agent.Name)
	}
}

func TestLoadRejectsUnknownVersion(t *testing.T) {
	path := writeManifest(t, `{"version":2}`)
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "manifest version") {
		t.Fatalf("expected version error, got %v", err)
	}
}

func TestLoadRejectsInvalidManifest(t *testing.T) {
	tests := []struct {
		name  string
		json  string
		field string
	}{
		{"empty fence path", strings.Replace(validManifest, `"/nix/store/fence/bin/fence"`, `""`, 1), "fenceExecutable"},
		{"empty RepoWolf client path", strings.Replace(validManifest, `"/nix/store/repowolf-client"`, `""`, 1), "repoWolfClientDir"},
		{"empty policy path", strings.Replace(validManifest, `"/nix/store/policy/fence.json"`, `""`, 1), "basePolicy"},
		{"empty closure file", strings.Replace(validManifest, `"/nix/store/closure-paths"`, `""`, 1), "closurePathsFile"},
		{"unsupported platform", strings.Replace(validManifest, `"platform": "linux"`, `"platform": "windows"`, 1), "platform"},
		{"missing ACL probe", strings.Replace(validManifest, `["/usr/bin/getfacl", "-lde"]`, `[]`, 1), "aclProbe"},
		{"relative ACL probe executable", strings.Replace(validManifest, `"/usr/bin/getfacl"`, `"getfacl"`, 1), "aclProbe"},
		{"empty ACL probe argument", strings.Replace(validManifest, `"-lde"`, `""`, 1), "aclProbe"},
		{"NUL ACL probe argument", strings.Replace(validManifest, `"-lde"`, `"\u0000"`, 1), "aclProbe"},
		{"empty path entries", strings.Replace(validManifest, `["/nix/store/fence/bin"]`, `[]`, 1), "pathEntries"},
		{"relative path entry", strings.Replace(validManifest, `"/nix/store/fence/bin"`, `"relative/bin"`, 1), "pathEntries"},
		{"relative immutable path", strings.Replace(validManifest, `"/tmp"`, `"tmp"`, 1), "scratchRoot"},
		{"empty agent program name", strings.Replace(validManifest, `"example-agent"`, `""`, 1), "agent.name"},
		{"unsafe agent program name", strings.Replace(validManifest, `"example-agent"`, `"../agent"`, 1), "agent.name"},
		{"relative agent executable", strings.Replace(validManifest, `"/nix/store/example-agent/bin/example-agent"`, `"example-agent"`, 1), "agent.executable"},
		{"relative agent state path", strings.Replace(validManifest, `"/home/user/.example"`, `".example"`, 1), "agent.defaultStatePaths"},
		{"relative container socket", strings.Replace(validManifest, `"socketPath": null`, `"socketPath": "relative-socket"`, 1), "docker.socketPath"},
		{"relative container client", strings.Replace(validManifest, `"clientPrograms": []`, `"clientPrograms": ["docker"]`, 1), "docker.clientPrograms"},
		{"unknown field", strings.Replace(validManifest, `"version": 1,`, `"version": 1, "unexpected": true,`, 1), "manifest"},
		{"second JSON value", validManifest + ` {}`, "manifest"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Load(writeManifest(t, test.json))
			if err == nil {
				t.Fatal("Load() error = nil")
			}
			if !strings.Contains(err.Error(), test.field) {
				t.Fatalf("Load() error = %q, want field %q", err, test.field)
			}
			if strings.Contains(err.Error(), "relative-socket") || strings.Contains(err.Error(), "../agent") {
				t.Fatalf("Load() error disclosed a manifest value: %q", err)
			}
		})
	}
}

func writeManifest(t *testing.T, content string) string {
	t.Helper()
	path := t.TempDir() + "/manifest.json"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return path
}
