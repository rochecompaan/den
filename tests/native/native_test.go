//go:build native

package native

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

var requiredEnvironment = []string{
	"DEN_NATIVE_CLAUDE",
	"DEN_NATIVE_SANDBOX",
	"DEN_NATIVE_MANIFEST",
	"DEN_NATIVE_LAUNCHER",
	"DEN_NATIVE_FENCE",
	"DEN_NATIVE_REPOWOLF_CLIENT_DIR",
	"DEN_NATIVE_REPOWOLF_FIXTURE",
	"DEN_NATIVE_UNRELATED_STORE_FILE",
}

func TestMain(m *testing.M) {
	for _, name := range requiredEnvironment {
		value := os.Getenv(name)
		if value == "" {
			fmt.Fprintf(os.Stderr, "native enforcement requires packaged fixture environment: %s is unset\n", name)
			os.Exit(1)
		}
		if strings.Contains(name, "ENDPOINT") || strings.Contains(name, "TOKEN") {
			continue
		}
		if !filepath.IsAbs(value) {
			fmt.Fprintf(os.Stderr, "native enforcement requires absolute packaged path: %s\n", name)
			os.Exit(1)
		}
	}
	os.Exit(m.Run())
}

func TestPackagedFenceIsReal(t *testing.T) {
	output, err := exec.Command(os.Getenv("DEN_NATIVE_FENCE"), "--version").CombinedOutput()
	if err != nil {
		t.Fatalf("packaged Fence: %v\n%s", err, output)
	}
	version := string(output)
	if !strings.Contains(version, "Version: 0.1.58") || strings.Contains(strings.ToLower(version), "fake") {
		t.Fatalf("unexpected packaged Fence version: %s", version)
	}
}

func TestNetworkEnforcement(t *testing.T) {
	fixture := newNativeFixture(t)
	fixture.startDNS(t)
	port := fixture.tlsPort(t)

	t.Run("broker", func(t *testing.T) {
		before := fixture.requestCount()
		result := fixture.launch("network-allow", fixtureURL(port, brokerHostname()))
		if result.err != nil {
			t.Fatalf("broker launch: %v\n%s\nDNS: %v\nrequests: %d", result.err, result.stderr, fixture.dns.names(), fixture.requestCount())
		}
		if fixture.requestCount() != before+1 {
			t.Fatal("allowed broker did not reach local TLS fixture")
		}
	})

	t.Run("registry", func(t *testing.T) {
		before := fixture.requestCount()
		result := launchRegistryFixture(t, fixture, port)
		if result.err != nil {
			t.Fatalf("registry launch: %v\n%s\nDNS: %v\nrequests: %d", result.err, result.stderr, fixture.dns.names(), fixture.requestCount())
		}
		if fixture.requestCount() != before+1 {
			t.Fatal("allowed registry did not reach local TLS fixture")
		}
	})

	for _, host := range []string{"github.com", "gitlab.com", "bitbucket.org"} {
		t.Run("denied_"+host, func(t *testing.T) {
			before := fixture.requestCount()
			requireSuccess(t, fixture.launch("network-deny", host, port))
			if fixture.requestCount() != before {
				t.Fatalf("denied host %s reached local recorder", host)
			}
		})
	}
	if fixture.dns != nil {
		for _, name := range fixture.dns.names() {
			if name != brokerHostname() && name != "registry.npmjs.org" {
				t.Fatalf("fixture attempted non-local DNS path %q", name)
			}
		}
	}
}

func TestFilesystemEnforcement(t *testing.T) {
	fixture := newNativeFixture(t)
	secret := filepath.Join(fixture.root, "credential")
	if err := os.WriteFile(secret, []byte("credential-marker"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("credential_symlinks", func(t *testing.T) {
		worktreeLink := filepath.Join(fixture.worktree, "credential-link")
		if err := os.Symlink(secret, worktreeLink); err != nil {
			t.Fatal(err)
		}
		requireSuccess(t, fixture.launch("credential-deny", worktreeLink))

		state := filepath.Join(fixture.root, "state")
		if err := os.Mkdir(state, 0o700); err != nil {
			t.Fatal(err)
		}
		stateLink := filepath.Join(state, "credential-link")
		if err := os.Symlink(secret, stateLink); err != nil {
			t.Fatal(err)
		}
		requireSuccess(t, fixture.launchWith([]string{"CLAUDE_CONFIG_DIR=" + state}, "credential-deny", stateLink))
	})

	t.Run("custom_state_denies_defaults", func(t *testing.T) {
		state := filepath.Join(fixture.root, "custom-state")
		if err := os.Mkdir(state, 0o700); err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{".claude", ".config/claude"} {
			defaultPath := filepath.Join(fixture.worktree, path)
			if err := os.MkdirAll(defaultPath, 0o700); err != nil {
				t.Fatal(err)
			}
			result := fixture.launchWith([]string{"HOME=" + fixture.worktree, "CLAUDE_CONFIG_DIR=" + state}, "write-deny", defaultPath)
			requireSuccess(t, result)
		}
		defaultFile := filepath.Join(fixture.worktree, ".claude.json")
		if err := os.WriteFile(defaultFile, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		requireSuccess(t, fixture.launchWith([]string{"HOME=" + fixture.worktree, "CLAUDE_CONFIG_DIR=" + state}, "write-file-deny", defaultFile))
	})

	t.Run("policy_is_immutable", func(t *testing.T) {
		requireSuccess(t, fixture.launch("policy-immutable"))
	})

	t.Run("effective_policy_denies_unrelated_paths", func(t *testing.T) {
		requireSuccess(t, fixture.launch("effective-deny", os.Getenv("DEN_NATIVE_UNRELATED_STORE_FILE")))
		requireSuccess(t, fixture.launch("effective-deny", secret))
		testImplicitHostWrites(t, fixture)
	})

	t.Run("user_plugin_and_mcp", func(t *testing.T) {
		plugin := filepath.Join(fixture.worktree, "plugin")
		if err := os.Mkdir(plugin, 0o700); err != nil {
			t.Fatal(err)
		}
		probe := "#!/bin/sh\nset -eu\n! cat \"$DEN_NATIVE_SECRET\"\n"
		if err := os.WriteFile(filepath.Join(plugin, "probe.sh"), []byte(probe), 0o600); err != nil {
			t.Fatal(err)
		}
		mcp := filepath.Join(fixture.worktree, "mcp.sh")
		mcpProbe := fmt.Sprintf("#!/bin/sh\nset -eu\n! curl --fail --silent --connect-timeout 2 --cacert \"$REPOWOLF_CA_FILE\" --resolve github.com:%s:127.0.0.1 https://github.com:%s/\n", fixture.tlsPort(t), fixture.tlsPort(t))
		if err := os.WriteFile(mcp, []byte(mcpProbe), 0o600); err != nil {
			t.Fatal(err)
		}
		before := fixture.requestCount()
		requireSuccess(t, fixture.launchWith([]string{"DEN_NATIVE_SECRET=" + secret}, "plugin-mcp", "--plugin-dir", plugin, "--mcp-config", mcp, "--strict-mcp-config"))
		if fixture.requestCount() != before {
			t.Fatal("user MCP reached denied provider endpoint")
		}
	})
}

func TestRepoWolfRoutes(t *testing.T) {
	fixture := newNativeFixture(t)
	fixture.startDNS(t)
	result := fixture.launch("repowolf")
	if result.err != nil {
		operations, _ := os.ReadFile(fixture.operations)
		t.Fatalf("RepoWolf route failed: %v\nstdout:\n%s\nstderr:\n%s\noperations:\n%s", result.err, result.stdout, result.stderr, operations)
	}
	operations, err := os.ReadFile(fixture.operations)
	if err != nil {
		t.Fatal(err)
	}
	log := string(operations)
	for _, operation := range []string{"gh", "git-upload-pack"} {
		if !strings.Contains(log, operation+"\n") {
			t.Fatalf("RepoWolf fixture did not record %s: %q", operation, log)
		}
	}
	if fixture.requestCount() != 0 {
		t.Fatal("RepoWolf route contacted provider recorder")
	}
	if strings.Contains(log, fixtureToken) {
		t.Fatal("RepoWolf fixture log disclosed token")
	}
}

func TestConfigDirectoryRacesFailClosed(t *testing.T) {
	fixture := newNativeFixture(t)

	t.Run("replaceable_ancestor", func(t *testing.T) {
		ancestor := filepath.Join(fixture.root, "replaceable")
		state := filepath.Join(ancestor, "state")
		if err := os.MkdirAll(state, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(ancestor, 0o777); err != nil {
			t.Fatal(err)
		}
		marker := filepath.Join(fixture.worktree, "replaceable-started")
		result := fixture.launchWith([]string{"CLAUDE_CONFIG_DIR=" + state}, "marker", marker)
		if result.err == nil || fileExists(marker) {
			t.Fatalf("replaceable ancestor did not fail closed: %v / %s", result.err, result.stderr)
		}
	})

	t.Run("validation_time_path_swap", func(t *testing.T) {
		testValidationTimePathSwap(t, fixture)
	})
}

func validationTimePathSwap(t *testing.T, fixture *nativeFixture, probe, mutation []string) {
	t.Helper()
	if len(probe) == 0 || len(mutation) == 0 {
		t.Fatal("ACL probe and mutation commands are required")
	}
	state := filepath.Join(fixture.root, "swap-state")
	other := filepath.Join(fixture.root, "swap-other")
	for _, directory := range []string{state, other} {
		if err := os.Mkdir(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	counter := filepath.Join(fixture.root, "acl-probe-count")
	wrapper := filepath.Join(fixture.root, "acl-probe")
	marker := filepath.Join(fixture.worktree, "swap-started")
	mv, err := exec.LookPath("mv")
	if err != nil {
		t.Fatal(err)
	}
	quoted := func(command []string) string {
		arguments := make([]string, 0, len(command))
		for _, argument := range command {
			arguments = append(arguments, strconv.Quote(argument))
		}
		return strings.Join(arguments, " ")
	}
	script := fmt.Sprintf(`#!/bin/sh
set -eu
case "${DEN_CONFIGDIR_ACL_ORIGINAL_PATH-}" in
  %s)
    count=0
    if test -f %s; then read -r count < %s; fi
    count=$((count + 1))
    printf '%%s\n' "$count" > %s
    if test "$count" -eq 1; then
      status=0
      %s "$@" || status=$?
      %s %s
      exit "$status"
    fi
    if test "$count" -eq 2; then
      %s %s %s
      %s %s %s
      %s %s %s
      status=0
      %s "$@" || status=$?
      %s %s %s
      %s %s %s
      %s %s %s
      exit "$status"
    fi
    ;;
esac
exec %s "$@"
`, strconv.Quote(state), strconv.Quote(counter), strconv.Quote(counter), strconv.Quote(counter),
		quoted(probe), quoted(mutation), strconv.Quote(state),
		strconv.Quote(mv), strconv.Quote(state), strconv.Quote(state+".hold"),
		strconv.Quote(mv), strconv.Quote(other), strconv.Quote(state),
		strconv.Quote(mv), strconv.Quote(state+".hold"), strconv.Quote(other),
		quoted(probe),
		strconv.Quote(mv), strconv.Quote(state), strconv.Quote(state+".hold"),
		strconv.Quote(mv), strconv.Quote(other), strconv.Quote(state),
		strconv.Quote(mv), strconv.Quote(state+".hold"), strconv.Quote(other),
		quoted(probe))
	if err := os.WriteFile(wrapper, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	result := launchWithManifest(t, fixture, func(document map[string]any) {
		document["explicitConfigDir"] = state
		document["aclProbe"] = []any{wrapper}
	}, "marker", marker)
	if result.err == nil || fileExists(marker) || !strings.Contains(result.stderr, "custom configuration directory changed before launch") {
		t.Fatalf("validation-time path swap did not fail closed at revalidation: %v / %s", result.err, result.stderr)
	}
}

func TestReservedClaudeArgumentsFailBeforeFence(t *testing.T) {
	fixture := newNativeFixture(t)
	for _, argument := range []string{"--settings", "--settings=x", "--permission-mode", "--permission-mode=x", "--dangerously-skip-permissions", "--dangerously-skip-permissions=x"} {
		command := exec.Command(os.Getenv("DEN_NATIVE_CLAUDE"), argument)
		command.Dir = fixture.worktree
		command.Env = replaceEnvironment(os.Environ(),
			"HOME="+fixture.home, "REPOWOLF_ENDPOINT="+fixture.repoWolfEndpoint,
			"REPOWOLF_TOKEN="+fixtureToken, "REPOWOLF_CA_FILE="+fixture.certificate.ca,
		)
		output, err := command.CombinedOutput()
		if err == nil || !strings.Contains(string(output), "Den-owned security flag") {
			t.Fatalf("reserved argument %q was not rejected: %v\n%s", argument, err, output)
		}
	}
}

func launchWithManifest(t *testing.T, fixture *nativeFixture, mutate func(map[string]any), arguments ...string) commandResult {
	t.Helper()
	contents, err := os.ReadFile(os.Getenv("DEN_NATIVE_MANIFEST"))
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(contents, &document); err != nil {
		t.Fatal(err)
	}
	mutate(document)
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(fixture.root, fmt.Sprintf("manifest-%d.json", time.Now().UnixNano()))
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	commandArguments := []string{"--manifest", path, "--"}
	commandArguments = append(commandArguments, arguments...)
	command := exec.Command(os.Getenv("DEN_NATIVE_LAUNCHER"), commandArguments...)
	command.Dir = fixture.worktree
	command.Env = replaceEnvironment(os.Environ(),
		"HOME="+fixture.home, "REPOWOLF_ENDPOINT="+fixture.repoWolfEndpoint,
		"REPOWOLF_TOKEN="+fixtureToken, "REPOWOLF_CA_FILE="+fixture.certificate.ca,
	)
	var stdout, stderr strings.Builder
	command.Stdout, command.Stderr = &stdout, &stderr
	err = command.Run()
	return commandResult{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

func outsideFenceWritableFile(t *testing.T, parent string) string {
	t.Helper()
	_, statErr := os.Stat(parent)
	createdParent := os.IsNotExist(statErr)
	if statErr != nil && !createdParent {
		t.Fatal("outside-Fence control parent is unavailable")
	}
	if createdParent {
		if err := os.Mkdir(parent, 0o700); err != nil {
			t.Fatal("outside-Fence control parent is not writable")
		}
	}
	file, err := os.CreateTemp(parent, "den-native-control-*")
	if err != nil {
		t.Fatal("outside-Fence positive control is not writable")
	}
	path := file.Name()
	if _, err := file.Write([]byte("outside\n")); err != nil {
		_ = file.Close()
		t.Fatal("outside-Fence positive control write failed")
	}
	if err := file.Close(); err != nil {
		t.Fatal("outside-Fence positive control close failed")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal("outside-Fence positive control cleanup failed")
	}
	if createdParent {
		if err := os.Remove(parent); err != nil {
			t.Fatal("outside-Fence control parent cleanup failed")
		}
	}
	return path
}

func fileExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}
