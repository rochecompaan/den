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

func TestLoadAcceptsRelativeExplicitConfigDir(t *testing.T) {
	manifest, err := Load(writeManifest(t, strings.Replace(validManifest, `"explicitConfigDir": null`, `"explicitConfigDir": ".claude"`, 1)))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if manifest.ExplicitConfigDir == nil || *manifest.ExplicitConfigDir != ".claude" {
		t.Fatalf("explicit config directory = %#v, want .claude", manifest.ExplicitConfigDir)
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
		{"relative Docker socket", strings.Replace(validManifest, "\"docker\": {\n    \"enable\": false,\n    \"socketPath\": null", "\"docker\": {\n    \"enable\": false,\n    \"socketPath\": \"relative-docker-socket\"", 1), "docker.socketPath"},
		{"relative Docker client", strings.Replace(validManifest, "\"docker\": {\n    \"enable\": false,\n    \"socketPath\": null,\n    \"hostPorts\": [],\n    \"clientPrograms\": []", "\"docker\": {\n    \"enable\": false,\n    \"socketPath\": null,\n    \"hostPorts\": [],\n    \"clientPrograms\": [\"docker\"]", 1), "docker.clientPrograms"},
		{"relative Podman socket", strings.Replace(validManifest, "\"podman\": {\n    \"enable\": false,\n    \"socketPath\": null", "\"podman\": {\n    \"enable\": false,\n    \"socketPath\": \"relative-podman-socket\"", 1), "podman.socketPath"},
		{"relative Podman client", strings.Replace(validManifest, "\"podman\": {\n    \"enable\": false,\n    \"socketPath\": null,\n    \"hostPorts\": [],\n    \"clientPrograms\": []", "\"podman\": {\n    \"enable\": false,\n    \"socketPath\": null,\n    \"hostPorts\": [],\n    \"clientPrograms\": [\"podman\"]", 1), "podman.clientPrograms"},
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
		})
	}
}

func TestLoadRedactsRejectedValues(t *testing.T) {
	tests := []struct {
		name  string
		field string
		json  func(string) string
	}{
		{"fence executable", "fenceExecutable", func(s string) string {
			return strings.Replace(validManifest, `"/nix/store/fence/bin/fence"`, `"`+s+`"`, 1)
		}},
		{"RepoWolf client", "repoWolfClientDir", func(s string) string {
			return strings.Replace(validManifest, `"/nix/store/repowolf-client"`, `"`+s+`"`, 1)
		}},
		{"base policy", "basePolicy", func(s string) string {
			return strings.Replace(validManifest, `"/nix/store/policy/fence.json"`, `"`+s+`"`, 1)
		}},
		{"closure paths file", "closurePathsFile", func(s string) string {
			return strings.Replace(validManifest, `"/nix/store/closure-paths"`, `"`+s+`"`, 1)
		}},
		{"platform", "platform", func(s string) string { return strings.Replace(validManifest, `"linux"`, `"`+s+`"`, 1) }},
		{"ACL probe executable", "aclProbe", func(s string) string { return strings.Replace(validManifest, `"/usr/bin/getfacl"`, `"`+s+`"`, 1) }},
		{"ACL probe argument", "aclProbe", func(s string) string { return strings.Replace(validManifest, `"-lde"`, `"`+s+`\u0000"`, 1) }},
		{"path entry", "pathEntries", func(s string) string { return strings.Replace(validManifest, `"/nix/store/fence/bin"`, `"`+s+`"`, 1) }},
		{"agent name", "agent.name", func(s string) string { return strings.Replace(validManifest, `"example-agent"`, `"unsafe/`+s+`"`, 1) }},
		{"agent executable", "agent.executable", func(s string) string {
			return strings.Replace(validManifest, `"/nix/store/example-agent/bin/example-agent"`, `"`+s+`"`, 1)
		}},
		{"agent state path", "agent.defaultStatePaths", func(s string) string { return strings.Replace(validManifest, `"/home/user/.example"`, `"`+s+`"`, 1) }},
		{"Docker socket", "docker.socketPath", func(s string) string {
			return strings.Replace(validManifest, "\"docker\": {\n    \"enable\": false,\n    \"socketPath\": null", "\"docker\": {\n    \"enable\": false,\n    \"socketPath\": \""+s+"\"", 1)
		}},
		{"Docker client", "docker.clientPrograms", func(s string) string {
			return strings.Replace(validManifest, "\"docker\": {\n    \"enable\": false,\n    \"socketPath\": null,\n    \"hostPorts\": [],\n    \"clientPrograms\": []", "\"docker\": {\n    \"enable\": false,\n    \"socketPath\": null,\n    \"hostPorts\": [],\n    \"clientPrograms\": [\""+s+"\"]", 1)
		}},
		{"Podman socket", "podman.socketPath", func(s string) string {
			return strings.Replace(validManifest, "\"podman\": {\n    \"enable\": false,\n    \"socketPath\": null", "\"podman\": {\n    \"enable\": false,\n    \"socketPath\": \""+s+"\"", 1)
		}},
		{"Podman client", "podman.clientPrograms", func(s string) string {
			return strings.Replace(validManifest, "\"podman\": {\n    \"enable\": false,\n    \"socketPath\": null,\n    \"hostPorts\": [],\n    \"clientPrograms\": []", "\"podman\": {\n    \"enable\": false,\n    \"socketPath\": null,\n    \"hostPorts\": [],\n    \"clientPrograms\": [\""+s+"\"]", 1)
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sentinel := "redaction-sentinel-" + strings.ReplaceAll(test.name, " ", "-")
			_, err := Load(writeManifest(t, test.json(sentinel)))
			if err == nil {
				t.Fatal("Load() error = nil")
			}
			if !strings.Contains(err.Error(), test.field) {
				t.Fatalf("Load() error = %q, want field %q", err, test.field)
			}
			if strings.Contains(err.Error(), sentinel) {
				t.Fatalf("Load() error disclosed manifest value %q: %q", sentinel, err)
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
