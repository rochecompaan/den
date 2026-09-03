package configdir

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

func TestExpandProtectedPatternsUsesAccountAndRuntimeHomes(t *testing.T) {
	account := filepath.Join(t.TempDir(), "account")
	runtimeHome := filepath.Join(t.TempDir(), "runtime")
	patterns := []string{"~/.ssh/id_*", "~/.aws/**", "~/.gitconfig"}
	got, err := expandProtectedPatterns(patterns, []string{account, runtimeHome, account})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		filepath.Join(account, ".ssh", "id_*"), filepath.Join(runtimeHome, ".ssh", "id_*"),
		filepath.Join(account, ".aws", "**"), filepath.Join(runtimeHome, ".aws", "**"),
		filepath.Join(account, ".gitconfig"), filepath.Join(runtimeHome, ".gitconfig"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expanded paths = %#v, want %#v", got, want)
	}
}

func TestExpandProtectedPatternsEscapesGlobHomesAndProtectsCanonicalAliases(t *testing.T) {
	root := t.TempDir()
	realHome := privateDir(t, root, "home[1]")
	aliasHome := filepath.Join(root, "alias?home")
	if err := os.Symlink(realHome, aliasHome); err != nil {
		t.Fatal(err)
	}

	got, err := expandProtectedPatterns([]string{"~/.ssh/id_*"}, []string{aliasHome})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		filepath.Join(root, `alias\?home`, ".ssh", "id_*"),
		filepath.Join(root, `home\[1\]`, ".ssh", "id_*"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expanded paths = %#v, want %#v", got, want)
	}
}

func TestExpandProtectedPatternsRejectsInvalidAccountHomeWithoutDisclosure(t *testing.T) {
	sentinel := "relative-secret-home"
	_, err := expandProtectedPatterns([]string{"~/.ssh/id_*"}, []string{sentinel})
	if err == nil {
		t.Fatal("expandProtectedPatterns() error = nil")
	}
	if strings.Contains(err.Error(), sentinel) {
		t.Fatalf("error disclosed home: %v", err)
	}
}

func TestProtectedOverlapKeepsInaccessibleAccountCredentialRoots(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory traversal permissions")
	}
	root := t.TempDir()
	runtimeHome := privateDir(t, root, "runtime-home")
	accountHome := privateDir(t, root, "account-home")
	candidate := privateDir(t, root, "state")
	if err := os.Chmod(accountHome, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(accountHome, 0o700) })
	selection, err := Select(&candidate, nil, runtimeHome, []string{"~/.ssh/id_*"}, Dependencies{
		ACLProbe: safeDependencies(t).ACLProbe, ProtectedHomes: []string{accountHome, runtimeHome},
	})
	if err != nil {
		t.Fatalf("Select() rejected an inaccessible disjoint account root: %v", err)
	}
	want := filepath.Join(accountHome, ".ssh", "id_*")
	if !containsPath(selection.ProtectedPaths, want) {
		t.Fatalf("ProtectedPaths = %#v, missing %q", selection.ProtectedPaths, want)
	}
}

func containsPath(paths []string, want string) bool {
	for _, path := range paths {
		if path == want {
			return true
		}
	}
	return false
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
