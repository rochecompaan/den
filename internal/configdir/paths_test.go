package configdir

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSelectTreatsWildcardCharactersInHomeAsLiteral(t *testing.T) {
	root := t.TempDir()
	home := privateDir(t, root, "home[1]")
	candidate := privateDir(t, home, "worktree-state")

	selection, err := Select(&candidate, nil, home, []string{"~/.ssh/id_*"}, safeDependencies(t))
	if err != nil {
		t.Fatalf("Select() rejected disjoint state because HOME contains glob syntax: %v", err)
	}
	if selection.CanonicalPath != candidate {
		t.Fatalf("CanonicalPath = %q, want %q", selection.CanonicalPath, candidate)
	}
}

func TestPathsOverlapConservativelyFoldsProspectiveCaseAliases(t *testing.T) {
	protected := filepath.Join(string(os.PathSeparator), "Users", "owner", ".ssh")
	candidate := filepath.Join(string(os.PathSeparator), "Users", "owner", ".SSH", "state")
	if !pathsOverlapWithCase(candidate, protected, true) {
		t.Fatal("pathsOverlapWithCase() missed a case-insensitive prospective descendant")
	}
	if pathsOverlapWithCase(candidate, protected, false) {
		t.Fatal("pathsOverlapWithCase() weakened case-sensitive Linux semantics")
	}
}

func TestPathsOverlapUsesExistingComponentIdentity(t *testing.T) {
	root := t.TempDir()
	real := privateDir(t, root, "real")
	child := privateDir(t, real, "child")
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(real, alias); err != nil {
		t.Fatal(err)
	}

	if !pathsOverlap(alias, child) {
		t.Fatal("pathsOverlap() missed an ancestor relationship through an existing identity alias")
	}
}

func TestValidateProtectedOverlapRejectsExistingIdentityAlias(t *testing.T) {
	root := t.TempDir()
	home := privateDir(t, root, "home")
	protected := filepath.Join(root, "protected")
	alias := filepath.Join(root, "identity-alias")
	if err := os.WriteFile(protected, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(protected, alias); err != nil {
		t.Fatal(err)
	}

	if err := validateProtectedOverlap(alias, home, []string{protected}); err == nil {
		t.Fatal("validateProtectedOverlap() accepted an existing filesystem identity alias")
	}
}
