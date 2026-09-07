//go:build native

package pi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPiPackageCommandsCannotMutateState(t *testing.T) {
	fixture := newPiFixture(t)
	settings := filepath.Join(fixture.agentDir, "settings.json")
	packages := filepath.Join(fixture.agentDir, "packages")
	if err := os.WriteFile(settings, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(packages, 0o700); err != nil {
		t.Fatal(err)
	}
	beforeSettings, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}

	for _, binary := range []string{os.Getenv("DEN_NATIVE_PI_SANDBOX"), os.Getenv("DEN_NATIVE_PI")} {
		for _, command := range []string{"install", "remove", "uninstall", "update", "list", "config"} {
			result := fixture.run(binary, "", nil, command)
			requireDenied(t, result)
			if !strings.Contains(result.stdout+result.stderr, "disabled by Den") && binary == os.Getenv("DEN_NATIVE_PI") {
				t.Fatalf("direct Pi command %q did not report immutable package denial: %s", command, result.stderr)
			}
		}
	}
	afterSettings, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	if string(beforeSettings) != string(afterSettings) {
		t.Fatal("package command changed Pi settings")
	}
	entries, err := os.ReadDir(packages)
	if err != nil || len(entries) != 0 {
		t.Fatalf("package command changed package state: %v / %v", entries, err)
	}
}

func TestPiRejectsReservedResourceAndRelativeExecutableInputs(t *testing.T) {
	fixture := newPiFixture(t)
	for _, arguments := range [][]string{
		{"--extension", "resource.ts"},
		{"--skill", "skill"},
		{"--prompt-template", "prompt.md"},
		{"--theme", "theme.json"},
		{"--session-dir", "relative"},
	} {
		requireDenied(t, fixture.sandbox("", arguments...))
	}
}
