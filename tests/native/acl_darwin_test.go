//go:build native && darwin

package native

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
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
		psOutput := dumpDarwinHookProcesses(evidenceContext, fixtureBash, command.Process.Pid)
		printDarwinHookObservation(evidenceContext, fixture, scenario, psOutput, hookEvidence, stdout, stderr)
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

func dumpDarwinHookProcesses(diagnosticContext context.Context, fixtureBash string, processGroup int) string {
	psContext, cancel := context.WithTimeout(diagnosticContext, 2*time.Second)
	ps := exec.CommandContext(psContext, "ps", "-Ao", "pid,ppid,pgid,stat,etime,command")
	ps.WaitDelay = 2 * time.Second
	output, err := ps.CombinedOutput()
	cancel()
	fmt.Println("DIAG-HOOK: ps:", err)
	printDiagnosticOutput("DIAG-HOOK:", string(output))
	if diagnosticContext.Err() != nil {
		fmt.Println("DIAG-HOOK: overall diagnostic deadline reached before samples:", diagnosticContext.Err())
		return string(output)
	}
	if _, err := os.Stat("/usr/bin/sample"); err != nil {
		fmt.Println("DIAG-HOOK: /usr/bin/sample unavailable:", err)
		return string(output)
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
	return string(output)
}

const darwinHookObservationLimit = 64 * 1024

func printDarwinHookObservation(diagnosticContext context.Context, fixture *nativeFixture, scenario *claudeScenario, psOutput, hookEvidence string, stdout, stderr *lockedBuffer) {
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
	fmt.Println("DIAG-HOOK: egress summary requests=" + strconv.Itoa(requests) + " providerMessageRequests=" + strconv.Itoa(providerMessageRequests) + " dnsNames=" + strconv.Itoa(len(dnsNames)) + " policyContent=" + policyContent + " lsof=" + lsof)
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

func printDarwinHookLsof(diagnosticContext context.Context, psOutput string) string {
	pids := darwinHookProcessGroupPIDs(diagnosticContext, psOutput)
	if !darwinHookEvidenceActive(diagnosticContext) {
		return "error"
	}
	if len(pids) == 0 {
		fmt.Println("DIAG-HOOK: lsof unavailable (no launched process-group pids)")
		return "error"
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
		return "error"
	}
	if truncated {
		fmt.Println("DIAG-HOOK: lsof output truncated at", darwinHookObservationLimit, "bytes")
	}
	if !printDarwinHookOutput(diagnosticContext, "DIAG-HOOK:", contents) {
		return "error"
	}
	if err != nil || truncated {
		return "error"
	}
	return "ok"
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
