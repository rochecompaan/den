//go:build native && darwin

package native

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func testValidationTimePathSwap(t *testing.T, fixture *nativeFixture) {
	t.Helper()
	validationTimePathSwap(t, fixture,
		[]string{"/bin/ls", "-lde"},
		[]string{"/bin/chmod", "+a", "everyone allow read,readattr,readextattr,readsecurity"},
	)
}

func launchRegistryFixture(t *testing.T, fixture *nativeFixture, port string) commandResult {
	t.Helper()
	return fixture.launch("network-allow", fixtureURL(port, "registry.npmjs.org"))
}

func testImplicitHostWrites(t *testing.T, fixture *nativeFixture) {
	t.Helper()
	paths := []string{
		outsideFenceDarwinProbe(t, "/tmp/fence"),
		outsideFenceDarwinProbe(t, "/private/tmp/fence"),
	}
	arguments := append([]string{"implicit-host-darwin"}, paths...)
	requireSuccess(t, fixture.launch(arguments...))
	for _, path := range paths {
		if fileExists(path) {
			t.Fatal("Fence created an implicit host temporary artifact")
		}
	}
}

func outsideFenceDarwinProbe(t *testing.T, parent string) string {
	t.Helper()
	_, err := os.Stat(parent)
	createdParent := os.IsNotExist(err)
	if err != nil && !createdParent {
		t.Fatal("outside-Fence host temporary control is unavailable")
	}
	if createdParent {
		if err := os.Mkdir(parent, 0o700); err != nil {
			t.Fatal("outside-Fence host temporary parent is not writable")
		}
		t.Cleanup(func() { _ = os.Remove(parent) })
	}
	path := filepath.Join(parent, fmt.Sprintf("den-native-host-control-%d", os.Getpid()))
	if err := os.WriteFile(path, []byte("outside\n"), 0o600); err != nil {
		t.Fatal("outside-Fence host temporary control is not writable")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal("outside-Fence host temporary control cleanup failed")
	}
	return path
}

func TestDarwinACLGrantRejected(t *testing.T) {
	fixture := newNativeFixture(t)
	state := filepath.Join(fixture.root, "acl-state")
	if err := os.Mkdir(state, 0o700); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("/bin/chmod", "+a", "everyone allow read,readattr,readextattr,readsecurity", state).CombinedOutput(); err != nil {
		t.Fatalf("apply real macOS ACL: %v\n%s", err, output)
	}
	marker := filepath.Join(fixture.worktree, "acl-started")
	result := fixture.launchWith([]string{"CLAUDE_CONFIG_DIR=" + state}, "marker", marker)
	if result.err == nil || fileExists(marker) {
		t.Fatalf("macOS ACL grant did not fail closed: %v\n%s", result.err, result.stderr)
	}
}

func TestDarwinBashHook(t *testing.T) {
	fixture := newNativeFixture(t)
	replacement := filepath.Join(fixture.worktree, "replacement-policy")
	if err := os.WriteFile(replacement, []byte("replacement\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mutations := map[string]string{
		"truncate": `: > "$DEN_FENCE_POLICY_FILE" 2>/dev/null || true`,
		"replace":  `cp "$DEN_NATIVE_REPLACEMENT" "$DEN_FENCE_POLICY_FILE" 2>/dev/null || true`,
		"rename":   `mv "$DEN_FENCE_POLICY_FILE" "$DEN_FENCE_POLICY_FILE.old" 2>/dev/null || true`,
		"chmod":    `chmod u+w "$DEN_FENCE_POLICY_FILE" 2>/dev/null || true`,
		"append":   `printf mutation >> "$DEN_FENCE_POLICY_FILE" 2>/dev/null || true`,
	}
	for name, mutation := range mutations {
		t.Run(name, func(t *testing.T) {
			marker := filepath.Join(fixture.worktree, "blocked-"+name)
			blocked := `gh repo create forbidden; printf escaped > "$DEN_NATIVE_BLOCKED_MARKER"`
			scenario := fixture.provider.register(mutation, blocked)
			result := fixture.launchClaude(scenario, []string{
				"DEN_NATIVE_REPLACEMENT=" + replacement,
				"DEN_NATIVE_BLOCKED_MARKER=" + marker,
			})
			requireSuccess(t, result)
			if fileExists(marker) {
				t.Fatal("later Bash tool escaped the mandatory hook after policy mutation")
			}
			if len(fixture.provider.scenarioResults(scenario)) != 2 {
				t.Fatal("Claude did not complete both mutation and later blocked Bash calls")
			}
		})
	}

	allowedMarker := filepath.Join(fixture.worktree, "allowed-rerouted")
	blockedMarker := filepath.Join(fixture.worktree, "allowed-sequence-blocked")
	allowed := fmt.Sprintf(`printf allowed > %s; printf 'scratch:%%s\n' "$DEN_FENCE_TMPDIR"`, strconv.Quote(allowedMarker))
	blocked := fmt.Sprintf(`gh repo create forbidden; printf escaped > %s`, strconv.Quote(blockedMarker))
	scenario := fixture.provider.register(allowed, blocked)
	requireSuccess(t, fixture.launchClaude(scenario, nil))
	results := fixture.provider.scenarioResults(scenario)
	if !fileExists(allowedMarker) || fileExists(blockedMarker) || len(results) != 2 || scratchPath(results[0]) == "" {
		t.Fatalf("packaged Claude did not reroute allowed and deny blocked Bash calls: %#v", results)
	}
}

func TestDarwinHookCannotBeSuppressed(t *testing.T) {
	fixture := newNativeFixture(t)
	for _, argument := range []string{"--bare", "--bare=value"} {
		t.Run(argument, func(t *testing.T) {
			marker := filepath.Join(fixture.worktree, "bare-escaped")
			scenario := fixture.provider.register(fmt.Sprintf(`printf escaped > %s`, strconv.Quote(marker)))
			result := fixture.launchClaudeWithArguments(scenario, nil, argument)
			if result.err == nil || !strings.Contains(result.stderr, "bypasses the mandatory macOS security hook") {
				t.Fatalf("hook-suppressing %q was not rejected: %v\n%s", argument, result.err, result.stderr)
			}
			if fileExists(marker) || len(fixture.provider.scenarioResults(scenario)) != 0 {
				t.Fatal("hook-sensitive provider call ran after bare-mode rejection")
			}
		})
	}

	marker := filepath.Join(fixture.worktree, "simple-escaped")
	scenario := fixture.provider.register(fmt.Sprintf(`gh repo create forbidden; printf escaped > %s`, strconv.Quote(marker)))
	result := fixture.launchClaude(scenario, []string{"CLAUDE_CODE_SIMPLE=1"})
	requireSuccess(t, result)
	if fileExists(marker) || len(fixture.provider.scenarioResults(scenario)) != 1 {
		t.Fatal("inherited CLAUDE_CODE_SIMPLE suppressed the mandatory Bash hook")
	}
}

func TestDarwinTemporaryDirectories(t *testing.T) {
	fixture := newNativeFixture(t)
	probePaths := outsideFencePositiveControls(t)
	const launches = 2
	scenarios := make([]*claudeScenario, launches)
	results := make([]commandResult, launches)
	for index := range scenarios {
		first := `set -eu
 test "$TMPDIR" = "$DEN_FENCE_TMPDIR"
 printf own > "$DEN_FENCE_TMPDIR/marker"
 printf 'scratch:%s\n' "$DEN_FENCE_TMPDIR"`
		scenarios[index] = fixture.provider.registerScratch(first)
	}
	var wait sync.WaitGroup
	for index := range scenarios {
		wait.Add(1)
		go func() {
			defer wait.Done()
			results[index] = fixture.launchClaude(scenarios[index], []string{
				"DEN_NATIVE_TMP_PROBE=" + probePaths[index][0],
				"DEN_NATIVE_PRIVATE_TMP_PROBE=" + probePaths[index][1],
			})
		}()
	}
	wait.Wait()

	paths := make(map[string]struct{}, launches)
	for index, result := range results {
		requireSuccess(t, result)
		toolResults := fixture.provider.scenarioResults(scenarios[index])
		if len(toolResults) != 2 || !strings.Contains(toolResults[1], "complete") {
			t.Fatalf("scratch tool exchange did not complete: %#v", toolResults)
		}
		first, second := scratchPath(toolResults[0]), scratchPath(toolResults[1])
		if first == "" || first != second {
			t.Fatalf("outer/hook/nested scratch path was not stable: %#v", toolResults)
		}
		paths[first] = struct{}{}
	}
	if len(paths) != launches {
		t.Fatalf("concurrent packaged-Claude launches shared scratch state: %#v", paths)
	}
}

func outsideFencePositiveControls(t *testing.T) [][2]string {
	t.Helper()
	probes := make([][2]string, 2)
	for index := range probes {
		probes[index] = [2]string{
			outsideFenceWritableFile(t, "/tmp/fence"),
			outsideFenceWritableFile(t, "/private/tmp/fence"),
		}
	}
	return probes
}

func (fixture *nativeFixture) launchClaude(scenario *claudeScenario, extraEnvironment []string) commandResult {
	return fixture.launchClaudeWithArguments(scenario, extraEnvironment)
}

func (fixture *nativeFixture) launchClaudeWithArguments(scenario *claudeScenario, extraEnvironment []string, arguments ...string) commandResult {
	commandArguments := []string{"-p", scenario.prompt(), "--output-format", "text"}
	commandArguments = append(commandArguments, arguments...)
	command := exec.Command(os.Getenv("DEN_NATIVE_CLAUDE"), commandArguments...)
	command.Dir = fixture.worktree
	providerEnvironment := []string{
		"ANTHROPIC_API_KEY=den-native-local-test-key",
		"ANTHROPIC_BASE_URL=" + fixtureURL(tlsRecorderPort, brokerHostname()),
		"NODE_EXTRA_CA_CERTS=" + fixture.certificate.ca,
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1",
	}
	providerEnvironment = append(providerEnvironment, extraEnvironment...)
	command.Env = fixture.launchEnvironment(providerEnvironment)
	return runCommand(command)
}
