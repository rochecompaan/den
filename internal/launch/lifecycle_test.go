package launch

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/rochecompaan/den/internal/manifest"
)

func TestRunStartsFenceWithReadOnlyPolicyAndCleansTemporaryDirectories(t *testing.T) {
	requireTrustedScratchRoot(t)
	root := t.TempDir()
	ca := filepath.Join(root, "ca.pem")
	base := filepath.Join(root, "base.json")
	closures := filepath.Join(root, "closures")
	fence := filepath.Join(root, "fence")
	agent := filepath.Join(root, "agent")
	for path, contents := range map[string]string{
		ca:       "certificate",
		base:     "{}\n",
		closures: root + "\n",
		fence: `#!/bin/sh
set -eu
printf '%s\n' "$@" > "$FENCE_ARGS"
printf '%s' "$DEN_FENCE_POLICY_FILE" > "$FENCE_POLICY"
stat -c %a "$DEN_FENCE_POLICY_FILE" > "$FENCE_MODE"
while [ "$1" != -- ]; do shift; done
shift
exec "$@"
`,
		agent: "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$AGENT_ARGS\"\nprintf '%s' \"$TMPDIR\" > \"$AGENT_TMP\"\nexit 17\n",
	} {
		mode := os.FileMode(0o600)
		if path == fence || path == agent {
			mode = 0o700
		}
		if err := os.WriteFile(path, []byte(contents), mode); err != nil {
			t.Fatal(err)
		}
	}
	fenceArgs := filepath.Join(root, "fence-args")
	policyPath := filepath.Join(root, "policy-path")
	policyMode := filepath.Join(root, "policy-mode")
	agentArgs := filepath.Join(root, "agent-args")
	agentTMP := filepath.Join(root, "agent-tmp")
	for name, value := range map[string]string{
		"REPOWOLF_ENDPOINT": "https://broker.example.test/",
		"REPOWOLF_TOKEN":    "rw1_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"REPOWOLF_CA_FILE":  ca,
		"FENCE_ARGS":        fenceArgs,
		"FENCE_POLICY":      policyPath,
		"FENCE_MODE":        policyMode,
		"AGENT_ARGS":        agentArgs,
		"AGENT_TMP":         agentTMP,
	} {
		t.Setenv(name, value)
	}
	stat, err := exec.LookPath("stat")
	if err != nil {
		t.Fatal(err)
	}
	code := Run(context.Background(), manifest.Manifest{
		Platform: "darwin", FenceExecutable: fence, BasePolicy: base, ClosurePathsFile: closures, ScratchRoot: "/tmp", PathEntries: []string{filepath.Dir(stat)},
		Agent: manifest.Agent{Name: "test", Executable: agent, MandatoryArgs: []string{"--mandatory"}},
	}, []string{"--user=value"})
	if code != 17 {
		t.Fatalf("Run() = %d, want 17", code)
	}
	if got := strings.TrimSpace(readLifecycleFile(t, policyMode)); got != "400" {
		t.Fatalf("policy mode = %q, want 400", got)
	}
	policy := readLifecycleFile(t, policyPath)
	if _, err := os.Lstat(policy); !os.IsNotExist(err) {
		t.Fatalf("policy was not cleaned up: %v", err)
	}
	tmp := readLifecycleFile(t, agentTMP)
	if !strings.Contains(tmp, "/den-") || !strings.Contains(tmp, "/scratch-") {
		t.Fatalf("TMPDIR = %q, want private scratch directory", tmp)
	}
	if got := strings.Split(strings.TrimSpace(readLifecycleFile(t, agentArgs)), "\n"); strings.Join(got, ",") != "--mandatory,--user=value" {
		t.Fatalf("agent arguments = %#v", got)
	}
	gotFenceArgs := strings.Split(strings.TrimSpace(readLifecycleFile(t, fenceArgs)), "\n")
	if len(gotFenceArgs) < 7 || gotFenceArgs[0] != "--settings" || gotFenceArgs[2] != "--expose-host-path" || gotFenceArgs[4] != "--" || gotFenceArgs[5] != agent {
		t.Fatalf("Fence arguments = %#v", gotFenceArgs)
	}
}

func TestRunRejectsDarwinSecurityOverridesBeforeStartingFence(t *testing.T) {
	for name, testCase := range map[string]struct {
		arguments []string
		settings  string
	}{
		"reserved":         {arguments: []string{"--settings=override"}},
		"bare":             {arguments: []string{"--bare=true"}},
		"hook replacement": {settings: `{"hooks":{"PreToolUse":[{"hooks":[{"command":"fence --claude-pre-tool-use"}]}]}}`},
	} {
		t.Run(name, func(t *testing.T) {
			arguments, settings := testCase.arguments, testCase.settings
			root := t.TempDir()
			home := filepath.Join(root, "home")
			if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o700); err != nil {
				t.Fatal(err)
			}
			if settings != "" {
				if err := os.WriteFile(filepath.Join(home, ".claude", "settings.json"), []byte(settings), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			marker := filepath.Join(root, "fence-started")
			fence := filepath.Join(root, "fence")
			if err := os.WriteFile(fence, []byte("#!/bin/sh\ntouch \"$FENCE_MARKER\"\n"), 0o700); err != nil {
				t.Fatal(err)
			}
			ca := filepath.Join(root, "ca.pem")
			if err := os.WriteFile(ca, []byte("certificate"), 0o600); err != nil {
				t.Fatal(err)
			}
			for key, value := range map[string]string{
				"HOME": home, "FENCE_MARKER": marker,
				"REPOWOLF_ENDPOINT": "https://broker.example.test/", "REPOWOLF_TOKEN": "rw1_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", "REPOWOLF_CA_FILE": ca,
			} {
				t.Setenv(key, value)
			}
			code := Run(context.Background(), manifest.Manifest{Platform: "darwin", FenceExecutable: fence, Agent: manifest.Agent{Name: "claude"}}, arguments)
			if code != 1 {
				t.Fatalf("Run() = %d, want 1", code)
			}
			if _, err := os.Lstat(marker); !os.IsNotExist(err) {
				t.Fatalf("Fence started after %s: %v", name, err)
			}
		})
	}
}

func requireTrustedScratchRoot(t *testing.T) {
	t.Helper()
	info, err := os.Lstat("/tmp")
	if err != nil {
		t.Skip("sandbox does not provide a trusted /tmp")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || info.Mode()&os.ModeSticky == 0 || stat.Uid != 0 {
		t.Skip("sandbox does not provide a trusted /tmp")
	}
}

func readLifecycleFile(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}
