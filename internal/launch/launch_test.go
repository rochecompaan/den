package launch

import (
	"bytes"
	"context"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/rochecompaan/den/internal/environment"
	"github.com/rochecompaan/den/internal/manifest"
)

func TestRunRejectsInvalidRepoWolfInputsWithoutLeakingValues(t *testing.T) {
	values := map[string]string{
		"REPOWOLF_ENDPOINT":    "https://secret.example.test/no",
		"REPOWOLF_TOKEN":       "rw1_secret-token",
		"REPOWOLF_CA_FILE":     "/secret/ca.pem",
		"REPOWOLF_SERVER_NAME": "secret-server",
	}
	var stderr bytes.Buffer
	got := run(
		context.Background(),
		manifest.Manifest{},
		nil,
		lookup(values),
		func(string) (fs.FileInfo, error) { return launchFileInfo{}, nil },
		func() []string { return nil },
		func([]string, environment.Controlled) []string {
			t.Fatal("environment builder was called")
			return nil
		},
		&stderr,
	)
	if got != 1 {
		t.Fatalf("run() = %d, want 1", got)
	}
	if !strings.Contains(stderr.String(), "REPOWOLF_ENDPOINT") {
		t.Fatalf("stderr = %q, want endpoint field", stderr.String())
	}
	for _, secret := range values {
		if strings.Contains(stderr.String(), secret) {
			t.Fatalf("stderr leaked %q: %q", secret, stderr.String())
		}
	}
}

func TestRunBuildsControlledEnvironmentAfterValidation(t *testing.T) {
	values := map[string]string{
		"REPOWOLF_ENDPOINT": "https://broker.example.test/",
		"REPOWOLF_TOKEN":    "rw1_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"REPOWOLF_CA_FILE":  "/canonical/ca.pem",
		"HOME":              "/home/tester",
	}
	launcherManifest := manifest.Manifest{
		RepoWolfClientDir: "/nix/store/repowolf-client",
		PathEntries:       []string{"/nix/store/git/bin", "/nix/store/coreutils/bin"},
	}
	var gotHost []string
	var gotControlled environment.Controlled
	got := run(
		context.Background(),
		launcherManifest,
		nil,
		lookup(values),
		func(string) (fs.FileInfo, error) { return launchFileInfo{mode: 0o444}, nil },
		func() []string { return []string{"KEEP=value", "REPOWOLF_SERVER_NAME=blocked"} },
		func(host []string, controlled environment.Controlled) []string {
			gotHost = host
			gotControlled = controlled
			return []string{"next-stage=value"}
		},
		&bytes.Buffer{},
	)
	if got != 0 {
		t.Fatalf("run() = %d, want 0", got)
	}
	if !reflect.DeepEqual(gotHost, []string{"KEEP=value", "REPOWOLF_SERVER_NAME=blocked"}) {
		t.Fatalf("host = %#v", gotHost)
	}
	want := environment.Controlled{
		Endpoint:    values["REPOWOLF_ENDPOINT"],
		Token:       values["REPOWOLF_TOKEN"],
		CAFile:      values["REPOWOLF_CA_FILE"],
		ClientDir:   launcherManifest.RepoWolfClientDir,
		PathEntries: launcherManifest.PathEntries,
	}
	if !reflect.DeepEqual(gotControlled, want) {
		t.Fatalf("controlled = %#v, want %#v", gotControlled, want)
	}
}

func TestRunRejectsClaudeReservedArgumentsBeforeBuildingEnvironment(t *testing.T) {
	values := map[string]string{
		"REPOWOLF_ENDPOINT": "https://broker.example.test/",
		"REPOWOLF_TOKEN":    "rw1_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"REPOWOLF_CA_FILE":  "/canonical/ca.pem",
		"HOME":              t.TempDir(),
	}
	called := false
	got := run(context.Background(), manifest.Manifest{Agent: manifest.Agent{Name: "claude"}}, []string{"--settings=/tmp/override.json"},
		lookup(values), func(string) (fs.FileInfo, error) { return launchFileInfo{mode: 0o444}, nil },
		func() []string { return nil }, func([]string, environment.Controlled) []string { called = true; return nil }, &bytes.Buffer{})
	if got != 1 || called {
		t.Fatalf("run() = %d, environment builder called = %t", got, called)
	}
}

func TestRunRejectsDarwinBareBeforeBuildingEnvironment(t *testing.T) {
	values := map[string]string{
		"REPOWOLF_ENDPOINT": "https://broker.example.test/",
		"REPOWOLF_TOKEN":    "rw1_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"REPOWOLF_CA_FILE":  "/canonical/ca.pem",
		"HOME":              t.TempDir(),
	}
	called := false
	got := run(context.Background(), manifest.Manifest{Platform: "darwin", Agent: manifest.Agent{Name: "claude"}}, []string{"--bare=true"},
		lookup(values), func(string) (fs.FileInfo, error) { return launchFileInfo{mode: 0o444}, nil },
		func() []string { return nil }, func([]string, environment.Controlled) []string { called = true; return nil }, &bytes.Buffer{})
	if got != 1 || called {
		t.Fatalf("run() = %d, environment builder called = %t", got, called)
	}
}

func TestRunScrubsClaudeCodeSimpleOnlyOnDarwin(t *testing.T) {
	fenceExecutable := filepath.Join(t.TempDir(), "fence")
	if err := os.WriteFile(fenceExecutable, []byte("#!/bin/sh\nprintf 'Linux Sandbox Features:\\n\\n  Capability         Required For              Status       Details\\n  -----------------  ------------------------  -----------  -------\\n  Network namespace  direct network isolation  ok           available\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	values := map[string]string{
		"REPOWOLF_ENDPOINT": "https://broker.example.test/",
		"REPOWOLF_TOKEN":    "rw1_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"REPOWOLF_CA_FILE":  "/canonical/ca.pem",
		"HOME":              t.TempDir(),
	}
	for _, platform := range []string{"darwin", "linux"} {
		t.Run(platform, func(t *testing.T) {
			var host []string
			got := run(context.Background(), manifest.Manifest{Platform: platform, FenceExecutable: fenceExecutable, Agent: manifest.Agent{Name: "claude"}}, nil,
				lookup(values), func(string) (fs.FileInfo, error) { return launchFileInfo{mode: 0o444}, nil },
				func() []string { return []string{"CLAUDE_CODE_SIMPLE=1"} },
				func(gotHost []string, _ environment.Controlled) []string { host = gotHost; return nil }, &bytes.Buffer{})
			if got != 0 {
				t.Fatalf("run() = %d, want 0", got)
			}
			contains := false
			for _, entry := range host {
				contains = contains || entry == "CLAUDE_CODE_SIMPLE=1"
			}
			if contains != (platform == "linux") {
				t.Fatalf("host = %#v", host)
			}
		})
	}
}

func TestRunSelectsCustomConfigurationAfterRepoWolfAndRollsItBack(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	if err := os.Mkdir(home, 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "claude-config")
	probe := filepath.Join(root, "acl-probe")
	output := "user::rwx\\ngroup::---\\nother::---\\n"
	if runtime.GOOS == "darwin" {
		output = ""
	}
	if err := os.WriteFile(probe, []byte("#!/bin/sh\nprintf '"+output+"'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	values := map[string]string{
		"REPOWOLF_ENDPOINT": "https://broker.example.test/",
		"REPOWOLF_TOKEN":    "rw1_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"REPOWOLF_CA_FILE":  "/canonical/ca.pem",
		"HOME":              home,
	}
	launcherManifest := manifest.Manifest{
		RepoWolfClientDir: "/nix/store/repowolf-client",
		ACLProbe:          []string{probe},
		ExplicitConfigDir: &configPath,
	}
	got := run(
		context.Background(), launcherManifest, nil, lookup(values),
		func(string) (fs.FileInfo, error) { return launchFileInfo{mode: 0o444}, nil },
		func() []string { return nil },
		func(host []string, controlled environment.Controlled) []string { return nil },
		&bytes.Buffer{},
	)
	if got != 0 {
		t.Fatalf("run() = %d, want 0", got)
	}
	if _, err := os.Lstat(configPath); !os.IsNotExist(err) {
		t.Fatalf("placeholder launch retained created config directory: %v", err)
	}
}

func TestRunFailsClosedWhenLinuxFenceNetworkNamespaceIsUnavailable(t *testing.T) {
	root := t.TempDir()
	probe := filepath.Join(root, "fence")
	output := "Linux Sandbox Features:\n\n  Capability         Required For              Status       Details\n  -----------------  ------------------------  -----------  -------\n  Network namespace  direct network isolation  unavailable  denied-secret-detail\n"
	if err := os.WriteFile(probe, []byte("#!/bin/sh\nprintf '%s' '"+output+"'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	values := map[string]string{
		"REPOWOLF_ENDPOINT": "https://broker.example.test/",
		"REPOWOLF_TOKEN":    "rw1_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"REPOWOLF_CA_FILE":  "/canonical/ca.pem",
		"HOME":              root,
	}
	var stderr bytes.Buffer
	got := run(context.Background(), manifest.Manifest{Platform: "linux", FenceExecutable: probe}, nil,
		lookup(values), func(string) (fs.FileInfo, error) { return launchFileInfo{mode: 0o444}, nil },
		func() []string { return nil },
		func([]string, environment.Controlled) []string {
			t.Fatal("environment builder called after failed preflight")
			return nil
		},
		&stderr,
	)
	if got != 1 {
		t.Fatalf("run() = %d, want 1", got)
	}
	if !strings.Contains(stderr.String(), "network namespace") {
		t.Fatalf("stderr = %q", stderr.String())
	}
	if strings.Contains(stderr.String(), "denied-secret-detail") {
		t.Fatalf("probe details leaked: %q", stderr.String())
	}
}

func TestRunRollsBackConfigurationWhenContainerSocketValidationFails(t *testing.T) {
	root := t.TempDir()
	probe := filepath.Join(root, "acl-probe")
	if err := os.WriteFile(probe, []byte("#!/bin/sh\nprintf 'user::rwx\\ngroup::---\\nother::---\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "claude-config")
	values := map[string]string{
		"REPOWOLF_ENDPOINT": "https://broker.example.test/", "REPOWOLF_TOKEN": "rw1_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"REPOWOLF_CA_FILE": "/canonical/ca.pem", "HOME": root,
	}
	called := false
	got := run(context.Background(), manifest.Manifest{
		ACLProbe: probeSlice(probe), ExplicitConfigDir: &configPath,
		Docker: manifest.ContainerConfig{Enable: true, SocketPath: stringPointer(filepath.Join(root, "missing.sock"))},
	}, nil, lookup(values), func(string) (fs.FileInfo, error) { return launchFileInfo{mode: 0o444}, nil },
		func() []string { return nil }, func([]string, environment.Controlled) []string { called = true; return nil }, &bytes.Buffer{})
	if got != 1 || called {
		t.Fatalf("run() = %d, build called = %t", got, called)
	}
	if _, err := os.Lstat(configPath); !os.IsNotExist(err) {
		t.Fatalf("socket failure retained created config directory: %v", err)
	}
}

func TestRunAddsOnlyValidatedContainerEnvironment(t *testing.T) {
	root := t.TempDir()
	socketPath := filepath.Join(root, "docker.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	values := map[string]string{
		"REPOWOLF_ENDPOINT": "https://broker.example.test/", "REPOWOLF_TOKEN": "rw1_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"REPOWOLF_CA_FILE": "/canonical/ca.pem", "HOME": root,
	}
	var got environment.Controlled
	result := run(context.Background(), manifest.Manifest{Docker: manifest.ContainerConfig{Enable: true, SocketPath: &socketPath}}, nil,
		lookup(values), func(string) (fs.FileInfo, error) { return launchFileInfo{mode: 0o444}, nil },
		func() []string { return []string{"DOCKER_HOST=tcp://untrusted.example.test"} },
		func(_ []string, controlled environment.Controlled) []string { got = controlled; return nil }, &bytes.Buffer{})
	if result != 0 || got.DockerHost != "unix://"+socketPath {
		t.Fatalf("run() = %d, controlled = %#v", result, got)
	}
}

func TestFenceTemporaryEnvironmentReplacesInheritedTemporaryState(t *testing.T) {
	host := []string{
		"KEEP=value",
		"TMPDIR=/host/tmp",
		"DEN_FENCE_TMPDIR=/attacker/tmp",
	}
	got := fenceTemporaryEnvironment(host, "/private/scratch")
	want := map[string]string{
		"KEEP":             "value",
		"TMPDIR":           "/private/scratch",
		"DEN_FENCE_TMPDIR": "/private/scratch",
	}
	values := make(map[string]string)
	for _, entry := range got {
		name, value, ok := strings.Cut(entry, "=")
		if !ok {
			t.Fatalf("malformed environment entry %q", entry)
		}
		if _, duplicate := values[name]; duplicate {
			t.Fatalf("duplicate environment variable %q", name)
		}
		values[name] = value
	}
	if !reflect.DeepEqual(values, want) {
		t.Fatalf("environment = %#v, want %#v", values, want)
	}
}

func lookup(values map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) { value, ok := values[name]; return value, ok }
}

func stringPointer(value string) *string { return &value }

func probeSlice(probe string) []string { return []string{probe} }

type launchFileInfo struct{ mode fs.FileMode }

func (f launchFileInfo) Name() string       { return "ca.pem" }
func (f launchFileInfo) Size() int64        { return 0 }
func (f launchFileInfo) Mode() fs.FileMode  { return f.mode }
func (f launchFileInfo) ModTime() time.Time { return time.Time{} }
func (f launchFileInfo) IsDir() bool        { return f.mode.IsDir() }
func (f launchFileInfo) Sys() any           { return nil }
