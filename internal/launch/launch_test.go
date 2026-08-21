package launch

import (
	"bytes"
	"context"
	"io/fs"
	"reflect"
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

func lookup(values map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) { value, ok := values[name]; return value, ok }
}

type launchFileInfo struct{ mode fs.FileMode }

func (f launchFileInfo) Name() string       { return "ca.pem" }
func (f launchFileInfo) Size() int64        { return 0 }
func (f launchFileInfo) Mode() fs.FileMode  { return f.mode }
func (f launchFileInfo) ModTime() time.Time { return time.Time{} }
func (f launchFileInfo) IsDir() bool        { return f.mode.IsDir() }
func (f launchFileInfo) Sys() any           { return nil }
