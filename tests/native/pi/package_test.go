//go:build native

package pi

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPiPackageCommandsRejectBeforeFenceAndMutation(t *testing.T) {
	commands := []string{"install", "remove", "uninstall", "update", "list", "config"}
	for _, binary := range []string{"wrapper", "direct"} {
		for _, command := range commands {
			t.Run(binary+"_"+command, func(t *testing.T) {
				fixture := newPiFixture(t)
				paths, extension, requestMarker := preparePackageState(t, fixture)
				before := snapshotPaths(t, paths...)
				var result commandResult
				if binary == "wrapper" {
					fence, fenceMarker := observingFence(t, fixture)
					result = fixture.launch("", []string{"DEN_PI_FENCE_MARKER=" + fenceMarker, "DEN_PI_HOSTILE_MARKER=" + requestMarker, "PATH=" + filepath.Dir(requestMarker)},
						func(document map[string]any) { document["fenceExecutable"] = fence }, command, "--extension", extension)
					if pathExists(fenceMarker) {
						t.Fatal("reserved package command reached Fence")
					}
				} else {
					result = fixture.run(os.Getenv("DEN_NATIVE_PI"), "", []string{"DEN_PI_HOSTILE_MARKER=" + requestMarker, "PATH=" + filepath.Dir(requestMarker)}, command, "--extension", extension)
				}
				requireExactDenPackageDenial(t, result, binary)
				if after := snapshotPaths(t, paths...); after != before {
					t.Fatalf("%s %s changed package state", binary, command)
				}
				if pathExists(requestMarker) {
					t.Fatalf("%s %s attempted package resolution", binary, command)
				}
			})
		}
	}
}

func TestPiReservedArgumentGrammarRejectsBeforeFence(t *testing.T) {
	rejected := []struct {
		name string
		args []string
		want string
	}{
		{"session-dir", []string{"--session-dir", "x"}, "Den-owned input"},
		{"session-dir-equals", []string{"--session-dir=x"}, "Den-owned input"},
		{"session-path", []string{"--session", "../outside.jsonl"}, "hexadecimal ID"},
		{"session-missing", []string{"--session"}, "hexadecimal ID"},
		{"session-equals", []string{"--session=abcdef"}, "Den-owned input"},
		{"fork-path", []string{"--fork", "/outside.jsonl"}, "hexadecimal ID"},
		{"fork-missing", []string{"--fork"}, "hexadecimal ID"},
		{"fork-equals", []string{"--fork=abcdef"}, "Den-owned input"},
		{"export", []string{"--export", "out.html"}, "Den-owned input"},
		{"export-equals", []string{"--export=out.html"}, "Den-owned input"},
		{"extension", []string{"--extension", "resource.ts"}, "Den-owned input"},
		{"extension-equals", []string{"--extension=resource.ts"}, "Den-owned input"},
		{"extension-short", []string{"-e", "resource.ts"}, "Den-owned input"},
		{"extension-short-attached", []string{"-eresource.ts"}, "Den-owned input"},
		{"extension-short-cluster", []string{"-xe"}, "Den-owned input"},
		{"skill", []string{"--skill", "skill"}, "Den-owned input"},
		{"skill-equals", []string{"--skill=skill"}, "Den-owned input"},
		{"prompt", []string{"--prompt-template", "prompt.md"}, "Den-owned input"},
		{"prompt-equals", []string{"--prompt-template=prompt.md"}, "Den-owned input"},
		{"theme", []string{"--theme", "theme.json"}, "Den-owned input"},
		{"theme-equals", []string{"--theme=theme.json"}, "Den-owned input"},
		{"session-id-missing", []string{"--session-id"}, "UUID"},
		{"session-id-malformed", []string{"--session-id", "abcdef"}, "UUID"},
		{"session-id-equals", []string{"--session-id=00000000-0000-0000-0000-000000000000"}, "Den-owned input"},
	}
	for _, flag := range []string{"--session-dir", "--export", "--extension", "-e", "--skill", "--prompt-template", "--theme"} {
		rejected = append(rejected, struct {
			name string
			args []string
			want string
		}{flag + "-missing", []string{flag}, "Den-owned input"})
	}
	fixture := newPiFixture(t)
	fence, marker := observingFence(t, fixture)
	for _, test := range rejected {
		t.Run(test.name, func(t *testing.T) {
			_ = os.Remove(marker)
			before := snapshotPaths(t, fixture.agentDir, fixture.sessionDir)
			result := fixture.launch("", []string{"DEN_PI_FENCE_MARKER=" + marker},
				func(document map[string]any) { document["fenceExecutable"] = fence }, test.args...)
			requireDeniedWith(t, result, test.want)
			if pathExists(marker) {
				t.Fatal("reserved argument reached Fence")
			}
			if after := snapshotPaths(t, fixture.agentDir, fixture.sessionDir); after != before {
				t.Fatal("reserved argument changed Pi state")
			}
		})
	}

	for name, args := range map[string][]string{
		"session-hex-id":  {"--session", "abcdef"},
		"fork-hex-id":     {"--fork", "abcdef"},
		"session-uuid":    {"--session-id", "00000000-0000-0000-0000-000000000000"},
		"short-no-ext":    {"-ne", "--version"},
		"short-no-skill":  {"-ns", "--version"},
		"short-no-prompt": {"-np", "--version"},
		"long-no-themes":  {"--no-themes", "--version"},
		"continue":        {"-c", "--version"}, "resume": {"-r", "--version"},
		"all": {"-a", "--version"}, "no-agents": {"-na", "--version"},
		"literal-delimiter": {"--mode", "rpc", "--", "--extension", "install"},
	} {
		t.Run("allowed_"+name, func(t *testing.T) {
			_ = os.Remove(marker)
			_ = fixture.launch("", []string{"DEN_PI_FENCE_MARKER=" + marker},
				func(document map[string]any) { document["fenceExecutable"] = fence }, args...)
			if !pathExists(marker) {
				t.Fatal("permitted reserved-grammar form was rejected before Fence")
			}
		})
	}
}

func TestPiRuntimePackageManagerStaysOfflineAfterEnvironmentMutation(t *testing.T) {
	for _, mode := range []string{"changed", "removed"} {
		t.Run(mode, func(t *testing.T) {
			fixture := newPiFixture(t)
			paths, _, requestMarker := preparePackageState(t, fixture)
			before := snapshotPaths(t, paths...)
			result := fixture.run(os.Getenv("DEN_NATIVE_PI_SANDBOX"), rpcInput(fmt.Sprintf(
				`{"id":"package-%s","type":"prompt","message":"/native-switch package-%s"}`, mode, mode)),
				[]string{"PATH=" + filepath.Dir(requestMarker) + string(os.PathListSeparator) + os.Getenv("PATH")}, "--mode", "rpc")
			if result.err != nil {
				t.Fatalf("runtime package attack fixture failed: %v\n%s%s", result.err, result.stdout, result.stderr)
			}
			requireReportLines(t, fixture.reportPath(), "package-attack:"+mode+":denied:30", "package-resolution:requests:0:state-checks:33")
			if after := snapshotPaths(t, paths...); after != before {
				t.Fatal("runtime-visible package manager changed global or project state")
			}
			if pathExists(requestMarker) {
				t.Fatal("runtime-visible package manager made a package-resolution request")
			}
		})
	}
}

func preparePackageState(t *testing.T, fixture *piFixture) ([]string, string, string) {
	t.Helper()
	globalSettings := filepath.Join(fixture.agentDir, "settings.json")
	globalPackages := filepath.Join(fixture.agentDir, "packages")
	projectSettings := filepath.Join(fixture.worktree, ".pi/settings.json")
	projectPackages := filepath.Join(fixture.worktree, ".pi/packages")
	for _, directory := range []string{globalPackages, projectPackages} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	declarations := []byte("{\"packages\":[\"npm:missing-package\",\"https://example.invalid/missing.git\"]}\n")
	for _, path := range []string{globalSettings, projectSettings} {
		if err := os.WriteFile(path, declarations, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	extension := filepath.Join(fixture.worktree, "hostile-package-extension.ts")
	if err := os.WriteFile(extension, []byte(`import { writeFileSync } from "node:fs";
export default function () { writeFileSync(process.env.DEN_PI_HOSTILE_MARKER!, "reached\\n"); }
`), 0o600); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(fixture.root, "hostile-bin")
	if err := os.Mkdir(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	requestMarker := filepath.Join(bin, "package-resolution.requested")
	for _, name := range []string{"npm", "git"} {
		script := "#!/bin/sh\nprintf requested > " + shellQuote(requestMarker) + "\nexit 97\n"
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return []string{globalSettings, globalPackages, projectSettings, projectPackages,
		filepath.Join(fixture.agentDir, "npm"), filepath.Join(fixture.agentDir, "git"),
		filepath.Join(fixture.worktree, ".pi/npm"), filepath.Join(fixture.worktree, ".pi/git")}, extension, requestMarker
}

// This observer delegates unchanged to the real packaged Fence. Reserved inputs
// must not reach even its capability probe; allowed controls prove it is live.
func observingFence(t *testing.T, fixture *piFixture) (string, string) {
	t.Helper()
	marker := filepath.Join(fixture.root, "fence.started")
	path := filepath.Join(fixture.root, "observing-fence")
	contents := "#!/bin/sh\nprintf started > \"$DEN_PI_FENCE_MARKER\"\nexec " + shellQuote(os.Getenv("DEN_NATIVE_FENCE")) + " \"$@\"\n"
	if err := os.WriteFile(path, []byte(contents), 0o700); err != nil {
		t.Fatal(err)
	}
	return path, marker
}

func requireExactDenPackageDenial(t *testing.T, result commandResult, binary string) {
	t.Helper()
	expected := "Pi package commands are disabled by Den"
	if binary == "direct" {
		expected = "Pi package mutation is disabled by Den"
	}
	output := result.stdout + result.stderr
	if result.err == nil || !strings.Contains(output, expected) {
		t.Fatalf("package command did not receive exact rejection %q: %v\n%s", expected, result.err, output)
	}
}

func snapshotPaths(t *testing.T, paths ...string) string {
	t.Helper()
	hash := sha256.New()
	for _, root := range paths {
		_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				fmt.Fprintf(hash, "error:%s:%v\n", path, err)
				return nil
			}
			relative, _ := filepath.Rel(root, path)
			fmt.Fprintf(hash, "%s:%s\n", root, relative)
			if !entry.IsDir() {
				contents, readErr := os.ReadFile(path)
				if readErr != nil {
					fmt.Fprintf(hash, "read-error:%v\n", readErr)
				} else {
					hash.Write(contents)
				}
			}
			return nil
		})
	}
	return hex.EncodeToString(hash.Sum(nil))
}
