package launch

import (
	"reflect"
	"testing"

	"github.com/rochecompaan/den/internal/environment"
	"github.com/rochecompaan/den/internal/manifest"
)

func TestBuildChildInputs(t *testing.T) {
	host := []string{
		"KEEP=first", "KEEP=last", "REMOVE=host", "SET=host", "PACKAGE=host",
		"GH_TOKEN=secret", "GIT_SSH=/unsafe", "REPOWOLF_TOKEN=untrusted", "PATH=/host/bin",
	}
	agent := manifest.Agent{
		MandatoryArgs: []string{"--mandatory"}, ResourceArgs: []string{"--extension", "/nix/store/resource.ts"},
		Environment:      manifest.AgentEnvironment{Scrub: []string{"REMOVE", "REPOWOLF_TOKEN"}, Set: map[string]string{"SET": "agent"}},
		PackageDirectory: &manifest.EnvironmentValue{Name: "PACKAGE", Value: "/nix/store/pi"},
		SecurityAdapter:  &manifest.SecurityAdapter{Arguments: []string{"--security", "/nix/store/security.ts"}},
	}
	controlled := environment.Controlled{Endpoint: "https://broker.example.test", Token: "token", CAFile: "/ca.pem", ClientDir: "/client", PathEntries: []string{"/path"}}
	state := StateInputs{Environment: map[string]string{"STATE": "/state"}, Arguments: []string{"--session-dir", "/sessions"}}
	got := BuildChildInputs(host, controlled, agent, state, []string{"--continue", "prompt"})
	if want := []string{"--mandatory", "--security", "/nix/store/security.ts", "--extension", "/nix/store/resource.ts", "--session-dir", "/sessions", "--continue", "prompt"}; !reflect.DeepEqual(got.Arguments, want) {
		t.Fatalf("arguments = %#v, want %#v", got.Arguments, want)
	}
	values := childEnvironmentEntries(got.Environment)
	for name, want := range map[string]string{
		"KEEP": "first", "SET": "agent", "PACKAGE": "/nix/store/pi", "STATE": "/state",
		"REPOWOLF_TOKEN": "token", "PATH": "/path",
	} {
		if values[name] != want {
			t.Errorf("%s = %q, want %q", name, values[name], want)
		}
	}
	for _, name := range []string{"REMOVE", "GH_TOKEN", "GIT_SSH"} {
		if _, ok := values[name]; ok {
			t.Errorf("%s was retained: %#v", name, values)
		}
	}
	if len(values) != len(got.Environment) {
		t.Fatalf("environment has duplicate names: %#v", got.Environment)
	}
}

func childEnvironmentEntries(entries []string) map[string]string {
	values := make(map[string]string, len(entries))
	for _, entry := range entries {
		for index := range entry {
			if entry[index] == '=' {
				values[entry[:index]] = entry[index+1:]
				break
			}
		}
	}
	return values
}
