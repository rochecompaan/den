package configdir

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/rochecompaan/den/internal/manifest"
)

func TestSelectAcceptsWritableAncestorsInsidePrivateBoundary(t *testing.T) {
	for _, test := range []struct {
		name string
		mode os.FileMode
		acl  bool
	}{
		{"group writable", 0o775, false},
		{"world writable", 0o777, false},
		{"writable ACL", 0o755, true},
	} {
		for _, existing := range []bool{false, true} {
			name := test.name + "/created state"
			if existing {
				name = test.name + "/existing state"
			}
			t.Run(name, func(t *testing.T) {
				root := privateBoundaryTestRoot(t)
				home := privateDir(t, root, "home")
				projects := privateDir(t, home, "projects")
				chmodDirectory(t, projects, test.mode)
				workspace := privateDir(t, projects, "workspace")
				chmodDirectory(t, workspace, 0o775)
				path := filepath.Join(workspace, "agent-state")
				if existing {
					privateDir(t, workspace, "agent-state")
				}
				deps := safeDependencies(t)
				if test.acl {
					deps = targetedDependencies(t, projects, unsafeWriteACL(t), safeACL(t))
				}

				selection, err := Select(&path, nil, home, nil, deps)
				if err != nil {
					t.Fatalf("Select() rejected writable ancestor behind private home: %v", err)
				}
				if selection.CanonicalPath != path {
					t.Fatalf("CanonicalPath = %q, want %q", selection.CanonicalPath, path)
				}
				assertDirectoryMode(t, path, 0o700)
				assertDirectoryMode(t, projects, test.mode)
				assertDirectoryMode(t, workspace, 0o775)
				if err := selection.Revalidate(); err != nil {
					t.Fatalf("Revalidate() rejected unchanged private layout: %v", err)
				}
			})
		}
	}
}

func TestBindingPlanCreatesDefaultStateInsidePrivateBoundary(t *testing.T) {
	root := privateBoundaryTestRoot(t)
	home := privateDir(t, root, "home")
	projects := privateDir(t, home, "projects")
	chmodDirectory(t, projects, 0o775)
	path := filepath.Join(projects, "den", "agent")
	plan, err := PlanBindings([]manifest.StateBinding{{
		Name: "agent", DefaultPath: path,
		Exports: []manifest.StateExport{{Kind: "environment", Name: "AGENT"}},
	}}, nil, home)
	if err != nil {
		t.Fatal(err)
	}
	handles, err := plan.Open(runtime.GOOS, safeDependencies(t))
	if err != nil {
		t.Fatalf("Open() rejected default state behind private home: %v", err)
	}
	t.Cleanup(func() {
		for _, handle := range handles {
			if err := handle.Close(); err != nil {
				t.Errorf("Close() error = %v", err)
			}
		}
	})
	assertDirectoryMode(t, filepath.Dir(path), 0o700)
	assertDirectoryMode(t, path, 0o700)
	assertDirectoryMode(t, projects, 0o775)
	if err := handles[0].Revalidate(); err != nil {
		t.Fatalf("Revalidate() error = %v", err)
	}
}

func TestSelectRejectsWritableAncestorAbovePrivateBoundary(t *testing.T) {
	root := unprotectedTestRoot(t)
	shared := privateDir(t, root, "shared")
	chmodDirectory(t, shared, 0o775)
	home := privateDir(t, shared, "home")
	path := privateDir(t, home, "agent-state")
	if _, err := Select(&path, nil, home, nil, safeDependencies(t)); err == nil {
		t.Fatal("Select() accepted a private boundary replaceable through its writable parent")
	}
}

func TestSelectDoesNotTrustBoundaryWithNonOwnerAccess(t *testing.T) {
	root := unprotectedTestRoot(t)
	home := privateDir(t, root, "home")
	projects := privateDir(t, home, "projects")
	chmodDirectory(t, projects, 0o775)
	path := privateDir(t, projects, "agent-state")
	deps := targetedDependencies(t, home, unsafeNonOwnerACL(t), safeACL(t))
	if _, err := Select(&path, nil, home, nil, deps); err == nil {
		t.Fatal("Select() trusted a private-mode boundary with non-owner ACL access")
	}
}

func TestCaptureAncestorsRequiresUserOwnedPrivateBoundary(t *testing.T) {
	root := unprotectedTestRoot(t)
	home := privateDir(t, root, "home")
	projects := privateDir(t, home, "projects")
	chmodDirectory(t, projects, 0o775)
	probe, err := snapshotACLProbe(safeDependencies(t).ACLProbe)
	if err != nil {
		t.Fatal(err)
	}
	// Model an invoking account other than the owner of the private ancestor.
	otherUID := uint32(os.Getuid()) + 1
	if _, err := captureAncestors(projects, otherUID, "other", "other", probe); err == nil {
		t.Fatal("captureAncestors() trusted a private boundary owned by another account")
	}
}

func TestSelectRejectsForeignOwnerAbovePrivateBoundary(t *testing.T) {
	if os.Getuid() != 0 || os.Geteuid() != 0 {
		t.Skip("changing directory ownership requires root")
	}
	root := unprotectedTestRoot(t)
	foreign := privateDir(t, root, "foreign")
	chmodDirectory(t, foreign, 0o755)
	if err := os.Chown(foreign, 1, -1); err != nil {
		t.Fatal(err)
	}
	home := privateDir(t, foreign, "home")
	projects := privateDir(t, home, "projects")
	chmodDirectory(t, projects, 0o775)
	path := privateDir(t, projects, "agent-state")
	if _, err := Select(&path, nil, home, nil, safeDependencies(t)); err == nil {
		t.Fatal("Select() trusted a private boundary replaceable by another parent owner")
	}
}

func TestSelectionRevalidateRejectsChangesInsidePrivateBoundary(t *testing.T) {
	for _, mutation := range []string{"boundary permissions", "ancestor permissions", "ancestor replacement", "state replacement", "state symlink"} {
		t.Run(mutation, func(t *testing.T) {
			root := privateBoundaryTestRoot(t)
			home := privateDir(t, root, "home")
			projects := privateDir(t, home, "projects")
			chmodDirectory(t, projects, 0o775)
			path := privateDir(t, projects, "agent-state")
			selection, err := Select(&path, nil, home, nil, safeDependencies(t))
			if err != nil {
				t.Fatal(err)
			}
			switch mutation {
			case "boundary permissions":
				chmodDirectory(t, home, 0o755)
			case "ancestor permissions":
				chmodDirectory(t, projects, 0o777)
			case "ancestor replacement":
				if err := os.Rename(projects, projects+"-old"); err != nil {
					t.Fatal(err)
				}
				privateDir(t, home, "projects")
				chmodDirectory(t, projects, 0o775)
				privateDir(t, projects, "agent-state")
			case "state replacement", "state symlink":
				if err := os.Rename(path, path+"-old"); err != nil {
					t.Fatal(err)
				}
				if mutation == "state replacement" {
					privateDir(t, projects, "agent-state")
				} else if err := os.Symlink(path+"-old", path); err != nil {
					t.Fatal(err)
				}
			}
			if err := selection.Revalidate(); err == nil {
				t.Fatal("Revalidate() accepted changed directories behind private boundary")
			}
		})
	}
}

// A private t.TempDir() ancestor could protect otherwise unsafe fixtures.
// Find a usable base with no trusted private boundary, including Nix sandboxes
// where /tmp has an unmapped owner and the build root has a foreign owner.
func unprotectedTestRoot(t *testing.T) string {
	t.Helper()
	base := ""
	for _, candidate := range []string{"/tmp", "/var/tmp", os.TempDir()} {
		canonical, err := filepath.EvalSymlinks(candidate)
		if err != nil {
			continue
		}
		trust, ok := fixtureAncestorTrust(canonical)
		if ok && !trust.privateBoundary {
			base = canonical
			break
		}
	}
	if base == "" {
		t.Skip("no usable temporary base without a private boundary")
	}
	root, err := os.MkdirTemp(base, "den-configdir-boundary-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(root); err != nil {
			t.Errorf("RemoveAll() error = %v", err)
		}
	})
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	chmodDirectory(t, root, 0o755)
	return root
}

func privateBoundaryTestRoot(t *testing.T) string {
	t.Helper()
	root := unprotectedTestRoot(t)
	trust, ok := fixtureAncestorTrust(root)
	if !ok || trust.foreignParent {
		t.Skip("private-boundary integration needs root- or user-owned ancestors; permission-policy tests cover foreign namespace roots")
	}
	return root
}

// These fixtures use ACL probes that report no extra grants. Inspect the real
// mode and owner so fixture selection works in host and build namespaces.
func fixtureAncestorTrust(path string) (ancestorTrust, bool) {
	trust := ancestorTrust{ownerUID: uint32(os.Getuid())}
	paths := ancestorPaths(path)
	for index := len(paths) - 1; index >= 0; index-- {
		info, err := os.Lstat(paths[index])
		if err != nil || !info.IsDir() {
			return trust, false
		}
		identity, ok := identityFromFileInfo(info)
		if !ok || trust.check(identity, aclAccess{}) != nil {
			return trust, false
		}
	}
	return trust, true
}

func chmodDirectory(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func assertDirectoryMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() || info.Mode().Perm() != want {
		t.Fatalf("directory mode = %v, want directory with permissions %04o", info.Mode(), want)
	}
}
