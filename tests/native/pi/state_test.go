//go:build native

package pi

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPiStartupUsesContainedDefaultAndInheritedState(t *testing.T) {
	fixture := newPiFixture(t)
	result := fixture.sandbox("", "--mode", "rpc")
	if result.err != nil {
		t.Fatalf("Pi RPC startup with EOF failed: %v\n%s", result.err, result.stderr)
	}
	requireNoCredential(t, result, "not-a-real-provider-credential")
	for _, path := range []string{fixture.agentDir, fixture.sessionDir} {
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			t.Fatalf("selected Pi state directory was not available: %s: %v", path, err)
		}
	}
}

func TestPiStatePrecedenceAndProtectedHomes(t *testing.T) {
	fixture := newPiFixture(t)
	inheritedAgent := filepath.Join(fixture.root, "inherited-agent")
	inheritedSession := filepath.Join(fixture.root, "inherited-sessions")
	for _, path := range []string{inheritedAgent, inheritedSession} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	fixture.agentDir, fixture.sessionDir = inheritedAgent, inheritedSession
	result := fixture.sandbox("", "--mode", "rpc")
	if result.err != nil {
		t.Fatalf("Pi inherited state startup failed: %v\n%s", result.err, result.stderr)
	}

	// The real wrapper must reject all path-like session input before Pi can read it.
	for _, argument := range []string{"../outside.jsonl", filepath.Join(fixture.root, "outside.jsonl")} {
		result := fixture.sandbox("", "--session", argument)
		requireDenied(t, result)
	}
}

func TestPiRejectsHostStateAliasesAndOverlaps(t *testing.T) {
	fixture := newPiFixture(t)
	fixture.sessionDir = fixture.agentDir
	requireDenied(t, fixture.sandbox("", "--mode", "rpc"))

	// A final-component link is not an admissible selected state directory.
	target := filepath.Join(fixture.root, "target")
	alias := filepath.Join(fixture.root, "alias")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, alias); err != nil {
		t.Fatal(err)
	}
	fixture.agentDir, fixture.sessionDir = alias, filepath.Join(fixture.root, "separate")
	requireDenied(t, fixture.sandbox("", "--mode", "rpc"))
}
