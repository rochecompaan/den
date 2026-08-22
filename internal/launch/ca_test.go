package launch

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestPrepareCAFilePinsRegularFileInPrivatePolicyDirectory(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.pem")
	policyDir := filepath.Join(root, "policy")
	if err := os.Mkdir(policyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("pinned certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	prepared, err := prepareCAFile(source, policyDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(source, source+".old"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("swapped certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(prepared)
	if err != nil || string(contents) != "pinned certificate" {
		t.Fatalf("prepared CA = %q, error = %v", contents, err)
	}
	info, err := os.Stat(prepared)
	if err != nil || info.Mode().Perm() != 0o400 {
		t.Fatalf("prepared CA mode = %v, error = %v", info, err)
	}
}

func TestPrepareCAFileRejectsUnsafeInputsWithoutDisclosureOrArtifacts(t *testing.T) {
	root := t.TempDir()
	regular := filepath.Join(root, "sentinel-secret.pem")
	if err := os.WriteFile(regular, []byte("secret contents"), 0o600); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(root, "link.pem")
	if err := os.Symlink(regular, symlink); err != nil {
		t.Fatal(err)
	}
	fifo := filepath.Join(root, "input.fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	unreadable := filepath.Join(root, "unreadable.pem")
	if err := os.WriteFile(unreadable, []byte("secret"), 0); err != nil {
		t.Fatal(err)
	}
	for name, source := range map[string]string{
		"missing":    filepath.Join(root, "missing-secret"),
		"directory":  root,
		"symlink":    symlink,
		"fifo":       fifo,
		"unreadable": unreadable,
	} {
		t.Run(name, func(t *testing.T) {
			policyDir := filepath.Join(root, "policy-"+name)
			if err := os.Mkdir(policyDir, 0o700); err != nil {
				t.Fatal(err)
			}
			_, err := prepareCAFile(source, policyDir)
			if err == nil {
				t.Fatal("prepareCAFile() error = nil")
			}
			if strings.Contains(err.Error(), "sentinel") || strings.Contains(err.Error(), "secret") {
				t.Fatalf("error disclosed input: %v", err)
			}
			entries, readErr := os.ReadDir(policyDir)
			if readErr != nil || len(entries) != 0 {
				t.Fatalf("failed preparation left artifacts: entries=%v error=%v", entries, readErr)
			}
		})
	}
}
