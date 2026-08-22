//go:build native && darwin

package native

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func testValidationTimePathSwap(t *testing.T, fixture *nativeFixture) {
	t.Helper()
	validationTimePathSwap(t, fixture, "/bin/ls", "-lde")
}

func launchRegistryFixture(t *testing.T, fixture *nativeFixture, port string) commandResult {
	t.Helper()
	return fixture.launch("network-allow", "--resolve", "registry.npmjs.org:"+port+":127.0.0.1", fixtureURL(port, "registry.npmjs.org"))
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
	requireSuccess(t, fixture.launch("policy-immutable"))

	allowed := runClaudeHook(t, "printf allowed")
	if allowed.Decision != "allow" || !strings.Contains(allowed.Command, "fence") || !strings.Contains(allowed.Command, "-c") {
		t.Fatalf("allowed Bash command was not rerouted through nested Fence: %#v", allowed)
	}
	blocked := runClaudeHook(t, "gh repo create forbidden")
	if blocked.Decision != "deny" || blocked.Command != "" {
		t.Fatalf("blocked Bash command escaped hook after policy mutations: %#v", blocked)
	}
}

func TestDarwinHookCannotBeSuppressed(t *testing.T) {
	fixture := newNativeFixture(t)
	for _, argument := range []string{"--bare", "--bare=value"} {
		command := exec.Command(os.Getenv("DEN_NATIVE_CLAUDE"), argument)
		command.Dir = fixture.worktree
		command.Env = replaceEnvironment(os.Environ(),
			"HOME="+fixture.home, "REPOWOLF_ENDPOINT="+fixture.repoWolfEndpoint,
			"REPOWOLF_TOKEN="+fixtureToken, "REPOWOLF_CA_FILE="+fixture.certificate.ca,
		)
		output, err := command.CombinedOutput()
		if err == nil || !strings.Contains(string(output), "bypasses the mandatory macOS security hook") {
			t.Fatalf("hook-suppressing %q was not rejected: %v\n%s", argument, err, output)
		}
	}

	command := exec.Command(os.Getenv("DEN_NATIVE_CLAUDE"), "--version")
	command.Dir = fixture.worktree
	command.Env = replaceEnvironment(os.Environ(),
		"HOME="+fixture.home, "REPOWOLF_ENDPOINT="+fixture.repoWolfEndpoint,
		"REPOWOLF_TOKEN="+fixtureToken, "REPOWOLF_CA_FILE="+fixture.certificate.ca,
		"CLAUDE_CODE_SIMPLE=1",
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("inherited CLAUDE_CODE_SIMPLE suppressed outer Claude/Fence launch: %v\n%s", err, output)
	}
}

func TestDarwinTemporaryDirectories(t *testing.T) {
	fixture := newNativeFixture(t)
	const launches = 2
	results := make([]commandResult, launches)
	var wait sync.WaitGroup
	for index := range results {
		wait.Add(1)
		go func() {
			defer wait.Done()
			results[index] = fixture.launch("scratch")
		}()
	}
	wait.Wait()
	paths := make(map[string]struct{}, launches)
	for _, result := range results {
		requireSuccess(t, result)
		var outer, nested string
		for _, line := range strings.Split(result.stdout, "\n") {
			if strings.HasPrefix(line, "outer:") {
				outer = strings.TrimPrefix(line, "outer:")
			}
			if strings.HasPrefix(line, "nested:") {
				nested = strings.TrimPrefix(line, "nested:")
			}
		}
		if outer == "" || outer != nested {
			t.Fatalf("outer and nested Fence did not share DEN_FENCE_TMPDIR: %q", result.stdout)
		}
		paths[outer] = struct{}{}
	}
	if len(paths) != launches {
		t.Fatalf("concurrent launches shared temporary state: %#v", paths)
	}
}

type hookResult struct {
	Decision string
	Command  string
}

func runClaudeHook(t *testing.T, commandText string) hookResult {
	t.Helper()
	manifestBytes, err := os.ReadFile(os.Getenv("DEN_NATIVE_MANIFEST"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		BasePolicy string `json:"basePolicy"`
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	input, err := json.Marshal(map[string]any{
		"hook_event_name": "PreToolUse", "tool_name": "Bash",
		"tool_input": map[string]any{"command": commandText},
	})
	if err != nil {
		t.Fatal(err)
	}
	process := exec.Command(os.Getenv("DEN_NATIVE_FENCE"), "--claude-pre-tool-use", "--settings", manifest.BasePolicy)
	process.Stdin = bytes.NewReader(input)
	output, err := process.Output()
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		HookSpecificOutput struct {
			PermissionDecision string `json:"permissionDecision"`
			UpdatedInput       struct {
				Command string `json:"command"`
			} `json:"updatedInput"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(output, &response); err != nil {
		t.Fatalf("decode hook response %q: %v", output, err)
	}
	return hookResult{Decision: response.HookSpecificOutput.PermissionDecision, Command: response.HookSpecificOutput.UpdatedInput.Command}
}
