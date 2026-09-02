//go:build native && darwin

package native

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
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
	probe := os.Getenv("DEN_NATIVE_ACL_PROBE")
	if probe == "" {
		t.Fatal("DEN_NATIVE_ACL_PROBE is required")
	}
	validationTimePathSwap(t, fixture,
		[]string{probe},
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

func TestDarwinACLProbeMatchesLs(t *testing.T) {
	probe := os.Getenv("DEN_NATIVE_ACL_PROBE")
	if probe == "" {
		t.Skip("DEN_NATIVE_ACL_PROBE is unset")
	}
	state := filepath.Join(t.TempDir(), "acl-state")
	if err := os.Mkdir(state, 0o700); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("/bin/chmod", "+a", "everyone allow read,readattr,readextattr,readsecurity", state).CombinedOutput(); err != nil {
		t.Fatalf("apply real macOS ACL: %v\n%s", err, output)
	}

	helperOutput, err := runDarwinACLProbe(probe, state)
	if err != nil {
		t.Fatalf("fd-native ACL probe: %v\n%s", err, helperOutput)
	}
	lsOutput, err := exec.Command("/bin/ls", "-lde", state).CombinedOutput()
	if err != nil {
		t.Fatalf("inspect ACL with ls: %v\n%s", err, lsOutput)
	}
	helperEntries := darwinAllowEntries(helperOutput)
	lsEntries := darwinAllowEntries(string(lsOutput))
	if len(helperEntries) != 1 || len(lsEntries) != 1 {
		t.Fatalf("expected one allow entry from helper and ls; helper=%q ls=%q", helperOutput, lsOutput)
	}
	if !strings.HasPrefix(helperEntries[0].principal, "group:") {
		t.Fatalf("helper principal is not a group: %q", helperEntries[0].principal)
	}
	if !sameDarwinPermissions(helperEntries[0].permissions, lsEntries[0].permissions) {
		t.Fatalf("helper permissions differ from ls; helper=%q ls=%q", helperOutput, lsOutput)
	}
	expectedPermissions := map[string]struct{}{
		"list": {}, "readattr": {}, "readextattr": {}, "readsecurity": {},
	}
	if !sameDarwinPermissions(helperEntries[0].permissions, expectedPermissions) {
		t.Fatalf("helper permissions do not match chmod grant: %q", helperOutput)
	}

	noACL := t.TempDir()
	output, err := runDarwinACLProbe(probe, noACL)
	if err != nil {
		t.Fatalf("no-ACL probe failed: %v\n%s", err, output)
	}
	if output != "" {
		t.Fatalf("no-ACL probe returned output: %q", output)
	}
}

type darwinACLEntry struct {
	principal   string
	permissions map[string]struct{}
}

func runDarwinACLProbe(probe, path string) (string, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return "", err
	}
	file := os.NewFile(uintptr(fd), path)
	defer file.Close()
	command := exec.Command(probe, "/dev/fd/9")
	command.ExtraFiles = []*os.File{file, file, file, file, file, file, file}
	output, err := command.CombinedOutput()
	return string(output), err
}

func darwinAllowEntries(output string) []darwinACLEntry {
	var entries []darwinACLEntry
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		for index, field := range fields {
			if field != "allow" || index == 0 || index+1 >= len(fields) {
				continue
			}
			permissions := make(map[string]struct{})
			for _, permission := range strings.Split(strings.Join(fields[index+1:], ""), ",") {
				if permission != "" {
					permissions[permission] = struct{}{}
				}
			}
			entries = append(entries, darwinACLEntry{principal: fields[index-1], permissions: permissions})
			break
		}
	}
	return entries
}

func sameDarwinPermissions(left, right map[string]struct{}) bool {
	if len(left) != len(right) {
		return false
	}
	for permission := range left {
		if _, ok := right[permission]; !ok {
			return false
		}
	}
	return true
}

// TEMPORARY Task 4j diagnostic
func TestDarwinBashHookHangDiagnostic(t *testing.T) {
	fixture := newNativeFixture(t)
	fixture.startDNS(t)
	fixture.installClaudeContextHook(t)
	replacement := filepath.Join(fixture.worktree, "replacement-policy")
	if err := os.WriteFile(replacement, []byte("replacement\n"), 0o600); err != nil {
		fmt.Println("DIAG-HOOK: write replacement policy:", err)
		return
	}
	marker := filepath.Join(fixture.worktree, "blocked-truncate")
	hookEvidence := filepath.Join(fixture.worktree, "hook-evidence")
	scenario := fixture.provider.register(
		`{
  printf 'before stat: '
  stat -f 'size=%z mode=%Mp%Lp perms=%Sp' "$DEN_FENCE_POLICY_FILE"
  printf '\n'
  printf 'before sha256: '
  shasum -a 256 "$DEN_FENCE_POLICY_FILE"
  if : > "$DEN_FENCE_POLICY_FILE"; then
    printf 'truncate: succeeded\n'
  else
    status=$?
    printf 'truncate: failed status=%s\n' "$status"
  fi
  printf 'after stat: '
  stat -f 'size=%z mode=%Mp%Lp perms=%Sp' "$DEN_FENCE_POLICY_FILE"
  printf '\n'
  printf 'after sha256: '
  shasum -a 256 "$DEN_FENCE_POLICY_FILE"
} >> "$DEN_NATIVE_HOOK_EVIDENCE" 2>&1 || true
true`,
		`gh repo create forbidden; printf escaped > "$DEN_NATIVE_BLOCKED_MARKER"`,
	)
	absoluteDeadline := time.Now().Add(60 * time.Second)
	diagnosticContext, cancel := context.WithDeadline(context.Background(), absoluteDeadline)
	defer cancel()
	command, results, stdout, stderr, err := startDarwinDiagnosticClaude(diagnosticContext, fixture, scenario, []string{
		"DEN_NATIVE_REPLACEMENT=" + replacement,
		"DEN_NATIVE_BLOCKED_MARKER=" + marker,
		"DEN_NATIVE_HOOK_EVIDENCE=" + hookEvidence,
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
		processEvidence := dumpDarwinHookProcesses(evidenceContext, fixtureBash, command.Process.Pid)
		printDarwinHookObservation(evidenceContext, fixture, scenario, processEvidence, hookEvidence, stdout, stderr)
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

type darwinHookProcessEvidence struct {
	output      string
	environment darwinHookEnvironmentSnapshot
}

type darwinHookProcess struct {
	pid, ppid, pgid int
	command         string
}

func dumpDarwinHookProcesses(diagnosticContext context.Context, fixtureBash string, processGroup int) darwinHookProcessEvidence {
	psContext, cancel := context.WithTimeout(diagnosticContext, 2*time.Second)
	ps := exec.CommandContext(psContext, "ps", "-Ao", "pid,ppid,pgid,stat,etime,command")
	ps.WaitDelay = 2 * time.Second
	output, err := ps.CombinedOutput()
	cancel()
	fmt.Println("DIAG-HOOK: ps:", err)
	printDiagnosticOutput("DIAG-HOOK:", string(output))
	evidence := darwinHookProcessEvidence{output: string(output), environment: darwinHookEnvironmentSnapshot{status: "unavailable", reason: darwinHookReasonPIDUnavailable}}
	if snapshot, failed := darwinHookPreselectionFailure(diagnosticContext); failed {
		evidence.environment = snapshot
		fmt.Println("DIAG-HOOK: overall diagnostic deadline reached before samples:", diagnosticContext.Err())
		return evidence
	}

	claudePID, claudeCommand, selection := darwinHookClaudeProcess(diagnosticContext, string(output), processGroup)
	var environmentDone <-chan darwinHookEnvironmentSnapshot
	if selection.status != "available" {
		evidence.environment = selection
	} else {
		captureContext, cancelCapture := context.WithTimeout(diagnosticContext, 3*time.Second)
		done := make(chan darwinHookEnvironmentSnapshot, 1)
		environmentDone = done
		go func() {
			defer cancelCapture()
			done <- captureDarwinHookEnvironment(captureContext, claudePID, claudeCommand)
		}()
	}

	if _, err := os.Stat("/usr/bin/sample"); err != nil {
		fmt.Println("DIAG-HOOK: /usr/bin/sample unavailable:", err)
		return waitForDarwinHookEnvironment(diagnosticContext, evidence, environmentDone)
	}

	sampleContext, cancel := context.WithTimeout(diagnosticContext, 3*time.Second)
	defer cancel()
	var samples sync.WaitGroup
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		command := strings.Join(fields[5:], " ")
		commandFields := strings.Fields(command)
		matchesFixtureBash := len(commandFields) > 0 && commandFields[0] == fixtureBash
		pgid, parsePGIDError := strconv.Atoi(fields[2])
		matchesResolver := strings.Contains(command, "resolver") || strings.Contains(command, "sudo")
		matches := matchesResolver || (parsePGIDError == nil && pgid == processGroup)
		if parsePGIDError != nil {
			matches = matchesResolver || strings.Contains(command, "fence") || matchesFixtureBash
		}
		if !matches {
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
	return waitForDarwinHookEnvironment(diagnosticContext, evidence, environmentDone)
}

func waitForDarwinHookEnvironment(diagnosticContext context.Context, evidence darwinHookProcessEvidence, done <-chan darwinHookEnvironmentSnapshot) darwinHookProcessEvidence {
	if done == nil {
		return evidence
	}
	select {
	case evidence.environment = <-done:
	case <-diagnosticContext.Done():
		evidence.environment = darwinHookEnvironmentSnapshot{status: "unavailable", reason: darwinHookReasonCaptureFailed}
	}
	return evidence
}

type darwinHookBinding struct {
	FenceExecutable string `json:"fenceExecutable"`
	Agent           struct {
		Executable string `json:"executable"`
	} `json:"agent"`
}

func darwinHookPreselectionFailure(diagnosticContext context.Context) (darwinHookEnvironmentSnapshot, bool) {
	if diagnosticContext.Err() == nil {
		return darwinHookEnvironmentSnapshot{}, false
	}
	return darwinHookEnvironmentSnapshot{status: "unavailable", reason: darwinHookReasonPIDUnavailable}, true
}

func darwinHookManifestFailure(truncated bool, err error) (darwinHookEnvironmentSnapshot, bool) {
	if err == nil && !truncated {
		return darwinHookEnvironmentSnapshot{}, false
	}
	return darwinHookEnvironmentSnapshot{status: "unavailable", reason: darwinHookReasonIdentityMismatch}, true
}

func darwinHookClaudeProcess(diagnosticContext context.Context, output string, processGroup int) (string, string, darwinHookEnvironmentSnapshot) {
	manifest, truncated, err := darwinHookManifestContents(diagnosticContext)
	if snapshot, failed := darwinHookManifestFailure(truncated, err); failed {
		return "", "", snapshot
	}
	return darwinHookClaudePIDFromBinding(diagnosticContext, output, processGroup, manifest, os.Getenv("DEN_NATIVE_FENCE"), readDarwinHookRegularFile, filepath.EvalSymlinks)
}

func darwinHookClaudePIDFromBinding(diagnosticContext context.Context, output string, processGroup int, manifest []byte, expectedFencePath string, read func(context.Context, string) ([]byte, bool, error), resolve func(string) (string, error)) (string, string, darwinHookEnvironmentSnapshot) {
	expectedFence, expectedWrapper, status := darwinHookBindingFromContents(manifest, expectedFencePath, resolve)
	if status != "available" {
		return "", "", darwinHookEnvironmentSnapshot{status: status, reason: darwinHookReasonIdentityMismatch}
	}
	processes := darwinHookProcesses(output, processGroup)
	var fences []darwinHookProcess
	for _, process := range processes {
		if darwinHookFenceProcess(process.command, expectedFence) {
			fences = append(fences, process)
		}
	}
	if len(fences) == 0 {
		return "", "", darwinHookEnvironmentSnapshot{status: "unavailable", reason: darwinHookReasonPIDUnavailable}
	}
	if len(fences) != 1 {
		return "", "", darwinHookEnvironmentSnapshot{status: "conflicting", reason: darwinHookReasonPIDUnavailable}
	}
	wrapper, status := darwinHookFenceWrapper(fences[0].command)
	if status != "available" {
		return "", "", darwinHookEnvironmentSnapshot{status: status, reason: darwinHookReasonIdentityMismatch}
	}
	if wrapper != expectedWrapper {
		return "", "", darwinHookEnvironmentSnapshot{status: "unavailable", reason: darwinHookReasonIdentityMismatch}
	}
	expectedClaude, status := darwinHookClaudePathFromWrapper(diagnosticContext, expectedWrapper, read, resolve)
	if status != "available" {
		return "", "", darwinHookEnvironmentSnapshot{status: status, reason: darwinHookReasonIdentityMismatch}
	}
	return darwinHookClaudePID(processes, expectedFence, expectedClaude)
}

func darwinHookManifestContents(diagnosticContext context.Context) ([]byte, bool, error) {
	manifestPath, ok := darwinHookCanonicalNixStorePath(os.Getenv("DEN_NATIVE_MANIFEST"), filepath.EvalSymlinks)
	if !ok {
		return nil, false, fmt.Errorf("invalid manifest path")
	}
	return readDarwinHookRegularFile(diagnosticContext, manifestPath)
}

func darwinHookBindingFromContents(contents []byte, expectedFencePath string, resolve func(string) (string, error)) (string, string, string) {
	var binding darwinHookBinding
	if err := json.Unmarshal(contents, &binding); err != nil {
		return "", "", "unavailable"
	}
	fence, ok := darwinHookCanonicalNixStorePath(binding.FenceExecutable, resolve)
	if !ok || !darwinHookNixStoreExecutable(fence, "fence") {
		return "", "", "unavailable"
	}
	expectedFence, ok := darwinHookCanonicalNixStorePath(expectedFencePath, resolve)
	if !ok || expectedFence != fence {
		return "", "", "unavailable"
	}
	wrapper, ok := darwinHookCanonicalNixStorePath(binding.Agent.Executable, resolve)
	if !ok {
		return "", "", "unavailable"
	}
	return fence, wrapper, "available"
}

func darwinHookFenceWrapper(command string) (string, string) {
	fields := strings.Fields(command)
	separator := -1
	for index, field := range fields {
		if field != "--" {
			continue
		}
		if separator != -1 || index+1 >= len(fields) {
			return "", "conflicting"
		}
		separator = index
	}
	if separator == -1 {
		return "", "unavailable"
	}
	return fields[separator+1], "available"
}

func darwinHookProcesses(output string, processGroup int) []darwinHookProcess {
	var processes []darwinHookProcess
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		pid, pidErr := strconv.Atoi(fields[0])
		ppid, ppidErr := strconv.Atoi(fields[1])
		pgid, pgidErr := strconv.Atoi(fields[2])
		if pidErr != nil || ppidErr != nil || pgidErr != nil || pgid != processGroup {
			continue
		}
		processes = append(processes, darwinHookProcess{pid: pid, ppid: ppid, pgid: pgid, command: strings.Join(fields[5:], " ")})
	}
	return processes
}

func darwinHookClaudePathFromWrapper(diagnosticContext context.Context, wrapper string, read func(context.Context, string) ([]byte, bool, error), resolve func(string) (string, error)) (string, string) {
	contents, truncated, err := read(diagnosticContext, wrapper)
	if err != nil || truncated {
		return "", "unavailable"
	}
	var realClaude string
	foundCAExport := false
	for _, line := range strings.Split(string(contents), "\n") {
		line = strings.TrimSpace(line)
		if line == `export NODE_EXTRA_CA_CERTS="$REPOWOLF_CA_FILE"` {
			foundCAExport = true
			continue
		}
		parts := strings.Fields(line)
		if len(parts) == 0 || parts[0] != "exec" {
			continue
		}
		if len(parts) != 3 || parts[2] != `"$@"` || realClaude != "" {
			return "", "conflicting"
		}
		candidate, ok := darwinHookCanonicalNixStorePath(parts[1], resolve)
		if !ok || !darwinHookNixStoreExecutable(candidate, "claude") {
			return "", "unavailable"
		}
		realClaude = candidate
	}
	if !foundCAExport || realClaude == "" {
		return "", "unavailable"
	}
	return realClaude, "available"
}

func darwinHookClaudePID(processes []darwinHookProcess, expectedFence, expectedClaude string) (string, string, darwinHookEnvironmentSnapshot) {
	if !darwinHookNixStoreExecutable(expectedFence, "fence") || !darwinHookNixStoreExecutable(expectedClaude, "claude") {
		return "", "", darwinHookEnvironmentSnapshot{status: "unavailable", reason: darwinHookReasonIdentityMismatch}
	}
	var fences []darwinHookProcess
	for _, process := range processes {
		if darwinHookFenceProcess(process.command, expectedFence) {
			fences = append(fences, process)
		}
	}
	if len(fences) == 0 {
		return "", "", darwinHookEnvironmentSnapshot{status: "unavailable", reason: darwinHookReasonPIDUnavailable}
	}
	if len(fences) != 1 {
		return "", "", darwinHookEnvironmentSnapshot{status: "conflicting", reason: darwinHookReasonPIDUnavailable}
	}
	var claude []darwinHookProcess
	for _, process := range processes {
		fields := strings.Fields(process.command)
		if process.ppid == fences[0].pid && len(fields) > 0 && fields[0] == expectedClaude {
			claude = append(claude, process)
		}
	}
	if len(claude) == 0 {
		return "", "", darwinHookEnvironmentSnapshot{status: "unavailable", reason: darwinHookReasonPIDUnavailable}
	}
	if len(claude) != 1 {
		return "", "", darwinHookEnvironmentSnapshot{status: "conflicting", reason: darwinHookReasonPIDUnavailable}
	}
	return strconv.Itoa(claude[0].pid), claude[0].command, darwinHookEnvironmentSnapshot{status: "available"}
}

func darwinHookFenceProcess(command, expectedFence string) bool {
	fields := strings.Fields(command)
	if len(fields) == 0 || fields[0] != expectedFence {
		return false
	}
	for index, field := range fields[:len(fields)-1] {
		if field == "--settings" && fields[index+1] != "" {
			return true
		}
	}
	return false
}

func darwinHookCanonicalNixStorePath(path string, resolve func(string) (string, error)) (string, bool) {
	if !darwinHookNixStorePath(path) {
		return "", false
	}
	resolved, err := resolve(path)
	if err != nil || resolved != path || !darwinHookNixStorePath(resolved) {
		return "", false
	}
	return resolved, true
}

func darwinHookNixStorePath(path string) bool {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || !strings.HasPrefix(path, "/nix/store/") {
		return false
	}
	relative, err := filepath.Rel("/nix/store", path)
	return err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func darwinHookNixStoreExecutable(path, name string) bool {
	return darwinHookNixStorePath(path) && strings.HasSuffix(path, "/bin/"+name)
}

func captureDarwinHookEnvironment(diagnosticContext context.Context, pid, command string) darwinHookEnvironmentSnapshot {
	output := &lockedBuffer{limit: darwinHookObservationLimit}
	ps := exec.CommandContext(diagnosticContext, "/bin/ps", "eww", "-p", pid, "-o", "command=")
	ps.WaitDelay = 2 * time.Second
	ps.Stdout, ps.Stderr = output, output
	err := ps.Run()
	contents, truncated, available := output.Snapshot(diagnosticContext, darwinHookObservationLimit)
	return classifyDarwinHookEnvironmentCapture(command, contents, truncated, available, err)
}

func classifyDarwinHookEnvironmentCapture(command, contents string, truncated, available bool, err error) darwinHookEnvironmentSnapshot {
	if !available || err != nil {
		return darwinHookEnvironmentSnapshot{status: "unavailable", reason: darwinHookReasonCaptureFailed}
	}
	if truncated {
		return darwinHookEnvironmentSnapshot{status: "truncated", reason: darwinHookReasonCaptureFailed}
	}
	return parseDarwinHookEnvironment(command, contents)
}

const darwinHookObservationLimit = 64 * 1024

func printDarwinHookObservation(diagnosticContext context.Context, fixture *nativeFixture, scenario *claudeScenario, processEvidence darwinHookProcessEvidence, hookEvidence string, stdout, stderr *lockedBuffer) {
	psOutput := processEvidence.output
	if !darwinHookEvidenceActive(diagnosticContext) {
		return
	}
	requests := fixture.requestCount()
	if !darwinHookEvidenceActive(diagnosticContext) {
		return
	}
	providerMessageRequests := fixture.provider.messageRequestCount()
	if !darwinHookEvidenceActive(diagnosticContext) {
		return
	}
	dnsNames := fixture.dns.names()
	if !darwinHookEvidenceActive(diagnosticContext) {
		return
	}
	fmt.Println("DIAG-HOOK: egress fixture requests:", requests)
	fmt.Println("DIAG-HOOK: provider /v1/messages arrivals:", providerMessageRequests)
	fmt.Println("DIAG-HOOK: egress fixture DNS names:", len(dnsNames))
	for index, name := range dnsNames {
		if !darwinHookEvidenceActive(diagnosticContext) {
			return
		}
		fmt.Println("DIAG-HOOK: egress fixture DNS name", index, ":", strconv.Quote(name))
	}
	if !darwinHookEvidenceActive(diagnosticContext) {
		return
	}
	policyPath := printDarwinHookPolicyEvidence(diagnosticContext, psOutput)
	if !darwinHookEvidenceActive(diagnosticContext) {
		return
	}
	policyContent := printDarwinHookPolicyContent(diagnosticContext, policyPath)
	if !darwinHookEvidenceActive(diagnosticContext) {
		return
	}
	lsof := printDarwinHookLsof(diagnosticContext, psOutput)
	if !darwinHookEvidenceActive(diagnosticContext) {
		return
	}
	printDarwinHookEnvironment(diagnosticContext, processEvidence.environment, lsof)
	if !darwinHookEvidenceActive(diagnosticContext) {
		return
	}
	fmt.Println("DIAG-HOOK: egress summary requests=" + strconv.Itoa(requests) + " providerMessageRequests=" + strconv.Itoa(providerMessageRequests) + " dnsNames=" + strconv.Itoa(len(dnsNames)) + " policyContent=" + policyContent + " lsof=" + lsof.status)
	if !darwinHookEvidenceActive(diagnosticContext) {
		return
	}
	printDarwinHookContextEvidence(diagnosticContext)
	if !darwinHookEvidenceActive(diagnosticContext) {
		return
	}
	printDarwinHookFile(diagnosticContext, "mutation evidence", hookEvidence)
	if !darwinHookEvidenceActive(diagnosticContext) {
		return
	}
	results := fixture.provider.scenarioResults(scenario)
	fmt.Println("DIAG-HOOK: provider scenario result count:", len(results))
	for index, result := range results {
		if !printDarwinHookOutput(diagnosticContext, fmt.Sprintf("DIAG-HOOK: provider scenario result %d:", index), result) {
			return
		}
	}
	printDarwinHookStream(diagnosticContext, "launch stdout at observation", stdout)
	printDarwinHookStream(diagnosticContext, "launch stderr at observation", stderr)
}

func darwinHookEvidenceActive(diagnosticContext context.Context) bool {
	if err := diagnosticContext.Err(); err != nil {
		fmt.Println("DIAG-HOOK: observation evidence deadline reached:", err)
		return false
	}
	return true
}

func printDarwinHookPolicyEvidence(diagnosticContext context.Context, psOutput string) string {
	policyPath, source := policyPathFromDarwinHookPS(diagnosticContext, psOutput)
	if policyPath == "" && darwinHookEvidenceActive(diagnosticContext) {
		var err error
		policyPath, err = newestDarwinHookPath(diagnosticContext, filepath.Join("/private/tmp", fmt.Sprintf("den-%d", os.Getuid()), "policy-*", "policy.json"))
		if err != nil {
			fmt.Println("DIAG-HOOK: policy fallback:", err)
		}
		source = "newest policy glob"
	}
	if policyPath == "" {
		fmt.Println("DIAG-HOOK: policy after-state unavailable")
		return ""
	}
	info, err := os.Stat(policyPath)
	if err != nil {
		fmt.Println("DIAG-HOOK: policy after-state source=", source, "path=", policyPath, "stat:", err)
		return policyPath
	}
	if !info.Mode().IsRegular() {
		fmt.Println("DIAG-HOOK: policy after-state source=", source, "path=", policyPath, "rejected non-regular mode=", info.Mode())
		return policyPath
	}
	contents, truncated, err := readDarwinHookRegularFile(diagnosticContext, policyPath)
	if err != nil {
		fmt.Println("DIAG-HOOK: policy after-state source=", source, "path=", policyPath, "read:", err)
		return policyPath
	}
	if truncated {
		fmt.Println("DIAG-HOOK: policy after-state source=", source, "path=", policyPath, "hash skipped: exceeds", darwinHookObservationLimit, "bytes")
		return policyPath
	}
	hash := sha256.Sum256(contents)
	fmt.Println(fmt.Sprintf("DIAG-HOOK: policy after-state source=%s path=%s size=%d mode=%#o sha256=%x", source, policyPath, info.Size(), info.Mode().Perm(), hash))
	return policyPath
}

func printDarwinHookPolicyContent(diagnosticContext context.Context, policyPath string) string {
	if policyPath == "" {
		fmt.Println("DIAG-HOOK: policy content unavailable")
		return "error"
	}
	contents, truncated, err := readDarwinHookRegularFile(diagnosticContext, policyPath)
	if err != nil {
		fmt.Println("DIAG-HOOK: policy content path=", policyPath, "read:", err)
		return "error"
	}
	if truncated {
		fmt.Println("DIAG-HOOK: policy content path=", policyPath, "truncated at", darwinHookObservationLimit, "bytes")
		return "error"
	}
	if !printDarwinHookOutput(diagnosticContext, "DIAG-HOOK: policy content:", string(contents)) {
		return "error"
	}
	return "printed"
}

type darwinHookLsofEvidence struct {
	status string
	output string
}

func printDarwinHookLsof(diagnosticContext context.Context, psOutput string) darwinHookLsofEvidence {
	pids := darwinHookProcessGroupPIDs(diagnosticContext, psOutput)
	if !darwinHookEvidenceActive(diagnosticContext) {
		return darwinHookLsofEvidence{status: "error"}
	}
	if len(pids) == 0 {
		fmt.Println("DIAG-HOOK: lsof unavailable (no launched process-group pids)")
		return darwinHookLsofEvidence{status: "error"}
	}
	lsofContext, cancel := context.WithTimeout(diagnosticContext, 2*time.Second)
	output := &lockedBuffer{limit: darwinHookObservationLimit}
	lsof := exec.CommandContext(lsofContext, "/usr/sbin/lsof", "-nP", "-a", "-i", "-p", strings.Join(pids, ","))
	lsof.WaitDelay = 2 * time.Second
	lsof.Stdout, lsof.Stderr = output, output
	err := lsof.Run()
	cancel()
	fmt.Println("DIAG-HOOK: lsof:", err)
	contents, truncated, available := output.Snapshot(diagnosticContext, darwinHookObservationLimit)
	if !available {
		fmt.Println("DIAG-HOOK: lsof output unavailable before observation deadline")
		return darwinHookLsofEvidence{status: "error"}
	}
	if truncated {
		fmt.Println("DIAG-HOOK: lsof output truncated at", darwinHookObservationLimit, "bytes")
	}
	if !printDarwinHookOutput(diagnosticContext, "DIAG-HOOK:", contents) {
		return darwinHookLsofEvidence{status: "error"}
	}
	if err != nil || truncated {
		return darwinHookLsofEvidence{status: "error"}
	}
	return darwinHookLsofEvidence{status: "ok", output: contents}
}

type darwinHookEnvironmentReason uint8

const (
	darwinHookReasonNone darwinHookEnvironmentReason = iota
	darwinHookReasonIdentityMismatch
	darwinHookReasonPIDUnavailable
	darwinHookReasonCaptureFailed
	darwinHookReasonParseAmbiguous
)

func (reason darwinHookEnvironmentReason) fixedCode() string {
	switch reason {
	case darwinHookReasonIdentityMismatch:
		return "identity-mismatch"
	case darwinHookReasonPIDUnavailable:
		return "pid-unavailable"
	case darwinHookReasonCaptureFailed:
		return "capture-failed"
	case darwinHookReasonParseAmbiguous:
		return "parse-ambiguous"
	default:
		return "capture-failed"
	}
}

type darwinHookEnvironmentSnapshot struct {
	status string
	reason darwinHookEnvironmentReason
	values map[string]string
}

var darwinHookEnvironmentNames = []string{
	"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY",
	"http_proxy", "https_proxy", "all_proxy",
	"NO_PROXY", "no_proxy",
	"LC_ALL", "LC_CTYPE", "LANG",
	"NODE_EXTRA_CA_CERTS", "REPOWOLF_CA_FILE", "FENCE_SANDBOX",
}

func parseDarwinHookEnvironment(command, output string) darwinHookEnvironmentSnapshot {
	line, remainder, hasMoreLines := strings.Cut(strings.TrimSuffix(output, "\n"), "\n")
	if hasMoreLines || !strings.HasPrefix(line, command) {
		return darwinHookEnvironmentSnapshot{status: "conflicting", reason: darwinHookReasonParseAmbiguous}
	}
	remainder = strings.TrimPrefix(line, command)
	if remainder != "" && (remainder[0] != ' ' && remainder[0] != '\t') {
		return darwinHookEnvironmentSnapshot{status: "conflicting", reason: darwinHookReasonParseAmbiguous}
	}
	if strings.TrimSpace(remainder) == "" {
		return darwinHookEnvironmentSnapshot{status: "available", values: map[string]string{}}
	}
	// ps eww separates entries with whitespace, which is also valid inside an
	// environment value. Its text format cannot prove any non-empty entry
	// boundary, so no value may be inferred from it.
	return darwinHookEnvironmentSnapshot{status: "conflicting", reason: darwinHookReasonParseAmbiguous}
}

func printDarwinHookEnvironment(diagnosticContext context.Context, snapshot darwinHookEnvironmentSnapshot, lsof darwinHookLsofEvidence) {
	for _, summary := range darwinHookEnvironmentSummary(snapshot, darwinHookFenceListeners(lsof)) {
		if !darwinHookEvidenceActive(diagnosticContext) {
			return
		}
		fmt.Println("DIAG-HOOK: Claude environment", summary)
	}
}

func darwinHookEnvironmentSummary(snapshot darwinHookEnvironmentSnapshot, listeners map[string]struct{}) []string {
	summary := "snapshot=" + snapshot.status
	if snapshot.status != "available" {
		return []string{summary + " reason=" + snapshot.reason.fixedCode()}
	}
	summaries := []string{summary}
	for _, name := range darwinHookEnvironmentNames {
		presence := "absent"
		if _, ok := snapshot.values[name]; ok {
			presence = "present"
		}
		summaries = append(summaries, name+"="+presence)
	}
	for _, pair := range [][2]string{{"HTTP_PROXY", "http_proxy"}, {"HTTPS_PROXY", "https_proxy"}, {"ALL_PROXY", "all_proxy"}, {"NODE_EXTRA_CA_CERTS", "REPOWOLF_CA_FILE"}} {
		summaries = append(summaries, pair[0]+"/"+pair[1]+"="+darwinHookEnvironmentEquality(snapshot.values, pair[0], pair[1]))
	}
	for _, name := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"} {
		summaries = append(summaries, name+" endpoint="+darwinHookProxyClass(snapshot.values, name, listeners))
	}
	for _, name := range []string{"NO_PROXY", "no_proxy"} {
		summaries = append(summaries, name+"="+darwinHookNoProxyClass(snapshot.values, name))
	}
	for _, name := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		summaries = append(summaries, name+"="+darwinHookLocaleClass(snapshot.values, name))
	}
	for _, name := range []string{"NODE_EXTRA_CA_CERTS", "REPOWOLF_CA_FILE"} {
		summaries = append(summaries, name+"="+darwinHookPathClass(snapshot.values, name))
	}
	summaries = append(summaries, "FENCE_SANDBOX="+darwinHookFenceMarkerClass(snapshot.values))
	return summaries
}

func darwinHookEnvironmentEquality(values map[string]string, left, right string) string {
	leftValue, leftPresent := values[left]
	rightValue, rightPresent := values[right]
	if !leftPresent || !rightPresent {
		return "missing"
	}
	if leftValue == rightValue {
		return "equal"
	}
	return "different"
}

func darwinHookProxyClass(values map[string]string, name string, listeners map[string]struct{}) string {
	value, present := values[name]
	if !present {
		return "absent"
	}
	if value == "" {
		return "empty"
	}
	endpoint, err := url.ParseRequestURI(value)
	if err != nil || endpoint.Hostname() == "" {
		return "malformed"
	}
	address := net.ParseIP(endpoint.Hostname())
	if address == nil || !address.IsLoopback() {
		return "non-loopback"
	}
	if listeners == nil {
		return "loopback-listener-unavailable"
	}
	if _, found := listeners[net.JoinHostPort(address.String(), endpoint.Port())]; found {
		return "loopback-live-fence-listener"
	}
	return "loopback-no-matching-listener"
}

func darwinHookNoProxyClass(values map[string]string, name string) string {
	value, present := values[name]
	if !present {
		return "absent"
	}
	if value == "" {
		return "empty"
	}
	return "nonempty"
}

func darwinHookLocaleClass(values map[string]string, name string) string {
	value, present := values[name]
	if !present {
		return "absent"
	}
	switch value {
	case "C", "POSIX":
		return "C-or-POSIX"
	case "en_US.UTF-8":
		return "en_US.UTF-8"
	}
	if strings.Contains(strings.ToLower(value), "utf-8") || strings.Contains(strings.ToLower(value), "utf8") {
		return "other-UTF-8"
	}
	return "other-or-invalid"
}

func darwinHookPathClass(values map[string]string, name string) string {
	value, present := values[name]
	if !present {
		return "absent"
	}
	if value == "" {
		return "empty"
	}
	if strings.HasPrefix(value, "/nix/store/") {
		return "Nix-store"
	}
	if strings.HasPrefix(value, "/tmp/") || strings.HasPrefix(value, "/private/tmp/") {
		return "temporary-absolute-path"
	}
	if filepath.IsAbs(value) {
		return "other-absolute-path"
	}
	return "relative-path"
}

func darwinHookFenceMarkerClass(values map[string]string) string {
	value, present := values["FENCE_SANDBOX"]
	if !present {
		return "absent"
	}
	if value == "" {
		return "empty"
	}
	if value == "1" || value == "true" {
		return "enabled-marker"
	}
	return "other-marker"
}

func darwinHookFenceListeners(lsof darwinHookLsofEvidence) map[string]struct{} {
	if lsof.status != "ok" {
		return nil
	}
	listeners := make(map[string]struct{})
	for _, line := range strings.Split(lsof.output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "fence" {
			continue
		}
		for index, field := range fields {
			if field != "TCP" || index+2 >= len(fields) || fields[index+2] != "(LISTEN)" {
				continue
			}
			host, port, err := net.SplitHostPort(fields[index+1])
			if err != nil {
				continue
			}
			address := net.ParseIP(host)
			if address != nil && address.IsLoopback() {
				listeners[net.JoinHostPort(address.String(), port)] = struct{}{}
			}
		}
	}
	return listeners
}

func TestDarwinHookEnvironmentSummarySanitizesValues(t *testing.T) {
	const secret = "https://token.example.invalid/hidden"
	snapshot := darwinHookEnvironmentSnapshot{status: "available", values: map[string]string{
		"HTTP_PROXY":          "http://127.0.0.1:4567",
		"http_proxy":          "http://127.0.0.1:4567",
		"NO_PROXY":            "internal.example.invalid",
		"LC_ALL":              "en_US.UTF-8",
		"NODE_EXTRA_CA_CERTS": "/private/tmp/ca.pem",
		"REPOWOLF_CA_FILE":    "/private/tmp/ca.pem",
		"FENCE_SANDBOX":       "1",
		"UNLISTED":            secret,
	}}
	summary := strings.Join(darwinHookEnvironmentSummary(snapshot, map[string]struct{}{net.JoinHostPort("127.0.0.1", "4567"): {}}), "\n")
	if strings.Contains(summary, secret) || strings.Contains(summary, "internal.example.invalid") || strings.Contains(summary, "/private/tmp/ca.pem") {
		t.Fatalf("summary disclosed a raw environment value: %q", summary)
	}
	for _, expected := range []string{"HTTP_PROXY endpoint=loopback-live-fence-listener", "HTTP_PROXY/http_proxy=equal", "LC_ALL=en_US.UTF-8", "NODE_EXTRA_CA_CERTS=temporary-absolute-path", "FENCE_SANDBOX=enabled-marker"} {
		if !strings.Contains(summary, expected) {
			t.Fatalf("summary missing %q: %q", expected, summary)
		}
	}
}

const darwinHookHostileDiagnostic = `error="permission denied" command="/bin/ps eww -p 42" path="/private/tmp/secret" value="TOKEN=secret" count=42 credential="secret" url="https://host.example.invalid" host="host.example.invalid"`

func TestDarwinHookEnvironmentReasonOutputContract(t *testing.T) {
	tests := []struct {
		name     string
		snapshot darwinHookEnvironmentSnapshot
		want     string
	}{
		{name: "available omits reason", snapshot: darwinHookEnvironmentSnapshot{status: "available", reason: darwinHookReasonIdentityMismatch}, want: "snapshot=available"},
		{name: "identity mismatch", snapshot: darwinHookEnvironmentSnapshot{status: "unavailable", reason: darwinHookReasonIdentityMismatch}, want: "snapshot=unavailable reason=identity-mismatch"},
		{name: "PID unavailable", snapshot: darwinHookEnvironmentSnapshot{status: "conflicting", reason: darwinHookReasonPIDUnavailable}, want: "snapshot=conflicting reason=pid-unavailable"},
		{name: "capture failed", snapshot: darwinHookEnvironmentSnapshot{status: "truncated", reason: darwinHookReasonCaptureFailed}, want: "snapshot=truncated reason=capture-failed"},
		{name: "parse ambiguous", snapshot: darwinHookEnvironmentSnapshot{status: "conflicting", reason: darwinHookReasonParseAmbiguous}, want: "snapshot=conflicting reason=parse-ambiguous"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			summaries := darwinHookEnvironmentSummary(test.snapshot, nil)
			got := strings.Join(summaries, "\n")
			if test.snapshot.status == "available" {
				if summaries[0] != test.want || strings.Contains(got, "reason=") || !strings.Contains(got, "HTTP_PROXY=absent") {
					t.Fatalf("available summary = %q, want status %q without a reason and with sanitized summaries", got, test.want)
				}
				return
			}
			if got != test.want {
				t.Fatalf("summary = %q, want %q", got, test.want)
			}
		})
	}
}

func TestDarwinHookEnvironmentReasonOutputSanitizesData(t *testing.T) {
	snapshot := darwinHookEnvironmentSnapshot{
		status: "unavailable",
		reason: darwinHookEnvironmentReason(255),
		values: map[string]string{"UNLISTED": darwinHookHostileDiagnostic},
	}
	got := strings.Join(darwinHookEnvironmentSummary(snapshot, nil), "\n")
	const want = "snapshot=unavailable reason=capture-failed"
	if got != want {
		t.Fatalf("summary = %q, want %q", got, want)
	}
}

func TestParseDarwinHookEnvironmentRejectsAmbiguity(t *testing.T) {
	command := "/nix/store/hash-claude-code-2.1.158/bin/claude"
	if snapshot := parseDarwinHookEnvironment(command, command); snapshot.status != "available" || snapshot.reason != darwinHookReasonNone || len(snapshot.values) != 0 {
		t.Fatalf("empty environment snapshot = %#v", snapshot)
	}
	for _, output := range []string{
		command + " HTTP_PROXY=http://127.0.0.1:1",
		command + " HTTP_PROXY=http://127.0.0.1:1 HTTP_PROXY=http://127.0.0.1:2",
		command + "x HTTP_PROXY=http://127.0.0.1:1",
		command + " HTTP_PROXY=http://127.0.0.1:1\nsecond line",
		command + " UNLISTED=prefix HTTP_PROXY=http://127.0.0.1:4567",
	} {
		if snapshot := parseDarwinHookEnvironment(command, output); snapshot.status != "conflicting" || snapshot.reason != darwinHookReasonParseAmbiguous {
			t.Fatalf("ambiguous output snapshot = %#v", snapshot)
		}
	}
}

func TestDarwinHookEnvironmentCaptureReasons(t *testing.T) {
	const command = "/nix/store/hash-claude-code-2.1.158/bin/claude"
	tests := []struct {
		name      string
		command   string
		contents  string
		truncated bool
		available bool
		err       error
		status    string
		reason    darwinHookEnvironmentReason
	}{
		{name: "snapshot unavailable", command: command, available: false, status: "unavailable", reason: darwinHookReasonCaptureFailed},
		{name: "command error", command: command, contents: darwinHookHostileDiagnostic, available: true, err: fmt.Errorf("%s", darwinHookHostileDiagnostic), status: "unavailable", reason: darwinHookReasonCaptureFailed},
		{name: "error precedes overflow", command: darwinHookHostileDiagnostic, contents: darwinHookHostileDiagnostic, truncated: true, available: true, err: fmt.Errorf("%s", darwinHookHostileDiagnostic), status: "unavailable", reason: darwinHookReasonCaptureFailed},
		{name: "overflow", command: command, truncated: true, available: true, status: "truncated", reason: darwinHookReasonCaptureFailed},
		{name: "empty capture", command: command, available: true, status: "conflicting", reason: darwinHookReasonParseAmbiguous},
		{name: "ambiguous environment bytes", command: command, contents: command + " HTTP_PROXY=http://127.0.0.1:1", available: true, status: "conflicting", reason: darwinHookReasonParseAmbiguous},
		{name: "proven empty environment", command: command, contents: command, available: true, status: "available", reason: darwinHookReasonNone},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := classifyDarwinHookEnvironmentCapture(test.command, test.contents, test.truncated, test.available, test.err)
			requireDarwinHookEnvironmentResult(t, got, test.status, test.reason)
			if test.err != nil {
				if summary := strings.Join(darwinHookEnvironmentSummary(got, nil), "\n"); summary != "snapshot=unavailable reason=capture-failed" {
					t.Fatalf("error summary = %q", summary)
				}
			}
		})
	}

	t.Run("complete 64 KiB capture", func(t *testing.T) {
		command := strings.Repeat("c", darwinHookObservationLimit)
		output := &lockedBuffer{limit: darwinHookObservationLimit}
		_, _ = output.Write([]byte(command))
		contents, truncated, available := output.Snapshot(context.Background(), darwinHookObservationLimit)
		if truncated {
			t.Fatal("complete 64 KiB capture was truncated")
		}
		requireDarwinHookEnvironmentResult(t, classifyDarwinHookEnvironmentCapture(command, contents, truncated, available, nil), "available", darwinHookReasonNone)
	})
	t.Run("overflow capture", func(t *testing.T) {
		command := strings.Repeat("c", darwinHookObservationLimit)
		output := &lockedBuffer{limit: darwinHookObservationLimit}
		_, _ = output.Write([]byte(command + "c"))
		contents, truncated, available := output.Snapshot(context.Background(), darwinHookObservationLimit)
		if !truncated {
			t.Fatal("over-limit capture was not truncated")
		}
		requireDarwinHookEnvironmentResult(t, classifyDarwinHookEnvironmentCapture(command, contents, truncated, available, nil), "truncated", darwinHookReasonCaptureFailed)
	})
	t.Run("caller wait deadline", func(t *testing.T) {
		diagnosticContext, cancel := context.WithCancel(context.Background())
		cancel()
		done := make(chan darwinHookEnvironmentSnapshot)
		evidence := waitForDarwinHookEnvironment(diagnosticContext, darwinHookProcessEvidence{}, done)
		requireDarwinHookEnvironmentResult(t, evidence.environment, "unavailable", darwinHookReasonCaptureFailed)
	})
}

func requireDarwinHookEnvironmentResult(t *testing.T, got darwinHookEnvironmentSnapshot, wantStatus string, wantReason darwinHookEnvironmentReason) {
	t.Helper()
	if got.status != wantStatus || got.reason != wantReason {
		t.Fatalf("snapshot = status %q, reason %d; want status %q, reason %d", got.status, got.reason, wantStatus, wantReason)
	}
}

func TestDarwinHookEnvironmentSelectionReasons(t *testing.T) {
	const (
		fence   = "/nix/store/hash-fence-0.1.58/bin/fence"
		wrapper = "/nix/store/hash-den-claude-agent/bin/den-claude-agent"
		claude  = "/nix/store/hash-claude-code-2.1.158/bin/claude"
	)
	manifest := []byte(`{"fenceExecutable":"` + fence + `","agent":{"executable":"` + wrapper + `"}}`)
	wrapperContents := []byte("export NODE_EXTRA_CA_CERTS=\"$REPOWOLF_CA_FILE\"\nexec " + claude + " \"$@\"\n")
	resolveIdentity := func(path string) (string, error) { return path, nil }
	processOutput := func(commands ...string) string {
		var lines []string
		for index, command := range commands {
			lines = append(lines, fmt.Sprintf("%d 1 7 S 00:01 %s", index+10, command))
		}
		return strings.Join(lines, "\n")
	}
	matchingFence := fence + " --settings policy -- " + wrapper

	t.Run("pre-selector deadline", func(t *testing.T) {
		diagnosticContext, cancel := context.WithCancel(context.Background())
		cancel()
		got, failed := darwinHookPreselectionFailure(diagnosticContext)
		if !failed {
			t.Fatal("pre-selector deadline was not classified")
		}
		requireDarwinHookEnvironmentResult(t, got, "unavailable", darwinHookReasonPIDUnavailable)
	})
	t.Run("manifest deadline", func(t *testing.T) {
		got, failed := darwinHookManifestFailure(false, context.DeadlineExceeded)
		if !failed {
			t.Fatal("manifest deadline was not classified")
		}
		requireDarwinHookEnvironmentResult(t, got, "unavailable", darwinHookReasonIdentityMismatch)
	})

	for _, test := range []struct {
		name    string
		output  string
		binding []byte
		read    func(context.Context, string) ([]byte, bool, error)
		status  string
		reason  darwinHookEnvironmentReason
	}{
		{name: "invalid manifest", binding: []byte("not JSON"), status: "unavailable", reason: darwinHookReasonIdentityMismatch},
		{name: "no Fence match", output: processOutput("/other --settings policy"), binding: manifest, status: "unavailable", reason: darwinHookReasonPIDUnavailable},
		{name: "multiple Fence matches", output: processOutput(matchingFence, matchingFence), binding: manifest, status: "conflicting", reason: darwinHookReasonPIDUnavailable},
		{name: "missing wrapper separator", output: processOutput(fence + " --settings policy"), binding: manifest, status: "unavailable", reason: darwinHookReasonIdentityMismatch},
		{name: "multiple wrapper separators", output: processOutput(matchingFence + " -- second"), binding: manifest, status: "conflicting", reason: darwinHookReasonIdentityMismatch},
		{name: "wrapper differs from manifest", output: processOutput(fence + " --settings policy -- /nix/store/other-agent/bin/den-claude-agent"), binding: manifest, status: "unavailable", reason: darwinHookReasonIdentityMismatch},
		{name: "wrapper read unavailable", output: processOutput(matchingFence), binding: manifest, read: func(context.Context, string) ([]byte, bool, error) { return nil, false, context.DeadlineExceeded }, status: "unavailable", reason: darwinHookReasonIdentityMismatch},
		{name: "wrapper lacks exact identity", output: processOutput(matchingFence), binding: manifest, read: func(context.Context, string) ([]byte, bool, error) {
			return []byte("exec " + claude + " \"$@\"\n"), false, nil
		}, status: "unavailable", reason: darwinHookReasonIdentityMismatch},
		{name: "wrapper has two exec lines", output: processOutput(matchingFence), binding: manifest, read: func(context.Context, string) ([]byte, bool, error) {
			return append(wrapperContents, []byte("exec "+claude+" \"$@\"\n")...), false, nil
		}, status: "conflicting", reason: darwinHookReasonIdentityMismatch},
	} {
		t.Run(test.name, func(t *testing.T) {
			read := test.read
			if read == nil {
				read = func(context.Context, string) ([]byte, bool, error) { return wrapperContents, false, nil }
			}
			_, _, got := darwinHookClaudePIDFromBinding(context.Background(), test.output, 7, test.binding, fence, read, resolveIdentity)
			requireDarwinHookEnvironmentResult(t, got, test.status, test.reason)
		})
	}

	for _, test := range []struct {
		name    string
		process []darwinHookProcess
		status  string
		reason  darwinHookEnvironmentReason
	}{
		{name: "no Claude child", process: []darwinHookProcess{{pid: 10, pgid: 7, command: matchingFence}}, status: "unavailable", reason: darwinHookReasonPIDUnavailable},
		{name: "multiple Claude children", process: []darwinHookProcess{{pid: 10, pgid: 7, command: matchingFence}, {pid: 11, ppid: 10, pgid: 7, command: claude}, {pid: 12, ppid: 10, pgid: 7, command: claude}}, status: "conflicting", reason: darwinHookReasonPIDUnavailable},
		{name: "one Claude child", process: []darwinHookProcess{{pid: 10, pgid: 7, command: matchingFence}, {pid: 11, ppid: 10, pgid: 7, command: claude}}, status: "available", reason: darwinHookReasonNone},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, _, got := darwinHookClaudePID(test.process, fence, claude)
			requireDarwinHookEnvironmentResult(t, got, test.status, test.reason)
		})
	}
}

func TestDarwinHookClaudePIDRequiresExactBoundPaths(t *testing.T) {
	const (
		fence  = "/nix/store/hash-fence-0.1.58/bin/fence"
		claude = "/nix/store/hash-claude-code-2.1.158/bin/claude"
	)
	processes := func(fencePath, claudePath string, extra ...darwinHookProcess) []darwinHookProcess {
		result := []darwinHookProcess{
			{pid: 10, pgid: 1, command: fencePath + " --settings policy -- wrapper"},
			{pid: 11, ppid: 10, pgid: 1, command: claudePath + " --flag"},
		}
		return append(result, extra...)
	}
	for _, test := range []struct {
		name           string
		processes      []darwinHookProcess
		expectedFence  string
		expectedClaude string
		status         string
		reason         darwinHookEnvironmentReason
	}{
		{name: "exact paths", processes: processes(fence, claude), status: "available", reason: darwinHookReasonNone},
		{name: "non-store Claude lookalike", processes: processes(fence, "/tmp/not-claude-code-helper/bin/claude"), status: "unavailable", reason: darwinHookReasonPIDUnavailable},
		{name: "wrong Fence path", processes: processes("/tmp/fence/bin/fence", claude), status: "unavailable", reason: darwinHookReasonPIDUnavailable},
		{name: "traversal Fence path", processes: processes("/nix/store/../../tmp/fence/bin/fence", claude), status: "unavailable", reason: darwinHookReasonPIDUnavailable},
		{name: "traversal Claude path", processes: processes(fence, "/nix/store/../../tmp/claude/bin/claude"), status: "unavailable", reason: darwinHookReasonPIDUnavailable},
		{name: "traversal expected binding paths", processes: processes("/nix/store/../../tmp/fence/bin/fence", "/nix/store/../../tmp/claude/bin/claude"), expectedFence: "/nix/store/../../tmp/fence/bin/fence", expectedClaude: "/nix/store/../../tmp/claude/bin/claude", status: "unavailable", reason: darwinHookReasonIdentityMismatch},
		{name: "zero Claude candidates", processes: []darwinHookProcess{{pid: 10, pgid: 1, command: fence + " --settings policy -- wrapper"}}, status: "unavailable", reason: darwinHookReasonPIDUnavailable},
		{name: "multiple Claude candidates", processes: processes(fence, claude, darwinHookProcess{pid: 12, ppid: 10, pgid: 1, command: claude + " --other"}), status: "conflicting", reason: darwinHookReasonPIDUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			expectedFence, expectedClaude := test.expectedFence, test.expectedClaude
			if expectedFence == "" {
				expectedFence = fence
			}
			if expectedClaude == "" {
				expectedClaude = claude
			}
			pid, _, snapshot := darwinHookClaudePID(test.processes, expectedFence, expectedClaude)
			requireDarwinHookEnvironmentResult(t, snapshot, test.status, test.reason)
			if snapshot.status == "available" && pid != "11" {
				t.Fatalf("pid = %q, want 11", pid)
			}
		})
	}
}

func TestDarwinHookClaudePIDFromBindingRejectsWrapperEscapes(t *testing.T) {
	const (
		fence   = "/nix/store/hash-fence-0.1.58/bin/fence"
		claude  = "/nix/store/hash-claude-code-2.1.158/bin/claude"
		wrapper = "/nix/store/hash-den-claude-agent/bin/den-claude-agent"
	)
	manifest := func(wrapperPath string) []byte {
		return []byte(`{"fenceExecutable":"` + fence + `","agent":{"executable":"` + wrapperPath + `"}}`)
	}
	for _, test := range []struct {
		name     string
		wrapper  string
		resolve  func(string) (string, error)
		status   string
		reason   darwinHookEnvironmentReason
		pid      string
		wantRead bool
	}{
		{name: "canonical selector", wrapper: wrapper, resolve: func(path string) (string, error) { return path, nil }, status: "available", reason: darwinHookReasonNone, pid: "11", wantRead: true},
		{name: "traversal wrapper", wrapper: "/nix/store/../outside-store/wrapper", resolve: func(path string) (string, error) { return path, nil }, status: "unavailable", reason: darwinHookReasonIdentityMismatch},
		{name: "symlinked wrapper identity", wrapper: wrapper, resolve: func(path string) (string, error) {
			if path == wrapper {
				return "/nix/store/different-den-claude-agent/bin/den-claude-agent", nil
			}
			return path, nil
		}, status: "unavailable", reason: darwinHookReasonIdentityMismatch},
	} {
		t.Run(test.name, func(t *testing.T) {
			processes := "10 1 7 S 00:01 " + fence + " --settings policy -- " + test.wrapper + "\n" +
				"11 10 7 S 00:01 " + claude + " --flag\n"
			readCalled := false
			pid, _, snapshot := darwinHookClaudePIDFromBinding(
				context.Background(), processes, 7, manifest(test.wrapper), fence,
				func(context.Context, string) ([]byte, bool, error) {
					readCalled = true
					return []byte("export NODE_EXTRA_CA_CERTS=\"$REPOWOLF_CA_FILE\"\nexec " + claude + " \"$@\"\n"), false, nil
				},
				test.resolve,
			)
			if pid != test.pid {
				t.Fatalf("selector pid = %q, want %q", pid, test.pid)
			}
			requireDarwinHookEnvironmentResult(t, snapshot, test.status, test.reason)
			if readCalled != test.wantRead {
				t.Fatalf("wrapper read = %t, want %t", readCalled, test.wantRead)
			}
		})
	}
}

func TestDarwinHookBindingRejectsEscapes(t *testing.T) {
	const (
		fence   = "/nix/store/hash-fence-0.1.58/bin/fence"
		wrapper = "/nix/store/hash-den-claude-agent"
	)
	manifest := func(fencePath, wrapperPath string) []byte {
		return []byte(`{"fenceExecutable":"` + fencePath + `","agent":{"executable":"` + wrapperPath + `"}}`)
	}
	for _, test := range []struct {
		name     string
		contents []byte
		expected string
		resolve  func(string) (string, error)
		status   string
	}{
		{name: "exact binding", contents: manifest(fence, wrapper), expected: fence, resolve: func(path string) (string, error) { return path, nil }, status: "available"},
		{name: "traversal wrapper", contents: manifest(fence, "/nix/store/../../tmp/wrapper"), expected: fence, resolve: func(path string) (string, error) { return path, nil }, status: "unavailable"},
		{name: "symlink wrapper", contents: manifest(fence, wrapper), expected: fence, resolve: func(path string) (string, error) {
			if path == wrapper {
				return "/tmp/wrapper", nil
			}
			return path, nil
		}, status: "unavailable"},
		{name: "manifest fence disagrees with input", contents: manifest(fence, wrapper), expected: "/nix/store/other-fence/bin/fence", resolve: func(path string) (string, error) { return path, nil }, status: "unavailable"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, _, status := darwinHookBindingFromContents(test.contents, test.expected, test.resolve)
			if status != test.status {
				t.Fatalf("status = %q, want %q", status, test.status)
			}
		})
	}
}

func TestDarwinHookCanonicalNixStorePathRejectsEscapes(t *testing.T) {
	const clean = "/nix/store/hash-claude-code-2.1.158/bin/claude"
	for _, test := range []struct {
		name     string
		path     string
		resolved string
		want     bool
	}{
		{name: "clean canonical store path", path: clean, resolved: clean, want: true},
		{name: "traversal", path: "/nix/store/../../tmp/claude", resolved: "/tmp/claude"},
		{name: "nested traversal", path: "/nix/store/hash/../other/bin/claude", resolved: "/nix/store/other/bin/claude"},
		{name: "symlink outside store", path: clean, resolved: "/tmp/claude"},
		{name: "symlink to another store path", path: clean, resolved: "/nix/store/other-claude/bin/claude"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, got := darwinHookCanonicalNixStorePath(test.path, func(string) (string, error) { return test.resolved, nil })
			if got != test.want {
				t.Fatalf("canonical status = %t, want %t", got, test.want)
			}
		})
	}
}

func darwinHookProcessGroupPIDs(diagnosticContext context.Context, output string) []string {
	var processGroup string
	for _, line := range strings.Split(output, "\n") {
		if !darwinHookEvidenceActive(diagnosticContext) {
			return nil
		}
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		command := strings.Join(fields[5:], " ")
		if strings.Contains(command, "fence") && strings.Contains(command, "--settings") {
			processGroup = fields[2]
			break
		}
	}
	if processGroup == "" {
		return nil
	}
	var pids []string
	for _, line := range strings.Split(output, "\n") {
		if !darwinHookEvidenceActive(diagnosticContext) {
			return nil
		}
		fields := strings.Fields(line)
		if len(fields) < 6 || fields[2] != processGroup {
			continue
		}
		if _, err := strconv.Atoi(fields[0]); err == nil {
			pids = append(pids, fields[0])
		}
	}
	return pids
}

func policyPathFromDarwinHookPS(diagnosticContext context.Context, output string) (string, string) {
	for _, line := range strings.Split(output, "\n") {
		if !darwinHookEvidenceActive(diagnosticContext) {
			return "", ""
		}
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		command := strings.Fields(strings.Join(fields[5:], " "))
		if !strings.Contains(strings.Join(command, " "), "fence") {
			continue
		}
		for index, argument := range command[:len(command)-1] {
			if argument == "--settings" {
				return command[index+1], "fence --settings"
			}
		}
	}
	return "", ""
}

func printDarwinHookContextEvidence(diagnosticContext context.Context) {
	scratchPath, err := newestDarwinHookPath(diagnosticContext, filepath.Join("/private/tmp", fmt.Sprintf("den-%d", os.Getuid()), "scratch-*"))
	if err != nil {
		fmt.Println("DIAG-HOOK: hook context scratch lookup:", err)
	}
	if scratchPath == "" {
		fmt.Println("DIAG-HOOK: hook context unavailable (no scratch directory)")
		return
	}
	printDarwinHookFile(diagnosticContext, "hook context", filepath.Join(scratchPath, nestedFenceContextFile))
}

func newestDarwinHookPath(diagnosticContext context.Context, pattern string) (string, error) {
	if err := diagnosticContext.Err(); err != nil {
		return "", err
	}
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return "", err
	}
	var newest string
	var newestTime time.Time
	for _, path := range paths {
		if err := diagnosticContext.Err(); err != nil {
			return "", err
		}
		info, err := os.Stat(path)
		if err != nil || (!info.IsDir() && filepath.Base(path) != "policy.json") {
			continue
		}
		if newest == "" || info.ModTime().After(newestTime) {
			newest, newestTime = path, info.ModTime()
		}
	}
	return newest, nil
}

func printDarwinHookFile(diagnosticContext context.Context, label, path string) {
	contents, truncated, err := readDarwinHookRegularFile(diagnosticContext, path)
	if err != nil {
		fmt.Println("DIAG-HOOK:", label, "path=", path, "read:", err)
		return
	}
	fmt.Println("DIAG-HOOK:", label, "path=", path, "exists")
	if truncated {
		fmt.Println("DIAG-HOOK:", label, "truncated at", darwinHookObservationLimit, "bytes")
	}
	printDarwinHookOutput(diagnosticContext, "DIAG-HOOK:", string(contents))
}

func readDarwinHookRegularFile(diagnosticContext context.Context, path string) ([]byte, bool, error) {
	if err := diagnosticContext.Err(); err != nil {
		return nil, false, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, false, err
	}
	if !info.Mode().IsRegular() {
		return nil, false, fmt.Errorf("rejected non-regular mode %s", info.Mode())
	}
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, false, err
	}
	defer file.Close()
	info, err = file.Stat()
	if err != nil {
		return nil, false, err
	}
	if !info.Mode().IsRegular() {
		return nil, false, fmt.Errorf("rejected non-regular mode %s", info.Mode())
	}
	contents := make([]byte, 0, darwinHookObservationLimit)
	chunk := make([]byte, 4096)
	for len(contents) <= darwinHookObservationLimit {
		if err := diagnosticContext.Err(); err != nil {
			return nil, false, err
		}
		limit := len(chunk)
		if remaining := darwinHookObservationLimit + 1 - len(contents); remaining < limit {
			limit = remaining
		}
		count, readErr := file.Read(chunk[:limit])
		contents = append(contents, chunk[:count]...)
		if len(contents) > darwinHookObservationLimit {
			return contents[:darwinHookObservationLimit], true, nil
		}
		if readErr == io.EOF {
			return contents, false, nil
		}
		if readErr != nil {
			return nil, false, readErr
		}
		if count == 0 {
			return nil, false, io.ErrNoProgress
		}
	}
	return contents, true, nil
}

func printDarwinHookStream(diagnosticContext context.Context, label string, buffer *lockedBuffer) {
	contents, truncated, available := buffer.Snapshot(diagnosticContext, darwinHookObservationLimit)
	if !available {
		fmt.Println("DIAG-HOOK:", label, "snapshot unavailable before observation deadline")
		return
	}
	if truncated {
		fmt.Println("DIAG-HOOK:", label, "truncated at", darwinHookObservationLimit, "bytes")
	}
	printDarwinHookOutput(diagnosticContext, "DIAG-HOOK: "+label+":", contents)
}

func printDarwinHookOutput(diagnosticContext context.Context, prefix, output string) bool {
	truncated := len(output) > darwinHookObservationLimit
	if truncated {
		output = output[:darwinHookObservationLimit]
	}
	for _, line := range strings.Split(strings.TrimSuffix(output, "\n"), "\n") {
		if !darwinHookEvidenceActive(diagnosticContext) {
			return false
		}
		fmt.Println(prefix, line)
	}
	if truncated {
		fmt.Println(prefix, "output truncated at", darwinHookObservationLimit, "bytes")
	}
	return true
}

type lockedBuffer struct {
	mu        sync.Mutex
	buffer    bytes.Buffer
	limit     int
	truncated bool
}

func (buffer *lockedBuffer) Write(contents []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	if buffer.limit > 0 {
		remaining := buffer.limit - buffer.buffer.Len()
		if remaining <= 0 {
			buffer.truncated = true
			return len(contents), nil
		}
		if len(contents) > remaining {
			_, _ = buffer.buffer.Write(contents[:remaining])
			buffer.truncated = true
			return len(contents), nil
		}
	}
	return buffer.buffer.Write(contents)
}

func (buffer *lockedBuffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.buffer.String()
}

func (buffer *lockedBuffer) Snapshot(diagnosticContext context.Context, limit int) (string, bool, bool) {
	for {
		if diagnosticContext.Err() != nil {
			return "", false, false
		}
		if buffer.mu.TryLock() {
			if diagnosticContext.Err() != nil {
				buffer.mu.Unlock()
				return "", false, false
			}
			contents := buffer.buffer.Bytes()
			truncated := buffer.truncated || len(contents) > limit
			if truncated {
				contents = contents[:limit]
			}
			snapshot := string(contents)
			buffer.mu.Unlock()
			return snapshot, truncated, true
		}
		select {
		case <-diagnosticContext.Done():
			return "", false, false
		case <-time.After(time.Millisecond):
		}
	}
}

func startDarwinDiagnosticClaude(diagnosticContext context.Context, fixture *nativeFixture, scenario *claudeScenario, extraEnvironment []string) (*exec.Cmd, <-chan commandResult, *lockedBuffer, *lockedBuffer, error) {
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
	stdout, stderr := &lockedBuffer{}, &lockedBuffer{}
	command.Stdout, command.Stderr = stdout, stderr
	if err := command.Start(); err != nil {
		return nil, nil, nil, nil, err
	}
	results := make(chan commandResult, 1)
	go func() {
		err := command.Wait()
		results <- commandResult{stdout: stdout.String(), stderr: stderr.String(), err: err}
	}()
	return command, results, stdout, stderr, nil
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

func darwinPolicyMutationDiagnostic(name, mutation string) string {
	return fmt.Sprintf(`set +e
mutation_stderr="$DEN_FENCE_TMPDIR/mutation-stderr"
2>"$mutation_stderr" %s
operation_status=$?
set -e
printf 'operation=%%s status=%%s\n' %s "$operation_status"
if policy_state=$(stat -f 'size=%%z mode=%%Mp%%Lp perms=%%Sp' "$DEN_FENCE_POLICY_FILE" 2>&1); then
  printf 'policy=exists %%s\n' "$policy_state"
else
  policy_status=$?
  printf 'policy=missing-or-unreadable status=%%s detail=%%s\n' "$policy_status" "$policy_state"
fi
if test -s "$mutation_stderr"; then
  printf 'seatbelt-evidence-begin\n'
  cat "$mutation_stderr"
  printf 'seatbelt-evidence-end\n'
fi
true`, mutation, strconv.Quote(name))
}

func TestDarwinBashHook(t *testing.T) {
	fixture := newNativeFixture(t)
	fixture.startDNS(t)
	fixture.installClaudeContextHook(t)
	replacement := filepath.Join(fixture.worktree, "replacement-policy")
	if err := os.WriteFile(replacement, []byte("replacement\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mutations := map[string]string{
		"truncate": `: > "$DEN_FENCE_POLICY_FILE"`,
		"replace":  `cp "$DEN_NATIVE_REPLACEMENT" "$DEN_FENCE_POLICY_FILE"`,
		"rename":   `mv "$DEN_FENCE_POLICY_FILE" "$DEN_FENCE_POLICY_FILE.old"`,
		"chmod":    `chmod u+w "$DEN_FENCE_POLICY_FILE"`,
		"append":   `printf mutation >> "$DEN_FENCE_POLICY_FILE"`,
	}
	for name, mutation := range mutations {
		t.Run(name, func(t *testing.T) {
			marker := filepath.Join(fixture.worktree, "blocked-"+name)
			blocked := `gh repo create forbidden; printf escaped > "$DEN_NATIVE_BLOCKED_MARKER"`
			scenario := fixture.provider.register(darwinPolicyMutationDiagnostic(name, mutation), blocked)
			requestsBefore := fixture.provider.messageRequestCount()
			traceBefore := len(fixture.provider.requestTraceSnapshot())
			result := fixture.launchClaude(scenario, []string{
				"DEN_NATIVE_REPLACEMENT=" + replacement,
				"DEN_NATIVE_BLOCKED_MARKER=" + marker,
			})
			toolResults := fixture.provider.scenarioResults(scenario)
			requestCount := fixture.provider.messageRequestCount() - requestsBefore
			requestTrace := fixture.provider.requestTraceSnapshot()[traceBefore:]
			if result.err != nil || fileExists(marker) || len(toolResults) != 2 {
				t.Fatalf("policy mutation %s scenario=%s: launch=%v marker=%t provider requests=%d trace=%v DNS=%v tool results=%#v\nstdout:\n%s\nstderr:\n%s", name, scenario.id, result.err, fileExists(marker), requestCount, requestTrace, fixture.dns.names(), toolResults, result.stdout, result.stderr)
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
	fixture.startDNS(t)
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
	fixture.startDNS(t)
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
	reportDarwinDiskTelemetry("before-claude-execution",
		darwinTelemetryPath{label: "fixture-worktree", path: fixture.worktree},
		darwinTelemetryPath{label: "private-tmp", path: "/private/tmp"},
		darwinTelemetryPath{label: "nix-store", path: "/nix/store"},
	)
	return runCommand(command)
}
