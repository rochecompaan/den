//go:build native

package pi

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type commandResult struct {
	stdout string
	stderr string
	err    error
}

type piFixture struct {
	root, worktree, runtimeHome, agentDir, sessionDir, caFile string
}

func newPiFixture(t *testing.T) *piFixture {
	t.Helper()
	root, err := os.MkdirTemp(os.Getenv("DEN_NATIVE_HOST_ROOT"), "pi-native-")
	if err != nil {
		t.Fatal(err)
	}
	fixture := &piFixture{
		root: root, worktree: filepath.Join(root, "worktree"), runtimeHome: filepath.Join(root, "runtime-home"),
		agentDir: filepath.Join(root, "agent"), sessionDir: filepath.Join(root, "sessions"), caFile: filepath.Join(root, "ca.pem"),
	}
	for _, path := range []string{fixture.worktree, fixture.runtimeHome, fixture.agentDir, fixture.sessionDir} {
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
		"PI_CODING_AGENT_DIR":         "PI_CODING_AGENT_DIR=" + fixture.agentDir,
		"PI_CODING_AGENT_SESSION_DIR": "PI_CODING_AGENT_SESSION_DIR=" + fixture.sessionDir,
		"PI_PROVIDER_FIXTURE_TOKEN":   "PI_PROVIDER_FIXTURE_TOKEN=not-a-real-provider-credential",
		"REPOWOLF_ENDPOINT":            "REPOWOLF_ENDPOINT=https://broker.example.test/",
		"REPOWOLF_TOKEN":               "REPOWOLF_TOKEN=rw1_AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE",
		"REPOWOLF_CA_FILE":             "REPOWOLF_CA_FILE=" + fixture.caFile,
	}
	for _, entry := range extra {
		name, _, _ := strings.Cut(entry, "=")
		replacements[name] = entry
	}
	result := make([]string, 0, len(os.Environ())+len(replacements))
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if _, replaced := replacements[name]; !replaced {
			result = append(result, entry)
		}
	}
	for _, entry := range replacements {
		result = append(result, entry)
	}
	return result
}

func (fixture *piFixture) run(binary string, stdin string, extra []string, arguments ...string) commandResult {
	command := exec.Command(binary, arguments...)
	command.Dir = fixture.worktree
	command.Env = fixture.environment(extra...)
	command.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	return commandResult{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

func (fixture *piFixture) sandbox(stdin string, arguments ...string) commandResult {
	return fixture.run(os.Getenv("DEN_NATIVE_PI_SANDBOX"), stdin, nil, arguments...)
}

func (fixture *piFixture) direct(stdin string, arguments ...string) commandResult {
	return fixture.run(os.Getenv("DEN_NATIVE_PI"), stdin, nil, arguments...)
}

func (fixture *piFixture) reportPath() string {
	return filepath.Join(fixture.agentDir, "pi-resources.report")
}
