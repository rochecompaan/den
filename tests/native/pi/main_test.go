//go:build native

package pi

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var requiredEnvironment = []string{
	"DEN_NATIVE_PI_TEST_BINARY",
	"DEN_NATIVE_PI",
	"DEN_NATIVE_PI_SANDBOX",
	"DEN_NATIVE_PI_MANIFEST",
	"DEN_NATIVE_PI_PACKAGE_ROOT",
	"DEN_NATIVE_PI_RESOURCE_FIXTURE",
	"DEN_NATIVE_LAUNCHER",
	"DEN_NATIVE_FENCE",
	"DEN_NATIVE_REPOWOLF_CLIENT_DIR",
}

func TestMain(m *testing.M) {
	for _, name := range requiredEnvironment {
		value := os.Getenv(name)
		if value == "" || !filepath.IsAbs(value) {
			fmt.Fprintf(os.Stderr, "Pi native enforcement requires absolute packaged input: %s\n", name)
			os.Exit(1)
		}
	}
	if root := os.Getenv("DEN_NATIVE_HOST_ROOT"); root == "" || !filepath.IsAbs(root) {
		fmt.Fprintln(os.Stderr, "Pi native enforcement requires DEN_NATIVE_HOST_ROOT")
		os.Exit(1)
	}
	if status := m.Run(); status != 0 {
		os.Exit(status)
	}
	if err := os.WriteFile(filepath.Join(os.Getenv("DEN_NATIVE_HOST_ROOT"), "pi-suite.complete"), []byte("complete\n"), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "write Pi suite completion:", err)
		os.Exit(1)
	}
}

func requireDenied(t testing.TB, result commandResult) {
	t.Helper()
	if result.err == nil {
		t.Fatalf("expected denial, got stdout=%q stderr=%q", result.stdout, result.stderr)
	}
}

func requireNoCredential(t testing.TB, result commandResult, credential string) {
	t.Helper()
	if strings.Contains(result.stdout, credential) || strings.Contains(result.stderr, credential) {
		t.Fatal("Pi launch disclosed fixture credential")
	}
}
