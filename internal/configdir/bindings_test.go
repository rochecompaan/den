package configdir

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rochecompaan/den/internal/manifest"
)

func TestPlanBindingsRejectsOverlapsBeforeOpeningDirectories(t *testing.T) {
	root := t.TempDir()
	home := privateDir(t, root, "home")
	parent := filepath.Join(root, "state")
	child := filepath.Join(parent, "child")

	for _, specs := range [][]manifest.StateBinding{
		{{Name: "one", ExplicitPath: &parent, Exports: []manifest.StateExport{{Kind: "environment", Name: "ONE"}}}, {Name: "two", ExplicitPath: &child, Exports: []manifest.StateExport{{Kind: "environment", Name: "TWO"}}}},
		{{Name: "two", ExplicitPath: &child, Exports: []manifest.StateExport{{Kind: "environment", Name: "TWO"}}}, {Name: "one", ExplicitPath: &parent, Exports: []manifest.StateExport{{Kind: "environment", Name: "ONE"}}}},
	} {
		if _, err := PlanBindings(specs, nil, home); err == nil {
			t.Fatal("PlanBindings() accepted overlapping paths")
		}
		if _, err := os.Lstat(parent); !os.IsNotExist(err) {
			t.Fatalf("planning created state: %v", err)
		}
	}
}

func TestPlanBindingsDefaultAndCustomDenySets(t *testing.T) {
	root := t.TempDir()
	home := privateDir(t, root, "home")
	custom := filepath.Join(root, "custom")
	defaultOne := filepath.Join(home, ".one")
	defaultTwo := filepath.Join(home, ".two")
	specs := []manifest.StateBinding{
		{Name: "one", DefaultPath: defaultOne, DefaultWritablePaths: []string{defaultOne + string(os.PathSeparator)}, Exports: []manifest.StateExport{{Kind: "environment", Name: "ONE", ExportDefault: true}}},
		{Name: "two", ExplicitPath: &custom, DefaultPath: defaultTwo, DefaultWritablePaths: []string{defaultTwo + string(os.PathSeparator)}, Exports: []manifest.StateExport{{Kind: "environment", Name: "TWO", ExportDefault: true}}},
	}
	plan, err := PlanBindings(specs, nil, home)
	if err != nil {
		t.Fatalf("PlanBindings() error = %v", err)
	}
	if got, want := plan.WritablePaths(), []string{defaultOne + string(os.PathSeparator)}; !sameStrings(got, want) {
		t.Fatalf("WritablePaths() = %#v, want %#v", got, want)
	}
	if got, want := plan.DeniedWritePaths(), []string{defaultTwo + string(os.PathSeparator)}; !sameStrings(got, want) {
		t.Fatalf("DeniedWritePaths() = %#v, want %#v", got, want)
	}
}

func TestPlanBindingsResolvesRelativeDefaultAndRejectsEmptyInherited(t *testing.T) {
	root := t.TempDir()
	home := privateDir(t, root, "home")
	spec := manifest.StateBinding{
		Name: "agent", InheritedEnvironment: "PI_CODING_AGENT_DIR",
		DefaultPath: ".local/state/den/pi/agent",
		Exports:     []manifest.StateExport{{Kind: "environment", Name: "PI_CODING_AGENT_DIR", ExportDefault: true}},
	}
	plan, err := PlanBindings([]manifest.StateBinding{spec}, nil, home)
	if err != nil {
		t.Fatalf("PlanBindings() error = %v", err)
	}
	want := filepath.Join(home, ".local/state/den/pi/agent")
	if got := plan.WritablePaths(); !sameStrings(got, []string{want + string(os.PathSeparator)}) {
		t.Fatalf("WritablePaths() = %#v, want %q", got, want+string(os.PathSeparator))
	}
	if _, err := PlanBindings([]manifest.StateBinding{spec}, map[string]string{"PI_CODING_AGENT_DIR": ""}, home); err == nil {
		t.Fatal("PlanBindings() accepted an empty inherited directory")
	}
}

func TestPlanBindingsMixedDefaultsAndCustomsInEitherOrder(t *testing.T) {
	root := t.TempDir()
	home := privateDir(t, root, "home")
	agentDefault := filepath.Join(home, ".agent")
	sessionDefault := filepath.Join(home, ".sessions")
	agentCustom := filepath.Join(root, "agent-custom")
	sessionCustom := filepath.Join(root, "session-custom")
	binding := func(name, defaultPath string, custom *string) manifest.StateBinding {
		return manifest.StateBinding{Name: name, ExplicitPath: custom, DefaultPath: defaultPath, DefaultWritablePaths: []string{defaultPath + string(os.PathSeparator)}, Exports: []manifest.StateExport{{Kind: "environment", Name: "STATE", ExportDefault: true}}}
	}
	for _, specs := range [][]manifest.StateBinding{
		{binding("agent", agentDefault, nil), binding("sessions", sessionDefault, &sessionCustom)},
		{binding("sessions", sessionDefault, &sessionCustom), binding("agent", agentDefault, nil)},
		{binding("agent", agentDefault, &agentCustom), binding("sessions", sessionDefault, nil)},
		{binding("sessions", sessionDefault, nil), binding("agent", agentDefault, &agentCustom)},
	} {
		plan, err := PlanBindings(specs, nil, home)
		if err != nil {
			t.Fatal(err)
		}
		if len(plan.WritablePaths()) != 1 || len(plan.DeniedWritePaths()) != 1 {
			t.Fatalf("mixed plan grants=%#v denies=%#v", plan.WritablePaths(), plan.DeniedWritePaths())
		}
	}
}

func TestBindingPlanDefaultRetainsProtectedPaths(t *testing.T) {
	root := t.TempDir()
	home := privateDir(t, root, "home")
	plan, err := PlanBindings([]manifest.StateBinding{{
		Name: "config", Exports: []manifest.StateExport{{Kind: "environment", Name: "CONFIG"}},
	}}, nil, home)
	if err != nil {
		t.Fatal(err)
	}
	handles, err := plan.Open("linux", ACLValidator{
		ProtectedHomes:        []string{home},
		ProtectedPathPatterns: []string{"~/.ssh/id_*"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := handles[0].ProtectedPaths, []string{filepath.Join(home, ".ssh", "id_*")}; !sameStrings(got, want) {
		t.Fatalf("ProtectedPaths = %#v, want %#v", got, want)
	}
}

func TestBindingPlanRollsBackEveryCreatedDirectoryOnLaterFailure(t *testing.T) {
	root := t.TempDir()
	home := privateDir(t, root, "home")
	first, second, invalid := filepath.Join(root, "first"), filepath.Join(root, "second"), filepath.Join(root, "invalid")
	if err := os.WriteFile(invalid, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	specs := []manifest.StateBinding{
		{Name: "first", ExplicitPath: &first, Exports: []manifest.StateExport{{Kind: "environment", Name: "FIRST"}}},
		{Name: "second", ExplicitPath: &second, Exports: []manifest.StateExport{{Kind: "environment", Name: "SECOND"}}},
		{Name: "invalid", ExplicitPath: &invalid, Exports: []manifest.StateExport{{Kind: "environment", Name: "INVALID"}}},
	}
	plan, err := PlanBindings(specs, nil, home)
	if err != nil {
		t.Fatal(err)
	}
	probe := filepath.Join(t.TempDir(), "acl-probe")
	if err := os.WriteFile(probe, []byte("#!/bin/sh\nprintf 'user::rwx\\ngroup::---\\nother::---\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := plan.Open("linux", ACLValidator{ACLProbe: []string{probe}}); err == nil {
		t.Fatal("Open() accepted non-directory binding")
	}
	for _, path := range []string{first, second} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("created directory %q remains after rollback: %v", path, err)
		}
	}
}

func TestPlanBindingsRejectsFinalSymlinkAndAllowsSiblingPrefix(t *testing.T) {
	root := t.TempDir()
	home := privateDir(t, root, "home")
	target := privateDir(t, root, "target")
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := PlanBindings([]manifest.StateBinding{{Name: "link", ExplicitPath: &link, Exports: []manifest.StateExport{{Kind: "environment", Name: "LINK"}}}}, nil, home); err == nil {
		t.Fatal("PlanBindings() accepted a final symlink")
	}
	left, right := filepath.Join(root, "state"), filepath.Join(root, "state-other")
	if _, err := PlanBindings([]manifest.StateBinding{{Name: "left", ExplicitPath: &left, Exports: []manifest.StateExport{{Kind: "environment", Name: "LEFT"}}}, {Name: "right", ExplicitPath: &right, Exports: []manifest.StateExport{{Kind: "environment", Name: "RIGHT"}}}}, nil, home); err != nil {
		t.Fatalf("PlanBindings() rejected sibling prefixes: %v", err)
	}
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
