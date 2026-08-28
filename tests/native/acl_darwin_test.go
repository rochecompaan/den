//go:build native && darwin

package native

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
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

// TEMPORARY Task 4j diagnostic
func TestDarwinACLProbeHandleDiagnostic(t *testing.T) {
	state := filepath.Join(t.TempDir(), "acl-state")
	if err := os.Mkdir(state, 0o700); err != nil {
		fmt.Println("DIAG-ACL: create state:", err)
		return
	}
	if output, err := exec.Command("/bin/chmod", "+a", "everyone allow read,readattr,readextattr,readsecurity", state).CombinedOutput(); err != nil {
		fmt.Println("DIAG-ACL: apply ACL:", err)
		printDiagnosticOutput("DIAG-ACL:", string(output))
		return
	}
	fd, err := syscall.Open(state, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		fmt.Println("DIAG-ACL: open directory handle:", err)
		return
	}
	file := os.NewFile(uintptr(fd), state)
	defer file.Close()

	command := exec.Command("/bin/sh", "-c", `
diag() {
  printf 'DIAG-ACL: $'
  printf ' %s' "$@"
  printf '\n'
  "$@" 2>&1 | sed 's/^/DIAG-ACL: /' || true
}
diag readlink /dev/fd/9
diag /bin/ls -lde /dev/fd/9
diag /bin/ls -lde /dev/fd/9/
diag /bin/ls -lde "$DIAG_STATE"
diag stat -f '%N type=%HT mode=%Sp owner=%Su:%Sg' /dev/fd/9
diag stat -f '%N type=%HT mode=%Sp owner=%Su:%Sg' /dev/fd/9/
diag stat -l -f '%N type=%HT mode=%Sp owner=%Su:%Sg' /dev/fd/9
`)
	command.ExtraFiles = []*os.File{file, file, file, file, file, file, file}
	command.Env = append(os.Environ(), "DIAG_STATE="+state)
	output, err := command.CombinedOutput()
	if err != nil {
		fmt.Println("DIAG-ACL: probe command:", err)
	}
	printRawDiagnosticOutput(string(output))
}

// TEMPORARY Task 4j diagnostic
func TestDarwinBashHookHangDiagnostic(t *testing.T) {
	fixture := newNativeFixture(t)
	fixture.installClaudeContextHook(t)
	replacement := filepath.Join(fixture.worktree, "replacement-policy")
	if err := os.WriteFile(replacement, []byte("replacement\n"), 0o600); err != nil {
		fmt.Println("DIAG-HOOK: write replacement policy:", err)
		return
	}
	marker := filepath.Join(fixture.worktree, "blocked-truncate")
	scenario := fixture.provider.register(
		`: > "$DEN_FENCE_POLICY_FILE" 2>/dev/null || true`,
		`gh repo create forbidden; printf escaped > "$DEN_NATIVE_BLOCKED_MARKER"`,
	)
	absoluteDeadline := time.Now().Add(60 * time.Second)
	diagnosticContext, cancel := context.WithDeadline(context.Background(), absoluteDeadline)
	defer cancel()
	command, results, err := startDarwinDiagnosticClaude(diagnosticContext, fixture, scenario, []string{
		"DEN_NATIVE_REPLACEMENT=" + replacement,
		"DEN_NATIVE_BLOCKED_MARKER=" + marker,
	})
	if err != nil {
		fmt.Println("DIAG-HOOK: launch failed:", err)
		return
	}

	var result *commandResult
	observation := time.NewTimer(45 * time.Second)
	defer observation.Stop()
	select {
	case returned := <-results:
		result = &returned
		fmt.Println("DIAG-HOOK: launch returned:", result.err)
		printDiagnosticOutput("DIAG-HOOK:", result.stdout)
		printDiagnosticOutput("DIAG-HOOK:", result.stderr)
	case <-observation.C:
		fmt.Println("DIAG-HOOK: launch still running after 45s")
		fixtureBash, err := resolveFixtureBash()
		if err != nil {
			fmt.Println("DIAG-HOOK: resolve fixture Bash:", err)
		}
		evidenceContext, cancelEvidence := context.WithTimeout(diagnosticContext, 5*time.Second)
		dumpDarwinHookProcesses(evidenceContext, fixtureBash)
		cancelEvidence()
	case <-diagnosticContext.Done():
		fmt.Println("DIAG-HOOK: overall diagnostic deadline reached before evidence:", diagnosticContext.Err())
	}
	confirmDarwinHookLaunchStopped(diagnosticContext, command.Process.Pid, results, result)
}

func resolveFixtureBash() (string, error) {
	claude, err := filepath.EvalSymlinks(os.Getenv("DEN_NATIVE_CLAUDE"))
	if err != nil {
		return "", err
	}
	contents, err := os.ReadFile(claude)
	if err != nil {
		return "", err
	}
	firstLine, _, _ := strings.Cut(string(contents), "\n")
	interpreter := strings.Fields(strings.TrimPrefix(firstLine, "#!"))
	if !strings.HasPrefix(firstLine, "#!") || len(interpreter) == 0 || !filepath.IsAbs(interpreter[0]) {
		return "", fmt.Errorf("fixture Claude executable has no absolute shebang: %q", firstLine)
	}
	return filepath.EvalSymlinks(interpreter[0])
}

func dumpDarwinHookProcesses(diagnosticContext context.Context, fixtureBash string) {
	psContext, cancel := context.WithTimeout(diagnosticContext, 2*time.Second)
	ps := exec.CommandContext(psContext, "ps", "-Ao", "pid,ppid,stat,etime,command")
	ps.WaitDelay = 2 * time.Second
	output, err := ps.CombinedOutput()
	cancel()
	fmt.Println("DIAG-HOOK: ps:", err)
	printDiagnosticOutput("DIAG-HOOK:", string(output))
	if diagnosticContext.Err() != nil {
		fmt.Println("DIAG-HOOK: overall diagnostic deadline reached before samples:", diagnosticContext.Err())
		return
	}
	if _, err := os.Stat("/usr/bin/sample"); err != nil {
		fmt.Println("DIAG-HOOK: /usr/bin/sample unavailable:", err)
		return
	}

	sampleContext, cancel := context.WithTimeout(diagnosticContext, 3*time.Second)
	defer cancel()
	var samples sync.WaitGroup
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		command := strings.Join(fields[4:], " ")
		commandFields := strings.Fields(command)
		matchesFixtureBash := len(commandFields) > 0 && commandFields[0] == fixtureBash
		if !strings.Contains(command, "fence") && !matchesFixtureBash {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		samples.Add(1)
		go func(pid int, command string) {
			defer samples.Done()
			sampleCommand := exec.CommandContext(sampleContext, "/usr/bin/sample", strconv.Itoa(pid), "2", "1")
			sampleCommand.WaitDelay = 2 * time.Second
			sample, err := sampleCommand.CombinedOutput()
			fmt.Println("DIAG-HOOK: sample pid=", pid, "command=", command, ":", err)
			printDiagnosticOutput("DIAG-HOOK:", string(sample))
		}(pid, command)
	}
	samplesDone := make(chan struct{})
	go func() {
		samples.Wait()
		close(samplesDone)
	}()
	select {
	case <-samplesDone:
	case <-diagnosticContext.Done():
		fmt.Println("DIAG-HOOK: overall diagnostic deadline reached while sampling:", diagnosticContext.Err())
	}
}

func startDarwinDiagnosticClaude(diagnosticContext context.Context, fixture *nativeFixture, scenario *claudeScenario, extraEnvironment []string) (*exec.Cmd, <-chan commandResult, error) {
	arguments := []string{"-p", scenario.prompt(), "--output-format", "text"}
	command := exec.CommandContext(diagnosticContext, os.Getenv("DEN_NATIVE_CLAUDE"), arguments...)
	command.Dir = fixture.worktree
	providerEnvironment := []string{
		"ANTHROPIC_API_KEY=den-native-local-test-key",
		"ANTHROPIC_BASE_URL=" + fixtureURL(tlsRecorderPort, brokerHostname()),
		"NODE_EXTRA_CA_CERTS=" + fixture.certificate.ca,
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1",
		"DEN_NATIVE_CONTEXT_CURL=/usr/bin/curl",
	}
	command.Env = fixture.launchEnvironment(append(providerEnvironment, extraEnvironment...))
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.WaitDelay = 2 * time.Second
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Start(); err != nil {
		return nil, nil, err
	}
	results := make(chan commandResult, 1)
	go func() {
		err := command.Wait()
		results <- commandResult{stdout: stdout.String(), stderr: stderr.String(), err: err}
	}()
	return command, results, nil
}

func confirmDarwinHookLaunchStopped(diagnosticContext context.Context, processGroup int, results <-chan commandResult, result *commandResult) {
	if result != nil && !darwinProcessGroupAlive(processGroup) {
		fmt.Println("DIAG-HOOK: launch result drained and process tree confirmed dead")
		return
	}
	if diagnosticContext.Err() != nil {
		printDarwinHookDeadlineEvidence(processGroup, result, diagnosticContext.Err())
		return
	}

	fmt.Println("DIAG-HOOK: terminating launched process group:", processGroup)
	fmt.Println("DIAG-HOOK: process-group TERM:", syscall.Kill(-processGroup, syscall.SIGTERM))
	for attempts := 0; attempts < 40; attempts++ {
		if result != nil && !darwinProcessGroupAlive(processGroup) {
			fmt.Println("DIAG-HOOK: launch result drained and process tree confirmed dead")
			return
		}
		select {
		case returned := <-results:
			result = &returned
			fmt.Println("DIAG-HOOK: launch returned after TERM:", result.err)
			printDiagnosticOutput("DIAG-HOOK:", result.stdout)
			printDiagnosticOutput("DIAG-HOOK:", result.stderr)
		case <-diagnosticContext.Done():
			printDarwinHookDeadlineEvidence(processGroup, result, diagnosticContext.Err())
			return
		case <-time.After(50 * time.Millisecond):
		}
	}
	drainDarwinHookResult(diagnosticContext, processGroup, results, result)
}

func drainDarwinHookResult(diagnosticContext context.Context, processGroup int, results <-chan commandResult, result *commandResult) {
	fmt.Println("DIAG-HOOK: process-group KILL:", syscall.Kill(-processGroup, syscall.SIGKILL))
	for {
		if result != nil && !darwinProcessGroupAlive(processGroup) {
			fmt.Println("DIAG-HOOK: launch result drained and process tree confirmed dead")
			return
		}
		select {
		case returned := <-results:
			result = &returned
			fmt.Println("DIAG-HOOK: launch returned after KILL:", result.err)
			printDiagnosticOutput("DIAG-HOOK:", result.stdout)
			printDiagnosticOutput("DIAG-HOOK:", result.stderr)
		case <-diagnosticContext.Done():
			printDarwinHookDeadlineEvidence(processGroup, result, diagnosticContext.Err())
			return
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func printDarwinHookDeadlineEvidence(processGroup int, result *commandResult, err error) {
	fmt.Println("DIAG-HOOK: overall diagnostic deadline reached:", err)
	fmt.Println("DIAG-HOOK: launch result drained:", result != nil)
	fmt.Println("DIAG-HOOK: process group alive:", darwinProcessGroupAlive(processGroup))
}

func darwinProcessGroupAlive(processGroup int) bool {
	return syscall.Kill(-processGroup, 0) == nil
}

func printDiagnosticOutput(prefix, output string) {
	for _, line := range strings.Split(strings.TrimSuffix(output, "\n"), "\n") {
		fmt.Println(prefix, line)
	}
}

func printRawDiagnosticOutput(output string) {
	for _, line := range strings.Split(strings.TrimSuffix(output, "\n"), "\n") {
		fmt.Println(line)
	}
}

func TestDarwinBashHook(t *testing.T) {
	fixture := newNativeFixture(t)
	fixture.installClaudeContextHook(t)
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
	allowed := nestedFenceWitnessCommand() + fmt.Sprintf(`
printf allowed > %s
printf 'scratch:%%s\n' "$DEN_FENCE_TMPDIR"`, strconv.Quote(allowedMarker))
	blocked := fmt.Sprintf(`gh repo create forbidden; printf escaped > %s`, strconv.Quote(blockedMarker))
	scenario := fixture.provider.register(allowed, blocked)
	requireSuccess(t, fixture.launchClaude(scenario, nil))
	results := fixture.provider.scenarioResults(scenario)
	if !fileExists(allowedMarker) || fileExists(blockedMarker) || len(results) != 2 ||
		scratchPath(results[0]) == "" || !strings.Contains(results[0], "nested-proxy:http://") {
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
	fixture.installClaudeContextHook(t)
	probePaths := outsideFencePositiveControls(t)
	const launches = 2
	scenarios := make([]*claudeScenario, launches)
	results := make([]commandResult, launches)
	for index := range scenarios {
		first := nestedFenceWitnessCommand() + `
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
		if len(toolResults) != 2 || !strings.Contains(toolResults[0], "nested-proxy:http://") ||
			!strings.Contains(toolResults[1], "nested-proxy:http://") || !strings.Contains(toolResults[1], "complete") {
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
		"DEN_NATIVE_CONTEXT_CURL=/usr/bin/curl",
	}
	providerEnvironment = append(providerEnvironment, extraEnvironment...)
	command.Env = fixture.launchEnvironment(providerEnvironment)
	return runCommand(command)
}
