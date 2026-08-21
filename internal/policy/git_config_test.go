package policy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateCanonicalizesGitConfigDenialsThroughSymlinkedGitDirectory(t *testing.T) {
	root := t.TempDir()
	paths := makePaths(t, root)
	gitDirectory := filepath.Join(root, "git-metadata")
	configTargets := filepath.Join(root, "config-targets")
	if err := os.MkdirAll(gitDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(configTargets, 0o700); err != nil {
		t.Fatal(err)
	}

	config := filepath.Join(configTargets, "config")
	worktreeConfig := filepath.Join(configTargets, "config.worktree")
	writeTestFile(t, config)
	writeTestFile(t, worktreeConfig)
	if err := os.Symlink(config, filepath.Join(gitDirectory, "config")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(worktreeConfig, filepath.Join(gitDirectory, "config.worktree")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(gitDirectory, filepath.Join(paths.worktree, ".git")); err != nil {
		t.Fatal(err)
	}

	got := generateTestPolicy(t, paths)
	for _, target := range []string{config, worktreeConfig} {
		if !contains(got.Filesystem.DenyWrite, target) {
			t.Errorf("denyWrite missing canonical Git config target %q: %#v", target, got.Filesystem.DenyWrite)
		}
	}
	for _, lexical := range []string{filepath.Join(paths.worktree, ".git/config"), filepath.Join(paths.worktree, ".git/config.worktree")} {
		if contains(got.Filesystem.DenyWrite, lexical) {
			t.Errorf("denyWrite retained lexical Git config path %q", lexical)
		}
	}
}

func TestGenerateResolvesLinkedWorktreeGitfileConfigLocations(t *testing.T) {
	root := t.TempDir()
	paths := makePaths(t, root)
	metadataRoot := filepath.Join(root, "metadata")
	commonDirectory := filepath.Join(metadataRoot, "common")
	worktreeGitDirectory := filepath.Join(metadataRoot, "worktrees", "task")
	if err := os.MkdirAll(commonDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(worktreeGitDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktreeGitDirectory, "commondir"), []byte("../../common\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paths.worktree, ".git"), []byte("gitdir: "+worktreeGitDirectory+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	commonConfig := filepath.Join(commonDirectory, "config")
	worktreeConfig := filepath.Join(worktreeGitDirectory, "config.worktree")
	writeTestFile(t, commonConfig)
	writeTestFile(t, worktreeConfig)

	got := generateTestPolicy(t, paths)
	for _, target := range []string{commonConfig, worktreeConfig} {
		if !contains(got.Filesystem.DenyWrite, target) {
			t.Errorf("denyWrite missing linked-worktree config %q: %#v", target, got.Filesystem.DenyWrite)
		}
	}
}

func TestGenerateFailsClosedForMalformedGitfile(t *testing.T) {
	paths := makePaths(t, t.TempDir())
	if err := os.WriteFile(filepath.Join(paths.worktree, ".git"), []byte("not a gitdir\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(Base(readBase(t)), testDynamic(paths)); err == nil || !strings.Contains(err.Error(), "Git metadata") {
		t.Fatalf("Generate error = %v, want redacted Git metadata failure", err)
	}
}

func generateTestPolicy(t *testing.T, paths testPaths) document {
	t.Helper()
	encoded, err := Generate(Base(readBase(t)), testDynamic(paths))
	if err != nil {
		t.Fatal(err)
	}
	var got document
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	return got
}

func testDynamic(paths testPaths) Dynamic {
	return Dynamic{
		Platform: "darwin", RepoWolfHostname: "broker.example.test", CAFile: paths.ca,
		Worktree: paths.worktree, ScratchDir: paths.scratch, PolicyFile: paths.policy,
	}
}

func writeTestFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("unchanged\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}
