package managedstate

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/rochecompaan/den/internal/manifest"
	"golang.org/x/sys/unix"
)

func TestRestoreCreatesParentsAndReplacesLeaves(t *testing.T) {
	rootPath, storeDir, source := restorePaths(t)
	mustWrite(t, filepath.Join(rootPath, "auth.json"), "keep")
	if err := os.Mkdir(filepath.Join(rootPath, "profiles"), 0o755); err != nil {
		t.Fatal(err)
	}

	result, err := restore(rootFor(t, rootPath), []manifest.ManagedStateFile{{
		Destination: "profiles/pi-subagents/openai.json", Source: source,
	}}, testDependencies(storeDir))
	if err != nil {
		t.Fatal(err)
	}
	if result.Mutated != true {
		t.Fatal("Restore() did not report mutation")
	}
	if target, err := os.Readlink(filepath.Join(rootPath, "profiles/pi-subagents/openai.json")); err != nil || target != source {
		t.Fatalf("managed link = %q, %v; want %q", target, err, source)
	}
	if mode := mustMode(t, filepath.Join(rootPath, "profiles")); mode.Perm() != 0o700 {
		t.Fatalf("profiles mode = %o, want 700", mode.Perm())
	}
	if mode := mustMode(t, filepath.Join(rootPath, "profiles", "pi-subagents")); mode.Perm() != 0o700 {
		t.Fatalf("pi-subagents mode = %o, want 700", mode.Perm())
	}
	if got := mustRead(t, filepath.Join(rootPath, "auth.json")); got != "keep" {
		t.Fatalf("unmanaged auth changed: %q", got)
	}
}

func TestRestoreReplacesRegularFileAndExistingSymlink(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(t *testing.T, leaf string)
	}{
		{"regular file", func(t *testing.T, leaf string) { mustWrite(t, leaf, "old") }},
		{"symlink", func(t *testing.T, leaf string) {
			if err := os.Symlink("old-target", leaf); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			rootPath, storeDir, source := restorePaths(t)
			leaf := filepath.Join(rootPath, "openai.json")
			test.setup(t, leaf)
			result, err := restore(rootFor(t, rootPath), []manifest.ManagedStateFile{{Destination: "openai.json", Source: source}}, testDependencies(storeDir))
			if err != nil || !result.Mutated {
				t.Fatalf("restore() = %#v, %v", result, err)
			}
			if target, err := os.Readlink(leaf); err != nil || target != source {
				t.Fatalf("link = %q, %v", target, err)
			}
		})
	}
}

func TestRestorePreservesUnmanagedSiblings(t *testing.T) {
	rootPath, storeDir, source := restorePaths(t)
	mustWrite(t, filepath.Join(rootPath, "profiles", "unmanaged.json"), "keep")
	if _, err := restore(rootFor(t, rootPath), []manifest.ManagedStateFile{{Destination: "profiles/openai.json", Source: source}}, testDependencies(storeDir)); err != nil {
		t.Fatal(err)
	}
	if got := mustRead(t, filepath.Join(rootPath, "profiles", "unmanaged.json")); got != "keep" {
		t.Fatalf("unmanaged sibling = %q", got)
	}
}

func TestRestoreRejectsParentSymlink(t *testing.T) {
	rootPath, storeDir, source := restorePaths(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(rootPath, "profiles")); err != nil {
		t.Fatal(err)
	}
	assertRestoreError(t, rootFor(t, rootPath), []manifest.ManagedStateFile{{Destination: "profiles/pi-subagents/openai.json", Source: source}}, testDependencies(storeDir), "profiles/pi-subagents/openai.json", "parent is a symbolic link")
}

func TestRestoreRejectsDirectoryLeaf(t *testing.T) {
	rootPath, storeDir, source := restorePaths(t)
	if err := os.MkdirAll(filepath.Join(rootPath, "profiles", "openai.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	assertRestoreError(t, rootFor(t, rootPath), []manifest.ManagedStateFile{{Destination: "profiles/openai.json", Source: source}}, testDependencies(storeDir), "profiles/openai.json", "managed leaf is a directory")
}

func TestRestoreRejectsSpecialLeaf(t *testing.T) {
	rootPath, storeDir, source := restorePaths(t)
	leaf := filepath.Join(rootPath, "openai.json")
	if err := unix.Mkfifo(leaf, 0o600); err != nil {
		t.Fatal(err)
	}
	assertRestoreError(t, rootFor(t, rootPath), []manifest.ManagedStateFile{{Destination: "openai.json", Source: source}}, testDependencies(storeDir), "openai.json", "managed leaf is not a regular file or symbolic link")
}

func TestRestoreRejectsSourceThatResolvesOutsideStore(t *testing.T) {
	rootPath, storeDir, _ := restorePaths(t)
	outside := filepath.Join(t.TempDir(), "source.json")
	mustWrite(t, outside, "source")
	source := filepath.Join(storeDir, "linked-source.json")
	if err := os.Symlink(outside, source); err != nil {
		t.Fatal(err)
	}
	assertRestoreError(t, rootFor(t, rootPath), []manifest.ManagedStateFile{{Destination: "profiles/pi-subagents/openai.json", Source: source}}, testDependencies(storeDir), "profiles/pi-subagents/openai.json", "source resolves outside the Nix store")
}

func TestRestoreRejectsChangedRootIdentity(t *testing.T) {
	rootPath, storeDir, source := restorePaths(t)
	root := rootFor(t, rootPath)
	if err := os.Rename(rootPath, rootPath+"-old"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(rootPath, 0o700); err != nil {
		t.Fatal(err)
	}
	assertRestoreError(t, root, []manifest.ManagedStateFile{{Destination: "profiles/pi-subagents/openai.json", Source: source}}, testDependencies(storeDir), "profiles/pi-subagents/openai.json", "state root identity changed")
}

func TestRestoreReportsMutationBeforeLaterFailure(t *testing.T) {
	rootPath, storeDir, source := restorePaths(t)
	if err := os.MkdirAll(filepath.Join(rootPath, "blocked", "openai.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	result, err := restore(rootFor(t, rootPath), []manifest.ManagedStateFile{
		{Destination: "profiles/openai.json", Source: source},
		{Destination: "blocked/openai.json", Source: source},
	}, testDependencies(storeDir))
	if err == nil || !strings.Contains(err.Error(), "blocked/openai.json") || !strings.Contains(err.Error(), "managed leaf is a directory") {
		t.Fatalf("restore() error = %v", err)
	}
	if !result.Mutated {
		t.Fatal("restore() did not report earlier mutation")
	}
}

func TestRestoreReportsFailedTemporaryLinkCleanup(t *testing.T) {
	rootPath, storeDir, source := restorePaths(t)
	renameFailure := errors.New("rename failure")
	cleanupFailure := errors.New("cleanup failure")
	deps := testDependencies(storeDir)
	deps.leafOperations.renameat = func(int, string, int, string) error { return renameFailure }
	deps.leafOperations.unlinkat = func(int, string, int) error { return cleanupFailure }

	result, err := restore(rootFor(t, rootPath), []manifest.ManagedStateFile{{
		Destination: "openai.json", Source: source,
	}}, deps)
	if !result.Mutated {
		t.Fatal("restore() did not report an unremoved temporary link as a mutation")
	}
	if !errors.Is(err, renameFailure) {
		t.Fatalf("restore() error = %v, want rename failure", err)
	}
	if !errors.Is(err, cleanupFailure) {
		t.Fatalf("restore() error = %v, want cleanup failure", err)
	}
	if _, statErr := os.Lstat(filepath.Join(rootPath, ".den-managed-test")); statErr != nil {
		t.Fatalf("temporary link = %v, want it to remain after cleanup failure", statErr)
	}
}

func TestRestoreDoesNotReportMutationWhenTemporaryLinkCleanupSucceeds(t *testing.T) {
	rootPath, storeDir, source := restorePaths(t)
	renameFailure := errors.New("rename failure")
	deps := testDependencies(storeDir)
	deps.leafOperations.renameat = func(int, string, int, string) error { return renameFailure }

	result, err := restore(rootFor(t, rootPath), []manifest.ManagedStateFile{{
		Destination: "openai.json", Source: source,
	}}, deps)
	if result.Mutated {
		t.Fatal("restore() reported mutation after successful temporary-link cleanup")
	}
	if !errors.Is(err, renameFailure) {
		t.Fatalf("restore() error = %v, want rename failure", err)
	}
	if _, statErr := os.Lstat(filepath.Join(rootPath, ".den-managed-test")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("temporary link stat error = %v, want not exist", statErr)
	}
}

func assertRestoreError(t *testing.T, root Root, files []manifest.ManagedStateFile, deps dependencies, destination, condition string) {
	t.Helper()
	_, err := restore(root, files, deps)
	if err == nil || !strings.Contains(err.Error(), destination) || !strings.Contains(err.Error(), condition) {
		t.Fatalf("restore() error = %v, want destination %q and condition %q", err, destination, condition)
	}
}

func restorePaths(t *testing.T) (rootPath, storeDir, source string) {
	t.Helper()
	rootPath = filepath.Join(t.TempDir(), "state")
	storeDir = filepath.Join(t.TempDir(), "store")
	if err := os.Mkdir(rootPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(storeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	source = filepath.Join(storeDir, "source.json")
	mustWrite(t, source, "source")
	return rootPath, storeDir, source
}

func testDependencies(storeDir string) dependencies {
	return dependencies{
		storeDir:       storeDir,
		uid:            uint32(os.Getuid()),
		tempName:       func() (string, error) { return ".den-managed-test", nil },
		leafOperations: systemLeafOperations,
	}
}

func rootFor(t *testing.T, path string) Root {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatal("root stat lacks identity")
	}
	return Root{Path: path, Device: uint64(stat.Dev), Inode: stat.Ino}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
func mustRead(t *testing.T, path string) string {
	t.Helper()
	value, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(value)
}
func mustMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode()
}
func mustChmod(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}
