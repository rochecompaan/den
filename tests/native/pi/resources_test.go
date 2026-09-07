//go:build native

package pi

import (
	"os"
	"strings"
	"testing"
)

func TestPiConfiguredImmutableResourcesLoadInOrder(t *testing.T) {
	fixture := newPiFixture(t)
	if err := os.Remove(fixture.reportPath()); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	result := fixture.sandbox("", "--mode", "rpc")
	if result.err != nil {
		t.Fatalf("Pi configured-resource startup failed: %v\n%s", result.err, result.stderr)
	}
	contents, err := os.ReadFile(fixture.reportPath())
	if err != nil {
		t.Fatalf("configured Pi resources did not report: %v", err)
	}
	report := string(contents)
	want := []string{
		"extension:report-extension", "extension:provider-extension", "extension:switch-extension", "package:fixture-package",
		"skill:fixture-skill", "prompt:fixture-prompt", "theme:fixture-theme",
	}
	previous := -1
	for _, name := range want {
		position := strings.Index(report, name)
		if position < 0 || position < previous {
			t.Fatalf("configured resource order is missing %q in %q", name, report)
		}
		previous = position
	}
}

func TestPiNoDiscoveryFlagsKeepMandatoryResources(t *testing.T) {
	fixture := newPiFixture(t)
	for _, flag := range []string{"--no-extensions", "--no-skills", "--no-prompt-templates", "--no-themes"} {
		if err := os.Remove(fixture.reportPath()); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		result := fixture.sandbox("", "--mode", "rpc", flag)
		if result.err != nil {
			t.Fatalf("Pi %s startup failed: %v\n%s", flag, result.err, result.stderr)
		}
		if _, err := os.Stat(fixture.reportPath()); err != nil {
			t.Fatalf("mandatory configured resources disappeared with %s: %v", flag, err)
		}
	}
}
