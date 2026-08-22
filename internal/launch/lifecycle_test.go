package launch

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/rochecompaan/den/internal/configdir"
	"github.com/rochecompaan/den/internal/container"
	"github.com/rochecompaan/den/internal/environment"
	"github.com/rochecompaan/den/internal/manifest"
	"github.com/rochecompaan/den/internal/repowolf"
)

func TestRunStartsFenceWithReadOnlyPolicyAndCleansTemporaryDirectories(t *testing.T) {
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
		agent: "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$AGENT_ARGS\"\nprintf '%s' \"$TMPDIR\" > \"$AGENT_TMP\"\nprintf '%s' \"$REPOWOLF_CA_FILE\" > \"$AGENT_CA\"\nexit 17\n",
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
	agentCA := filepath.Join(root, "agent-ca")
	for name, value := range map[string]string{
		"REPOWOLF_ENDPOINT": "https://broker.example.test/",
		"REPOWOLF_TOKEN":    "rw1_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"REPOWOLF_CA_FILE":  ca,
		"FENCE_ARGS":        fenceArgs,
		"FENCE_POLICY":      policyPath,
		"FENCE_MODE":        policyMode,
		"AGENT_ARGS":        agentArgs,
		"AGENT_TMP":         agentTMP,
		"AGENT_CA":          agentCA,
	} {
		t.Setenv(name, value)
	}
	stat, err := exec.LookPath("stat")
	if err != nil {
		t.Fatal(err)
	}
	code := runWithLifecycle(context.Background(), manifest.Manifest{
		Platform: "darwin", FenceExecutable: fence, BasePolicy: base, ClosurePathsFile: closures, ScratchRoot: "/test-only", PathEntries: []string{filepath.Dir(stat)},
		Agent: manifest.Agent{Name: "test", Executable: agent, MandatoryArgs: []string{"--mandatory"}},
	}, []string{"--user=value"}, os.LookupEnv, os.Lstat, os.Environ, environment.Build, os.Stderr,
		func(ctx context.Context, launcherManifest manifest.Manifest, arguments []string, config repowolf.Config, selection configdir.Selection, revalidate func() error, childEnvironment []string, docker, podman container.Socket, stderr io.Writer) int {
			return runFenceWithTemporary(ctx, launcherManifest, arguments, config, selection, revalidate, childEnvironment, docker, podman, stderr,
				func(string, int, time.Duration) error { return nil }, testTemporaryPair(root))
		},
	)
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
	preparedCA := readLifecycleFile(t, agentCA)
	if gotFenceArgs[3] != preparedCA || preparedCA == ca || filepath.Base(preparedCA) != "repowolf-ca.pem" {
		t.Fatalf("CA consumers were not pinned: args=%#v env=%q", gotFenceArgs, preparedCA)
	}
	if _, err := os.Lstat(preparedCA); !os.IsNotExist(err) {
		t.Fatalf("prepared CA was not cleaned up: %v", err)
	}
}

func TestLifecycleCommitControlsCustomConfigurationRollback(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config")
	ca := filepath.Join(root, "ca.pem")
	probe := filepath.Join(root, "acl-probe")
	if err := os.WriteFile(ca, []byte("certificate"), 0o400); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(probe, []byte("#!/bin/sh\nprintf 'user::rwx\\ngroup::---\\nother::---\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	values := map[string]string{
		"REPOWOLF_ENDPOINT": "https://broker.example.test/", "REPOWOLF_TOKEN": "rw1_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", "REPOWOLF_CA_FILE": ca, "HOME": root,
	}
	for name, commit := range map[string]bool{"pre-start failure rolls back": false, "started Fence commits": true} {
		t.Run(name, func(t *testing.T) {
			_ = os.RemoveAll(configPath)
			code := runWithLifecycle(context.Background(), manifest.Manifest{ExplicitConfigDir: &configPath, ACLProbe: []string{probe}}, nil, lookup(values), os.Lstat, os.Environ, environment.Build, &bytes.Buffer{},
				func(_ context.Context, _ manifest.Manifest, _ []string, _ repowolf.Config, selection configdir.Selection, _ func() error, _ []string, _, _ container.Socket, _ io.Writer) int {
					if commit {
						selection.Commit()
						return 17
					}
					return 1
				})
			if want := map[bool]int{false: 1, true: 17}[commit]; code != want {
				t.Fatalf("runWithLifecycle() = %d, want %d", code, want)
			}
			_, err := os.Lstat(configPath)
			if commit && err != nil {
				t.Fatalf("committed config was rolled back: %v", err)
			}
			if !commit && !os.IsNotExist(err) {
				t.Fatalf("uncommitted config remains: %v", err)
			}
		})
	}
}

func TestRunFencePreservesChildStatusWhenTemporaryCleanupFails(t *testing.T) {
	root := t.TempDir()
	ca := filepath.Join(root, "ca.pem")
	base := filepath.Join(root, "base.json")
	closures := filepath.Join(root, "closures")
	fence := filepath.Join(root, "fence")
	agent := filepath.Join(root, "agent")
	for path, contents := range map[string]string{
		ca: "certificate", base: "{}\n", closures: root + "\n",
		fence: "#!/bin/sh\nwhile [ \"$1\" != -- ]; do shift; done\nshift\nexec \"$@\"\n",
		agent: "#!/bin/sh\nexit 17\n",
	} {
		mode := os.FileMode(0o600)
		if path == fence || path == agent {
			mode = 0o700
		}
		if err := os.WriteFile(path, []byte(contents), mode); err != nil {
			t.Fatal(err)
		}
	}
	policyDir, scratchDir := filepath.Join(root, "policy"), filepath.Join(root, "scratch")
	if err := os.Mkdir(policyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(scratchDir, 0o700); err != nil {
		t.Fatal(err)
	}
	code := runFenceWithTemporary(context.Background(), manifest.Manifest{
		Platform: "darwin", FenceExecutable: fence, BasePolicy: base, ClosurePathsFile: closures, ScratchRoot: "/unused", PathEntries: []string{filepath.Dir(fence)},
		Agent: manifest.Agent{Executable: agent},
	}, nil, repowolf.Config{Hostname: "broker.example.test", CAFile: ca}, configdir.Selection{}, nil,
		[]string{"PATH=" + filepath.Dir(fence)}, container.Socket{}, container.Socket{}, &bytes.Buffer{},
		func(string, int, time.Duration) error { return nil },
		func(string) (string, string, func() error, error) {
			return policyDir, scratchDir, func() error { return errors.New("cleanup failed") }, nil
		},
	)
	if code != 17 {
		t.Fatalf("runFenceWithTemporary() = %d, want child status 17", code)
	}
}

func TestRunRejectsIncompleteFenceCommand(t *testing.T) {
	root := t.TempDir()
	ca := filepath.Join(root, "ca.pem")
	fence := filepath.Join(root, "fence")
	marker := filepath.Join(root, "started")
	for path, contents := range map[string]string{ca: "certificate", fence: "#!/bin/sh\ntouch \"$FENCE_MARKER\"\n"} {
		mode := os.FileMode(0o600)
		if path == fence {
			mode = 0o700
		}
		if err := os.WriteFile(path, []byte(contents), mode); err != nil {
			t.Fatal(err)
		}
	}
	for key, value := range map[string]string{
		"REPOWOLF_ENDPOINT": "https://broker.example.test/", "REPOWOLF_TOKEN": "rw1_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", "REPOWOLF_CA_FILE": ca, "FENCE_MARKER": marker,
	} {
		t.Setenv(key, value)
	}
	if code := Run(context.Background(), manifest.Manifest{Platform: "darwin", FenceExecutable: fence}, nil); code != 1 {
		t.Fatalf("Run() = %d, want 1", code)
	}
	if _, err := os.Lstat(marker); !os.IsNotExist(err) {
		t.Fatalf("Fence started with incomplete command: %v", err)
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

func testTemporaryPair(root string) temporaryPair {
	return func(string) (string, string, func() error, error) {
		parent := filepath.Join(root, "den-"+strconv.Itoa(os.Getuid()))
		if err := os.MkdirAll(parent, 0o700); err != nil {
			return "", "", nil, err
		}
		policyDir, err := os.MkdirTemp(parent, "policy-")
		if err != nil {
			return "", "", nil, err
		}
		scratchDir, err := os.MkdirTemp(parent, "scratch-")
		if err != nil {
			_ = os.RemoveAll(policyDir)
			return "", "", nil, err
		}
		return policyDir, scratchDir, func() error {
			if err := os.RemoveAll(policyDir); err != nil {
				return err
			}
			return os.RemoveAll(scratchDir)
		}, nil
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
