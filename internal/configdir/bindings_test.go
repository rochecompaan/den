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
