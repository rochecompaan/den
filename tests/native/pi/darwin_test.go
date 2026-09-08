//go:build native && darwin

package pi

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const piDarwinStartupCompletion = "pi-darwin-startup.complete"

func requirePiDarwinStartupCompletion(root string) error {
	if root == "" {
		return fmt.Errorf("Darwin Pi startup completion requires DEN_NATIVE_HOST_ROOT")
	}
	contents, err := os.ReadFile(filepath.Join(root, piDarwinStartupCompletion))
	if err != nil {
		return fmt.Errorf("read Darwin Pi startup completion: %w", err)
	}
	if string(contents) != "complete\n" {
		return fmt.Errorf("unexpected Darwin Pi startup completion content: %q", contents)
	}
	return nil
}

func TestPiDarwinStartupFixtureCompleted(t *testing.T) {
	root := os.Getenv("DEN_NATIVE_HOST_ROOT")
	if err := requirePiDarwinStartupCompletion(root); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(filepath.Join(root, "pi-darwin-startup", "assertions.report"))
	if err != nil {
		t.Fatal(err)
	}
	for _, assertion := range []string{
		"allowed-bash-after-no-change-helper", "fail-closed:deny", "fail-closed:rewrite",
		"fail-closed:malformed", "fail-closed:failed", "native-user-bash-parity",
		"user-and-project-hostile-extensions-loaded-through-real-scopes",
		"hostile-user-project-extensions-cannot-replace-entrypoints", "identity-change-fails-closed",
		"helper-created-no-http-or-socks-listener", "outer-fence-required-for-shell-entrypoints",
		"direct-extension-process-outer-fence-constrained",
		"prestart-extension-mismatch-fails-before-launch", "prestart-policy-mismatch-fails-before-launch",
		"prestart-extension-identity-diagnostic", "prestart-policy-identity-diagnostic",
	} {
		if !strings.Contains("\n"+string(contents), "\n"+assertion+"\n") {
			t.Fatalf("Darwin Pi startup assertion %q is missing from %q", assertion, contents)
		}
	}
	worktree := filepath.Join(root, "pi-darwin-startup", "worktree")
	started, err := os.ReadFile(filepath.Join(worktree, "pi-started"))
	if err != nil {
		t.Fatal(err)
	}
	if string(started) != "started\n" {
		t.Fatalf("unexpected Pi startup marker: %q", started)
	}
	direct, err := os.ReadFile(filepath.Join(worktree, "direct-extension.report"))
	if err != nil {
		t.Fatal(err)
	}
	if string(direct) != "denied\n" {
		t.Fatalf("unexpected direct-extension report: %q", direct)
	}
}

func TestPiDarwinStartupCompletionContract(t *testing.T) {
	root := t.TempDir()
	if err := requirePiDarwinStartupCompletion(root); err == nil {
		t.Fatal("missing Darwin Pi startup completion was accepted")
	}
	if err := os.WriteFile(filepath.Join(root, piDarwinStartupCompletion), []byte("incomplete\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := requirePiDarwinStartupCompletion(root); err == nil {
		t.Fatal("malformed Darwin Pi startup completion was accepted")
	}
	if err := os.WriteFile(filepath.Join(root, piDarwinStartupCompletion), []byte("complete\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := requirePiDarwinStartupCompletion(root); err != nil {
		t.Fatal(err)
	}
}
