package configdir

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

func TestSelectAcceptsWritableAncestorsWithNativeLinuxACLs(t *testing.T) {
	probe, err := exec.LookPath("getfacl")
	if err != nil {
		t.Skip("native ACL verification requires getfacl")
	}
	root := privateBoundaryTestRoot(t)
	home := privateDir(t, root, "home")
	projects := privateDir(t, home, "projects")
	chmodDirectory(t, projects, 0o775)
	path := filepath.Join(projects, "agent-state")

	selection, err := Select(&path, nil, home, nil, Dependencies{
		ACLProbe: []string{probe, "--absolute-names", "--omit-header"},
	})
	if err != nil {
		t.Fatalf("Select() rejected real private-home layout: %v", err)
	}
	assertDirectoryMode(t, path, 0o700)
	assertDirectoryMode(t, projects, 0o775)
	if err := selection.Revalidate(); err != nil {
		t.Fatalf("Revalidate() error = %v", err)
	}
}

func TestRevalidationRejectsNativeAncestorACLChangeInsidePrivateBoundary(t *testing.T) {
	probe, err := exec.LookPath("getfacl")
	if err != nil {
		t.Skip("native ACL verification requires getfacl")
	}
	modify, err := exec.LookPath("setfacl")
	if err != nil {
		t.Skip("native ACL mutation requires setfacl")
	}
	root := privateBoundaryTestRoot(t)
	home := privateDir(t, root, "home")
	projects := privateDir(t, home, "projects")
	chmodDirectory(t, projects, 0o775)
	path := privateDir(t, projects, "agent-state")
	selection, err := Select(&path, nil, home, nil, Dependencies{
		ACLProbe: []string{probe, "--absolute-names", "--omit-header"},
	})
	if err != nil {
		t.Fatal(err)
	}
	grant := "u:" + strconv.Itoa(os.Getuid()+1) + ":rwx"
	if output, err := exec.Command(modify, "--modify", grant, projects).CombinedOutput(); err != nil {
		t.Fatalf("setfacl error = %v: %s", err, output)
	}
	// Keep mode and ownership unchanged so only the retained ACL snapshot can
	// reject this grant. Writable ACLs below the private boundary remain allowed.
	assertDirectoryMode(t, projects, 0o775)
	if err := selection.Revalidate(); err == nil {
		t.Fatal("Revalidate() accepted changed ACL below private boundary")
	}
}
