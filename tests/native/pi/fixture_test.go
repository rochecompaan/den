//go:build native

package pi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// PTY output is inspected while the command's writer goroutine is active.
type lockedBuffer struct {
	mu       sync.Mutex
	contents bytes.Buffer
}

func (buffer *lockedBuffer) Write(data []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.contents.Write(data)
}
func (buffer *lockedBuffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.contents.String()
}

type commandResult struct {
	stdout string
	stderr string
	err    error
}

type piFixture struct {
	root, worktree, invokingHome, runtimeHome, agentDir, sessionDir, caFile string
}

func newPiFixture(t *testing.T) *piFixture {
	t.Helper()
	root, err := os.MkdirTemp(os.Getenv("DEN_NATIVE_HOST_ROOT"), "pi-native-")
	if err != nil {
		t.Fatal(err)
	}
	fixture := &piFixture{
		root: root, worktree: filepath.Join(root, "worktree"), invokingHome: filepath.Join(root, "worktree/invoking-home"), runtimeHome: filepath.Join(root, "worktree/runtime-home"),
		agentDir: filepath.Join(root, "agent"), sessionDir: filepath.Join(root, "sessions"), caFile: filepath.Join(root, "ca.pem"),
	}
	for _, path := range []string{fixture.worktree, fixture.invokingHome, fixture.runtimeHome, fixture.agentDir, fixture.sessionDir} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(fixture.caFile, []byte("fixture CA\\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	return fixture
}

func (fixture *piFixture) environment(extra ...string) []string {
	replacements := map[string]string{
		"HOME":                        "HOME=" + fixture.runtimeHome,
		"DEN_NATIVE_INVOKING_HOME":    "DEN_NATIVE_INVOKING_HOME=" + fixture.invokingHome,
		"PI_CODING_AGENT_DIR":         "PI_CODING_AGENT_DIR=" + fixture.agentDir,
		"PI_CODING_AGENT_SESSION_DIR": "PI_CODING_AGENT_SESSION_DIR=" + fixture.sessionDir,
		"PI_PROVIDER_FIXTURE_TOKEN":   "PI_PROVIDER_FIXTURE_TOKEN=not-a-real-provider-credential",
		"REPOWOLF_ENDPOINT":           "REPOWOLF_ENDPOINT=https://broker.example.test/",
		"REPOWOLF_TOKEN":              "REPOWOLF_TOKEN=rw1_AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE",
		"REPOWOLF_CA_FILE":            "REPOWOLF_CA_FILE=" + fixture.caFile,
	}
	removed := make(map[string]bool)
	for _, entry := range extra {
		name, _, present := strings.Cut(entry, "=")
		if !present {
			removed[name] = true
			delete(replacements, name)
			continue
		}
		replacements[name] = entry
	}
	result := make([]string, 0, len(os.Environ())+len(replacements))
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if _, replaced := replacements[name]; !replaced && !removed[name] {
			result = append(result, entry)
		}
	}
	for _, entry := range replacements {
		result = append(result, entry)
	}
	return result
}

func (fixture *piFixture) run(binary string, stdin string, extra []string, arguments ...string) commandResult {
	if stdin != "" && slicesContain(arguments, "rpc") {
		return fixture.runRPC(binary, stdin, extra, arguments...)
	}
	command := exec.Command(binary, arguments...)
	command.Dir = fixture.worktree
	command.Env = fixture.environment(extra...)
	command.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	return commandResult{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

// RPC EOF is sent only after the requested response (and model turn, if any).
// Immediate EOF races Pi's asynchronous switch/prompt against runtime disposal.
func (fixture *piFixture) runRPC(binary, input string, extra []string, arguments ...string) commandResult {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, arguments...)
	command.Dir, command.Env = fixture.worktree, fixture.environment(extra...)
	stdin, err := command.StdinPipe()
	if err != nil {
		return commandResult{err: err}
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return commandResult{err: err}
	}
	var output, errors bytes.Buffer
	command.Stderr = &errors
	pending := map[string]bool{}
	turns := 0
	for _, line := range strings.Split(strings.TrimSpace(input), "\n") {
		var request struct{ ID, Type, Message string }
		if json.Unmarshal([]byte(line), &request) != nil {
			return commandResult{err: fmt.Errorf("invalid fixture RPC request")}
		}
		pending[request.ID] = true
		if request.Type == "prompt" && !strings.HasPrefix(request.Message, "/") {
			turns++
		}
	}
	if err := command.Start(); err != nil {
		return commandResult{err: err}
	}
	_, _ = fmt.Fprint(stdin, input)
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		fmt.Fprintln(&output, line)
		var response struct {
			ID, Type string
			Success  bool
		}
		if json.Unmarshal([]byte(line), &response) == nil {
			if response.Type == "response" {
				delete(pending, response.ID)
				if !response.Success {
					turns = 0
				}
			}
			if response.Type == "agent_end" {
				turns--
			}
		}
		if len(pending) == 0 && turns <= 0 {
			_ = stdin.Close()
		}
	}
	_ = stdin.Close()
	err = command.Wait()
	if err == nil {
		err = scanner.Err()
	}
	return commandResult{stdout: output.String(), stderr: errors.String(), err: err}
}

func slicesContain(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func (fixture *piFixture) sandbox(stdin string, arguments ...string) commandResult {
	return fixture.run(os.Getenv("DEN_NATIVE_PI_SANDBOX"), stdin, nil, arguments...)
}

func (fixture *piFixture) direct(stdin string, arguments ...string) commandResult {
	return fixture.run(os.Getenv("DEN_NATIVE_PI"), stdin, nil, arguments...)
}

func (fixture *piFixture) launch(stdin string, extra []string, mutate func(map[string]any), arguments ...string) commandResult {
	manifest, err := fixture.manifest(mutate)
	if err != nil {
		return commandResult{err: err}
	}
	launcherArgs := append([]string{"--manifest", manifest, "--"}, arguments...)
	return fixture.run(os.Getenv("DEN_NATIVE_LAUNCHER"), stdin, extra, launcherArgs...)
}

func (fixture *piFixture) manifest(mutate func(map[string]any)) (string, error) {
	contents, err := os.ReadFile(os.Getenv("DEN_NATIVE_PI_MANIFEST"))
	if err != nil {
		return "", err
	}
	var document map[string]any
	if err := json.Unmarshal(contents, &document); err != nil {
		return "", err
	}
	if mutate != nil {
		mutate(document)
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return "", err
	}
	path := filepath.Join(fixture.root, fmt.Sprintf("pi-manifest-%d.json", time.Now().UnixNano()))
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func (fixture *piFixture) rpc(arguments []string, requests ...string) commandResult {
	input := strings.Join(requests, "\n")
	if input != "" {
		input += "\n"
	}
	return fixture.sandbox(input, append([]string{"--mode", "rpc"}, arguments...)...)
}

func (fixture *piFixture) reportPath() string {
	return filepath.Join(fixture.agentDir, "pi-resources.report")
}

func writeSession(t *testing.T, directory, name, id, cwd string) string {
	t.Helper()
	path := filepath.Join(directory, name)
	contents := fmt.Sprintf("{\"type\":\"session\",\"version\":3,\"id\":\"00000000-0000-4000-8000-000000000001\",\"name\":%q,\"timestamp\":\"2026-09-05T00:00:00.000Z\",\"cwd\":%q}\n", id, cwd)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func rpcInput(records ...string) string { return strings.Join(records, "\n") + "\n" }

func rpcSwitch(id, path string) string {
	encoded, _ := json.Marshal(map[string]string{"id": id, "type": "switch_session", "sessionPath": path})
	return string(encoded)
}

func requireRPCSuccess(t *testing.T, result commandResult) {
	t.Helper()
	requireRPCResponse(t, result, "valid", true, "")
}

func requireRPCResponse(t *testing.T, result commandResult, id string, success bool, message string) {
	t.Helper()
	if result.err != nil {
		t.Fatalf("RPC process failed: %v\n%s%s", result.err, result.stdout, result.stderr)
	}
	for _, line := range strings.Split(result.stdout, "\n") {
		var response struct {
			ID      string `json:"id"`
			Success bool   `json:"success"`
			Type    string `json:"type"`
			Error   string `json:"error"`
		}
		if json.Unmarshal([]byte(line), &response) == nil && response.Type == "response" && response.ID == id {
			if response.Success != success || !strings.Contains(response.Error, message) {
				t.Fatalf("unexpected correlated RPC response: %s", line)
			}
			return
		}
	}
	t.Fatalf("missing RPC response %q: %s%s", id, result.stdout, result.stderr)
}

func reportHasLine(contents []byte, line string) bool {
	return slicesContain(strings.Split(string(contents), "\n"), line)
}

func requireReportLines(t *testing.T, path string, lines ...string) {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range lines {
		if !reportHasLine(contents, line) {
			t.Fatalf("missing report line %q in %q", line, contents)
		}
	}
}

func requireDeniedWith(t *testing.T, result commandResult, fragment string) {
	t.Helper()
	if result.err == nil || !strings.Contains(strings.ToLower(result.stdout+result.stderr), strings.ToLower(fragment)) {
		t.Fatalf("expected denial containing %q: %v\n%s%s", fragment, result.err, result.stdout, result.stderr)
	}
}

func waitForReportLine(t *testing.T, path, line string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if contents, err := os.ReadFile(path); err == nil && reportHasLine(contents, line) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %q in %s", line, path)
}

func directoryHasEntries(t *testing.T, path string) bool {
	t.Helper()
	entries, err := os.ReadDir(path)
	if os.IsNotExist(err) {
		return false
	}
	if err != nil {
		t.Fatal(err)
	}
	return len(entries) > 0
}

func pathExists(path string) bool { _, err := os.Lstat(path); return err == nil }

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }
