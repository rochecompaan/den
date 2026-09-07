//go:build native

package pi

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func prepareAmbientResources(t *testing.T, fixture *piFixture) {
	t.Helper()
	files := map[string]string{
		"extensions/ambient.ts":         `import {appendFileSync} from "node:fs"; import {join} from "node:path"; export default function() { appendFileSync(join(process.env.PI_CODING_AGENT_DIR!, "pi-resources.report"), "extension:ambient-extension\n"); }`,
		"skills/ambient-skill/SKILL.md": "---\nname: ambient-skill\ndescription: Ambient skill fixture\n---\nAmbient skill.\n",
		"prompts/ambient-prompt.md":     "Ambient prompt.\n",
	}
	theme, err := os.ReadFile(filepath.Join(os.Getenv("DEN_NATIVE_PI_RESOURCE_FIXTURE"), "themes/package-theme.json"))
	if err != nil {
		t.Fatal(err)
	}
	files["themes/ambient-theme.json"] = strings.ReplaceAll(string(theme), "fixture-package-theme", "ambient-theme")
	for relative, contents := range files {
		path := filepath.Join(fixture.agentDir, relative)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

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
	requireReportLines(t, fixture.reportPath(),
		"inventory:skill:fixture-skill:direct", "inventory:skill:fixture-package-skill:package",
		"inventory:prompt:prompt:direct", "inventory:prompt:package-prompt:package",
		"inventory:theme:fixture-theme:direct", "inventory:theme:fixture-package-theme:package",
		"collision:skill:native-collision:package:direct", "collision:prompt:native-collision:package:direct",
		"collision:theme:native-collision:package:direct")
	for _, kind := range []string{"skill", "prompt", "theme"} {
		if strings.Contains(report, "inventory:"+kind+":native-collision:direct") {
			t.Fatal("immutable collision loser was loaded")
		}
	}
	want := []string{
		"extension:report-extension", "extension:provider-extension", "extension:switch-extension", "package:fixture-package",
	}
	for _, fabricated := range []string{"skill:fixture-skill", "prompt:fixture-prompt", "theme:fixture-theme"} {
		if strings.Contains("\n"+report, "\n"+fabricated+"\n") {
			t.Fatalf("resource report fabricated an unloaded resource %q in %q", fabricated, report)
		}
	}
	previous := -1
	for _, name := range want {
		position := reportLineIndex(contents, name)
		if position < 0 || position < previous {
			t.Fatalf("configured resource order is missing %q in %q", name, report)
		}
		previous = position
	}
}

func TestPiImmutableExtensionCollisionNamesWinnerAndLoser(t *testing.T) {
	fixture := newPiFixture(t)
	loser := filepath.Join(os.Getenv("DEN_NATIVE_PI_RESOURCE_FIXTURE"), "collision-package/extensions/z-collision.ts")
	var winner string
	result := fixture.launch("", nil, func(document map[string]any) {
		agent := document["agent"].(map[string]any)
		for _, argument := range agent["resourceArgs"].([]any) {
			if path, ok := argument.(string); ok && strings.HasSuffix(path, "-report-extension.ts") {
				winner = path
			}
		}
		agent["resourceArgs"] = append(agent["resourceArgs"].([]any), "--extension", filepath.Dir(filepath.Dir(loser)))
	}, "--mode", "rpc")
	requireDeniedWith(t, result, `Tool "native-collision" conflicts with `)
	if winner == "" || !reportHasLine([]byte(result.stderr), fmt.Sprintf(`Error: Failed to load extension "%s": Tool "native-collision" conflicts with %s`, loser, winner)) {
		t.Fatalf("actual extension collision did not name both immutable paths: %s", result.stderr)
	}
	report, _ := os.ReadFile(fixture.reportPath())
	if strings.Contains(string(report), "session-start:") {
		t.Fatal("extension collision did not fail before runtime startup")
	}
}

func TestPiNoDiscoveryFlagsKeepMandatoryResources(t *testing.T) {
	fixture := newPiFixture(t)
	prepareAmbientResources(t, fixture)
	for _, flag := range []string{"", "--no-extensions", "-ne", "--no-skills", "-ns", "--no-prompt-templates", "-np", "--no-themes"} {
		if err := os.Remove(fixture.reportPath()); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		args := []string{"--mode", "rpc"}
		if flag != "" {
			args = append(args, flag)
		}
		result := fixture.sandbox("", args...)
		if result.err != nil {
			t.Fatalf("Pi %s startup failed: %v\n%s", flag, result.err, result.stderr)
		}
		requireReportLines(t, fixture.reportPath(), "extension:report-extension", "package:fixture-package",
			"inventory:skill:fixture-skill:direct", "inventory:skill:fixture-package-skill:package",
			"inventory:prompt:prompt:direct", "inventory:prompt:package-prompt:package",
			"inventory:theme:fixture-theme:direct", "inventory:theme:fixture-package-theme:package")
		report, _ := os.ReadFile(fixture.reportPath())
		for kind, disabled := range map[string]bool{
			"extension": flag == "--no-extensions" || flag == "-ne",
			"skill":     flag == "--no-skills" || flag == "-ns",
			"prompt":    flag == "--no-prompt-templates" || flag == "-np",
			"theme":     flag == "--no-themes",
		} {
			needle := "inventory:" + kind + ":ambient-" + kind + ":ambient"
			if kind == "extension" {
				needle = "extension:ambient-extension"
			}
			if reportHasLine(report, needle) == disabled {
				t.Fatalf("%s ambient subgroup not isolated by %q: %s", kind, flag, report)
			}
			if !disabled && kind == "extension" {
				direct := reportLineIndex(report, "extension:switch-extension")
				packaged := reportLineIndex(report, "package:fixture-package")
				ambient := reportLineIndex(report, needle)
				if direct < 0 || direct >= packaged || packaged >= ambient {
					t.Fatalf("extension direct/package/ambient order changed: %s", report)
				}
			}
			if !disabled && kind != "extension" {
				packageIndex := reportLineIndex(report, "inventory:"+kind+":native-collision:package")
				ambientIndex := reportLineIndex(report, needle)
				directIndex := reportLineIndex(report, "inventory:"+kind+":"+map[string]string{"skill": "fixture-skill", "prompt": "prompt", "theme": "fixture-theme"}[kind]+":direct")
				if packageIndex < 0 || packageIndex >= ambientIndex || ambientIndex >= directIndex {
					t.Fatalf("%s subgroup order changed: %s", kind, report)
				}
			}
		}
	}
}

func reportLineIndex(report []byte, line string) int {
	for index, candidate := range strings.Split(string(report), "\n") {
		if candidate == line {
			return index
		}
	}
	return -1
}
