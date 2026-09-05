package launch

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/rochecompaan/den/internal/configdir"
	"github.com/rochecompaan/den/internal/manifest"
)

func TestLaunchStateBindingsPreservesBindingAndExportOrder(t *testing.T) {
	root := t.TempDir()
	one := privateState(t, root, "one")
	two := privateState(t, root, "two")
	plan, err := configdir.PlanBindings([]manifest.StateBinding{
		{Name: "one", ExplicitPath: &one, Exports: []manifest.StateExport{{Kind: "argument", Name: "--one"}, {Kind: "environment", Name: "ONE"}}},
		{Name: "two", ExplicitPath: &two, Exports: []manifest.StateExport{{Kind: "argument", Name: "--two"}, {Kind: "environment", Name: "TWO"}}},
	}, nil, root)
	if err != nil {
		t.Fatal(err)
	}
	handles, err := plan.Open("linux", configdir.ACLValidator{ACLProbe: []string{writeStateProbe(t)}})
	if err != nil {
		t.Fatal(err)
	}
	defer closeTestStateHandles(handles)
	inputs := StateInputsFrom(handles)
	if got, want := inputs.Arguments, []string{"--one", one, "--two", two}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Arguments = %#v, want %#v", got, want)
	}
	if got, want := inputs.Environment, map[string]string{"ONE": one, "TWO": two}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Environment = %#v, want %#v", got, want)
	}
}

func privateState(t *testing.T, root, name string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeStateProbe(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "acl-probe")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf 'user::rwx\\ngroup::---\\nother::---\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func closeTestStateHandles(handles []*configdir.Handle) {
	for _, handle := range handles {
		_ = handle.Close()
	}
}
