package configdir

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDarwinOwnerWriteDenyOrdering(t *testing.T) {
	owner := currentUserName(t)
	tests := []struct {
		name   string
		acl    string
		denied bool
	}{
		{"owner deny", fmt.Sprintf("0: user:%s deny write\n", owner), true},
		{"owner allow before deny", fmt.Sprintf("0: user:%s allow write\n1: user:%s deny write\n", owner, owner), false},
		{"owner deny before allow", fmt.Sprintf("0: user:%s deny write\n1: user:%s allow write\n", owner, owner), true},
		{"owner read deny", fmt.Sprintf("0: user:%s deny read\n", owner), false},
		{"other owner deny", "0: user:other deny write\n", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			access, _, err := parseDarwinACL([]byte(test.acl), owner, "unused-id")
			if err != nil {
				t.Fatalf("parseDarwinACL() error = %v", err)
			}
			if access.ownerWriteDenied != test.denied {
				t.Fatalf("ownerWriteDenied = %t, want %t", access.ownerWriteDenied, test.denied)
			}
		})
	}
}

func TestDarwinWritePermissionClassification(t *testing.T) {
	for _, permission := range []string{
		"write", "append", "writeattr", "writeextattr", "writesecurity",
		"add_file", "add_subdirectory", "delete", "delete_child", "chown",
	} {
		if !darwinWritePermission(permission) {
			t.Errorf("darwinWritePermission(%q) = false", permission)
		}
	}
	for _, permission := range []string{"read", "execute", "readattr", "readsecurity"} {
		if darwinWritePermission(permission) {
			t.Errorf("darwinWritePermission(%q) = true", permission)
		}
	}
}

func TestSelectACLPrivacy(t *testing.T) {
	ownerName := currentUserName(t)
	tests := []struct {
		name   string
		linux  string
		darwin string
		ok     bool
	}{
		{
			name:   "owner ACL",
			linux:  "user::rwx\ngroup::---\nmask::---\nother::---\n",
			darwin: fmt.Sprintf(" 0: user:%s allow read,write,execute\n", ownerName),
			ok:     true,
		},
		{
			name:   "non-owner user ACL",
			linux:  "user::rwx\nuser:other:r--\ngroup::---\nmask::r--\nother::---\n",
			darwin: " 0: user:other allow read\n",
		},
		{
			name:   "non-owner group ACL",
			linux:  "user::rwx\ngroup::---\ngroup:other:--x\nmask::--x\nother::---\n",
			darwin: " 0: group:other allow execute\n",
		},
		{
			name:   "ineffective masked ACL",
			linux:  "user::rwx\nuser:other:rwx #effective:---\ngroup::---\nmask::---\nother::---\n",
			darwin: " 0: user:other deny read,write,execute\n",
			ok:     true,
		},
		{
			name:   "inherited non-owner ACL",
			linux:  "user::rwx\ngroup::---\nmask::---\nother::---\ndefault:user::rwx\ndefault:user:other:r--\ndefault:mask::r--\ndefault:other::---\n",
			darwin: " 0: group:other inherited allow read\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			home := privateDir(t, root, "home")
			path := privateDir(t, root, "config")
			output := test.linux
			if runtime.GOOS == "darwin" {
				output = test.darwin
			}
			_, err := Select(&path, nil, home, nil, targetedDependencies(t, path, output, safeACL(t)))
			if (err == nil) != test.ok {
				t.Fatalf("Select() error = %v, want success %t", err, test.ok)
			}
		})
	}
}

func TestSelectRejectsWritableAncestorACLExceptSafeStickyAncestor(t *testing.T) {
	for _, test := range []struct {
		name   string
		sticky bool
		ok     bool
	}{
		{"writable ancestor ACL", false, false},
		{"writable ACL on sticky user-owned ancestor", true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			home := privateDir(t, root, "home")
			parent := privateDir(t, root, "parent")
			if test.sticky {
				if err := os.Chmod(parent, 0o700|os.ModeSticky); err != nil {
					t.Fatal(err)
				}
			}
			path := filepath.Join(parent, "config")
			deps := targetedDependencies(t, parent, unsafeWriteACL(t), safeACL(t))
			_, err := Select(&path, nil, home, nil, deps)
			if (err == nil) != test.ok {
				t.Fatalf("Select() error = %v, want success %t", err, test.ok)
			}
		})
	}
}

func TestSnapshotACLProbeRejectsInvalidArguments(t *testing.T) {
	tests := []struct {
		name string
		argv []string
	}{
		{"missing", nil},
		{"relative executable", []string{"getfacl"}},
		{"NUL executable", []string{"/probe\x00bad"}},
		{"empty argument", []string{"/probe", ""}},
		{"NUL argument", []string{"/probe", "bad\x00arg"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := snapshotACLProbe(test.argv); err == nil {
				t.Fatal("snapshotACLProbe() error = nil")
			}
		})
	}
}

func TestSelectionSnapshotsACLProbeArguments(t *testing.T) {
	root := t.TempDir()
	home := privateDir(t, root, "home")
	path := privateDir(t, root, "config")
	safeProbe := writeProbe(t, "cat <<'ACL_EOF'\n"+safeACL(t)+"ACL_EOF")
	unsafeProbe := writeProbe(t, "cat <<'ACL_EOF'\n"+unsafeNonOwnerACL(t)+"ACL_EOF")
	deps := Dependencies{ACLProbe: []string{safeProbe}}
	selection, err := Select(&path, nil, home, nil, deps)
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}

	deps.ACLProbe[0] = unsafeProbe
	if err := selection.Revalidate(); err != nil {
		t.Fatalf("Revalidate() used mutated ACL probe arguments: %v", err)
	}
}

func TestSelectionRevalidateIgnoresProbeDecorationChanges(t *testing.T) {
	root := t.TempDir()
	home := privateDir(t, root, "home")
	path := privateDir(t, root, "config")
	outputFile := filepath.Join(root, "acl-output")
	first, second := decoratedSafeACL(t, "first"), decoratedSafeACL(t, "second")
	if err := os.WriteFile(outputFile, []byte(first), 0o600); err != nil {
		t.Fatal(err)
	}
	probe := writeProbe(t, fmt.Sprintf("cat %s", shellQuote(outputFile)))
	selection, err := Select(&path, nil, home, nil, Dependencies{ACLProbe: []string{probe}})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if err := os.WriteFile(outputFile, []byte(second), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := selection.Revalidate(); err != nil {
		t.Fatalf("Revalidate() rejected unchanged ACL entries: %v", err)
	}
}

func TestSelectionRevalidateDetectsLinuxDefaultACLIdentityChange(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux getfacl format")
	}
	root := t.TempDir()
	home := privateDir(t, root, "home")
	path := privateDir(t, root, "config")
	outputFile := filepath.Join(root, "acl-output")
	if err := os.WriteFile(outputFile, []byte("user::rwx\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	probe := writeProbe(t, fmt.Sprintf("cat %s", shellQuote(outputFile)))
	selection, err := Select(&path, nil, home, nil, Dependencies{ACLProbe: []string{probe}})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if err := os.WriteFile(outputFile, []byte("default:user::rwx\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := selection.Revalidate(); err == nil {
		t.Fatal("Revalidate() accepted access-to-default ACL identity change")
	}
}

func TestSelectionRevalidateDetectsACLChanges(t *testing.T) {
	root := t.TempDir()
	home := privateDir(t, root, "home")
	path := privateDir(t, root, "config")
	outputFile := filepath.Join(root, "acl-output")
	if err := os.WriteFile(outputFile, []byte(safeACL(t)), 0o600); err != nil {
		t.Fatal(err)
	}
	probe := writeProbe(t, fmt.Sprintf("cat %s", shellQuote(outputFile)))
	selection, err := Select(&path, nil, home, nil, Dependencies{ACLProbe: []string{probe}})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if err := os.WriteFile(outputFile, []byte(changedSafeACL(t)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := selection.Revalidate(); err == nil {
		t.Fatal("Revalidate() accepted changed ACL identity")
	}
}

func TestACLErrorsDoNotDiscloseProbeOutputOrCustomPath(t *testing.T) {
	root := t.TempDir()
	home := privateDir(t, root, "home")
	path := privateDir(t, root, "sensitive-config-name")
	sentinel := "sensitive-acl-principal"
	deps := targetedDependencies(t, path, unsafeNonOwnerACLNamed(t, sentinel), safeACL(t))
	_, err := Select(&path, nil, home, nil, deps)
	if err == nil {
		t.Fatal("Select() error = nil")
	}
	for _, secret := range []string{path, sentinel} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("Select() disclosed %q: %v", secret, err)
		}
	}
}

func outputDependencies(t *testing.T, output string) Dependencies {
	t.Helper()
	probe := writeProbe(t, "cat <<'ACL_EOF'\n"+output+"ACL_EOF")
	return Dependencies{ACLProbe: []string{probe}}
}

func targetedDependencies(t *testing.T, target, targetOutput, otherOutput string) Dependencies {
	t.Helper()
	body := fmt.Sprintf(`last=""
for argument in "$@"; do last="$argument"; done
if [ "$last" = %s ]; then
cat <<'TARGET_EOF'
%sTARGET_EOF
else
cat <<'OTHER_EOF'
%sOTHER_EOF
fi`, shellQuote(target), targetOutput, otherOutput)
	return Dependencies{ACLProbe: []string{writeProbe(t, body)}}
}

func writeProbe(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "acl-probe")
	content := "#!/bin/sh\nset -eu\n" + body + "\n"
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func unsafeNonOwnerACL(t *testing.T) string {
	t.Helper()
	return unsafeNonOwnerACLNamed(t, "other")
}

func unsafeNonOwnerACLNamed(t *testing.T, name string) string {
	t.Helper()
	if runtime.GOOS == "darwin" {
		return fmt.Sprintf(" 0: user:%s allow read\n", name)
	}
	return fmt.Sprintf("user::rwx\nuser:%s:r--\ngroup::---\nmask::r--\nother::---\n", name)
}

func unsafeWriteACL(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "darwin" {
		return " 0: group:other allow write\n"
	}
	return "user::rwx\ngroup::---\ngroup:other:-w-\nmask::-w-\nother::---\n"
}

func decoratedSafeACL(t *testing.T, decoration string) string {
	t.Helper()
	if runtime.GOOS == "darwin" {
		return "drwx------ 2 owner group 64 Jan 1 00:00 " + decoration + "\n"
	}
	return "# file: " + decoration + "\n" + safeACL(t)
}

func changedSafeACL(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "darwin" {
		return fmt.Sprintf(" 0: user:%s allow read\n", currentUserName(t))
	}
	return "user::rw-\ngroup::---\nother::---\n"
}

func currentUserName(t *testing.T) string {
	t.Helper()
	current, err := user.Current()
	if err != nil {
		t.Fatalf("user.Current(): %v", err)
	}
	return current.Username
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
