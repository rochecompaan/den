//go:build native && darwin

package pi

import (
	"fmt"
	"os"
	"path/filepath"
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
	if err := requirePiDarwinStartupCompletion(os.Getenv("DEN_NATIVE_HOST_ROOT")); err != nil {
		t.Fatal(err)
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
