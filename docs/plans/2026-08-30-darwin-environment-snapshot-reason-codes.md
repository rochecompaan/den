# Darwin Environment Snapshot Reason Codes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add one sanitized fixed reason code to every non-available Darwin Claude environment snapshot without changing any existing status, evidence boundary, timing, assertion, cleanup, or enforcement behavior.

**Architecture:** Keep the change inside `tests/native/acl_darwin_test.go`. Extend `darwinHookEnvironmentSnapshot` with typed reason metadata, classify each existing identity, PID, capture, and parse branch at its current evidence boundary, and render only fixed reason strings. Add focused pure tests that execute the Darwin-tagged test logic on Linux through Go's explicit source-file mode, then retain the established Darwin cross-compilation and Nix verification gates.

**Tech Stack:** Go (`native && darwin` test harness, table-driven tests, `context.Context`), Nix flake checks, Git.

**Approved specification:** `docs/specs/2026-08-30-darwin-environment-snapshot-reason-codes-design.md` at commit `6c4c9ec716c8b6b95a382a7f29722e04b50824cc`.

## Global Constraints

- A later implementation can modify only `tests/native/acl_darwin_test.go`.
- Keep enforcement fail-closed.
- Preserve the existing status codes exactly: `available`, `unavailable`, `truncated`, and `conflicting`.
- The only allowed reason codes are `identity-mismatch`, `pid-unavailable`, `capture-failed`, and `parse-ambiguous`.
- Every non-available snapshot must emit exactly one fixed reason field on the status line. An available snapshot must omit the reason field.
- Emit no raw errors, commands, paths, values, counts, credentials, tokens, URLs, host names, or arbitrary host data in a reason diagnostic.
- Preserve the existing manifest, canonical Fence executable, Fence command, Claude wrapper, and real Claude executable identity proof before environment capture.
- Preserve the existing combined Fence predicate. Do not split it or add another process probe.
- Preserve the 45-second observation point, 5-second evidence budget, 60-second absolute deadline, 64 KiB capture cap, derived command timeouts, and output caps.
- Preserve existing process output, sampling, listener capture, stream output, assertions, diagnostic ordering, TERM/KILL cleanup, drain, and dead-process confirmation.
- Preserve concurrent bounded capture. Do not add retries, host-path allowances, writable trust copies, skipped assertions, enforcement exceptions, or broader process/file reads.
- No production, Nix, launch, environment, policy, profile, timeout, assertion, Linux, workflow, or repair change is allowed.
- Do not infer a Darwin cause or authorize a repair from a reason code.
- Do not push, rerun a workflow, amend, force-push, merge, or close PR #1 during this plan.
- Stop and request a new design decision if correct classification needs a new status, reason code, raw diagnostic data, capture source, launch input, broader host read, larger cap, longer budget, retry, timeout change, assertion change, or weaker enforcement.

---

## File Structure

- Modify and test: `tests/native/acl_darwin_test.go`
  - `darwinHookEnvironmentSnapshot` owns status, fixed reason metadata, and sanitized values.
  - Existing identity and PID selectors preserve their current status branches and return a snapshot classification instead of a bare status string.
  - One pure capture classifier owns command-error, unavailable-snapshot, overflow, and parse precedence.
  - `darwinHookEnvironmentSummary` is the only reason-output formatter and cannot render arbitrary reason text.
  - Focused table-driven tests cover every approved mapping and the output-sanitization contract.
- Create no source, test, Nix, script, fixture, or workflow file.
- Use `/tmp/den-darwin-reason-pure-tests` only as an untracked local test runner during implementation; remove it before completion.

### Task 1: Add sanitized snapshot reason metadata

**Files:**
- Modify: `tests/native/acl_darwin_test.go:285-614`
- Modify: `tests/native/acl_darwin_test.go:788-856`
- Test: `tests/native/acl_darwin_test.go:985-1177`
- Verify unchanged behavior around: `tests/native/acl_darwin_test.go:197-265`, `tests/native/acl_darwin_test.go:616-785`, and `tests/native/acl_darwin_test.go:1365-1524`

**Interfaces:**
- Consumes: existing process evidence, manifest binding, Fence and Claude identity helpers, `lockedBuffer.Snapshot`, `parseDarwinHookEnvironment`, and `darwinHookEnvironmentSummary`.
- Produces: `darwinHookEnvironmentReason`, four fixed reason constants, reason-bearing `darwinHookEnvironmentSnapshot` values, `classifyDarwinHookEnvironmentCapture`, and one sanitized status/reason summary line.
- Preserves: existing status strings and every existing timing, capture, output, assertion, and cleanup interface.

- [ ] **Step 1: Guard the approved base, clean worktree, and one-file implementation scope**

Run from `/home/roche/projects/den/.worktrees/den-claude-sandbox-design`:

```bash
set -euo pipefail
test "$(git branch --show-current)" = "feat/den-claude-sandbox"
git merge-base --is-ancestor 6c4c9ec716c8b6b95a382a7f29722e04b50824cc HEAD
git diff --quiet
git diff --cached --quiet
test -z "$(git status --porcelain)"
test -f docs/specs/2026-08-30-darwin-environment-snapshot-reason-codes-design.md
test -f docs/plans/2026-08-30-darwin-environment-snapshot-reason-codes.md
```

Expected: every command exits 0. Stop if the plan is not committed on a clean branch before implementation starts.

- [ ] **Step 2: Create and validate the untracked focused pure-test runner**

The ordinary package `TestMain` expects packaged fixture variables, and the Darwin file is excluded on Linux. Explicit Go source-file mode includes the Darwin file, excludes `acl_linux_test.go`, and runs only pure tests without changing any tracked file.

```bash
cat > /tmp/den-darwin-reason-pure-tests <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
pattern=${1:?focused test pattern is required}
root=$(git rev-parse --show-toplevel)
cd "$root"
fixture=/tmp/den-native-pure-test-placeholder
env \
  DEN_NATIVE_CLAUDE="$fixture" \
  DEN_NATIVE_SANDBOX="$fixture" \
  DEN_NATIVE_MANIFEST="$fixture" \
  DEN_NATIVE_LAUNCHER="$fixture" \
  DEN_NATIVE_FENCE="$fixture" \
  DEN_NATIVE_REPOWOLF_CLIENT_DIR="$fixture" \
  DEN_NATIVE_REPOWOLF_FIXTURE="$fixture" \
  DEN_NATIVE_SETTINGS_MERGE="$fixture" \
  DEN_NATIVE_UNRELATED_STORE_FILE="$fixture" \
  go test \
    tests/native/claude_fixture.go \
    tests/native/fixtures.go \
    tests/native/nested_witness.go \
    tests/native/network_fixture.go \
    tests/native/native_test.go \
    tests/native/nested_witness_test.go \
    tests/native/acl_darwin_test.go \
    -run "$pattern" \
    -count=1
EOF
chmod 0700 /tmp/den-darwin-reason-pure-tests
/tmp/den-darwin-reason-pure-tests \
  '^(TestDarwinHookEnvironmentSummarySanitizesValues|TestParseDarwinHookEnvironmentRejectsAmbiguity)$'
```

Expected: `ok command-line-arguments`. This is pure-logic coverage only; it is not Darwin runtime evidence.

- [ ] **Step 3: RED — add output-contract and sanitization tests before reason implementation**

Add `TestDarwinHookEnvironmentReasonOutputContract` with literal expected summaries:

```go
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
			got := strings.Join(darwinHookEnvironmentSummary(test.snapshot, nil), "\n")
			if got != test.want {
				t.Fatalf("summary = %q, want %q", got, test.want)
			}
		})
	}
}
```

Add `TestDarwinHookEnvironmentReasonOutputSanitizesData`. Define this exact hostile test payload near the reason tests:

```go
const darwinHookHostileDiagnostic = `error="permission denied" command="/bin/ps eww -p 42" path="/private/tmp/secret" value="TOKEN=secret" count=42 credential="secret" url="https://host.example.invalid" host="host.example.invalid"`
```

Construct this snapshot:

```go
snapshot := darwinHookEnvironmentSnapshot{
	status: "unavailable",
	reason: darwinHookEnvironmentReason(255),
	values: map[string]string{"UNLISTED": darwinHookHostileDiagnostic},
}
```

Assert the complete result, not a source string or partial implementation detail:

```go
want := "snapshot=unavailable reason=capture-failed"
```

The later capture RED/GREEN cycle must pass the same hostile payload as a command, captured value, and command error. Keeping that assertion in the capture test prevents implementation of the classifier before its failing test.

Run:

```bash
set +e
/tmp/den-darwin-reason-pure-tests \
  '^(TestDarwinHookEnvironmentReasonOutputContract|TestDarwinHookEnvironmentReasonOutputSanitizesData)$'
red_status=$?
set -e
test "$red_status" -ne 0
```

Expected RED: compilation fails because the reason type, constants, and field do not exist. Confirm those missing interfaces are the cause; do not proceed on a typo or unrelated failure.

- [ ] **Step 4: GREEN — add the fixed reason data model and sanitized formatter**

Add the closed reason representation beside `darwinHookEnvironmentSnapshot`:

```go
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
```

The default is fixed and fail-closed. It prevents a missing or invalid internal reason value from reaching output as arbitrary text; the branch-level tests in later steps still require every real branch to set its exact approved reason.

Change the start of `darwinHookEnvironmentSummary` to:

```go
summary := "snapshot=" + snapshot.status
if snapshot.status != "available" {
	return []string{summary + " reason=" + snapshot.reason.fixedCode()}
}
summaries := []string{summary}
```

Keep the existing available-only allowlist, equality, proxy, locale, path, Fence marker, and listener summaries byte-for-byte below that point.

Do not add `classifyDarwinHookEnvironmentCapture` in this cycle. Its first implementation belongs after the failing capture tests in Steps 7 and 8.

Run:

```bash
gofmt -w tests/native/acl_darwin_test.go
/tmp/den-darwin-reason-pure-tests \
  '^(TestDarwinHookEnvironmentReasonOutputContract|TestDarwinHookEnvironmentReasonOutputSanitizesData)$'
```

Expected GREEN: both tests pass. A mutation that appends `snapshot.reason` directly, emits available reasons, or emits values for a non-available snapshot must fail at least one test.

- [ ] **Step 5: RED — add pure identity and PID reason-mapping tests**

Add `TestDarwinHookEnvironmentSelectionReasons` and extend the two existing selector tests. Use the real selector functions with hand-written process records, manifest bytes, wrapper bytes, and resolver/read seams. Do not assert on a mock call as the behavior under test.

Use these literal cases and expectations:

| Evidence case | Existing status | Required reason | Real function under test |
| --- | --- | --- | --- |
| Canceled context at the pre-selector gate | `unavailable` | `pid-unavailable` | new `darwinHookPreselectionFailure` used by `dumpDarwinHookProcesses` |
| `context.DeadlineExceeded` from manifest reading | `unavailable` | `identity-mismatch` | new `darwinHookManifestFailure` used by `darwinHookClaudeProcess` |
| Invalid manifest JSON | `unavailable` | `identity-mismatch` | `darwinHookClaudePIDFromBinding` |
| No process matching exact Fence plus nonempty `--settings` | `unavailable` | `pid-unavailable` | `darwinHookClaudePIDFromBinding` |
| Two processes matching exact Fence plus nonempty `--settings` | `conflicting` | `pid-unavailable` | `darwinHookClaudePIDFromBinding` |
| One matching Fence with no `--` wrapper separator | `unavailable` | `identity-mismatch` | `darwinHookClaudePIDFromBinding` |
| One matching Fence with two `--` separators | `conflicting` | `identity-mismatch` | `darwinHookClaudePIDFromBinding` |
| Wrapper path disagrees with the manifest | `unavailable` | `identity-mismatch` | `darwinHookClaudePIDFromBinding` |
| Wrapper read is unavailable or lacks the exact CA export/Claude exec | `unavailable` | `identity-mismatch` | `darwinHookClaudePIDFromBinding` |
| Wrapper contains two candidate `exec` lines | `conflicting` | `identity-mismatch` | `darwinHookClaudePIDFromBinding` |
| Proven Fence and Claude identity with no Claude child | `unavailable` | `pid-unavailable` | `darwinHookClaudePID` |
| Proven Fence and Claude identity with two Claude children | `conflicting` | `pid-unavailable` | `darwinHookClaudePID` |
| Proven Fence and Claude identity with one Claude child | `available` | `darwinHookReasonNone` | `darwinHookClaudePID` |

Use these canonical fixtures throughout the table:

```go
const (
	fence   = "/nix/store/hash-fence-0.1.58/bin/fence"
	wrapper = "/nix/store/hash-den-claude-agent/bin/den-claude-agent"
	claude  = "/nix/store/hash-claude-code-2.1.158/bin/claude"
)
manifest := []byte(`{"fenceExecutable":"` + fence + `","agent":{"executable":"` + wrapper + `"}}`)
wrapperContents := []byte("export NODE_EXTRA_CA_CERTS=\"$REPOWOLF_CA_FILE\"\nexec " + claude + " \"$@\"\n")
resolveIdentity := func(path string) (string, error) { return path, nil }
```

Add a test assertion helper that compares both fields with literal expectations:

```go
func requireDarwinHookEnvironmentResult(t *testing.T, got darwinHookEnvironmentSnapshot, wantStatus string, wantReason darwinHookEnvironmentReason) {
	t.Helper()
	if got.status != wantStatus || got.reason != wantReason {
		t.Fatalf("snapshot = status %q, reason %d; want status %q, reason %d", got.status, got.reason, wantStatus, wantReason)
	}
}
```

Update `TestDarwinHookClaudePIDRequiresExactBoundPaths` expectations as follows:

- `exact paths`: `available`, `darwinHookReasonNone`.
- `non-store Claude lookalike`, `wrong Fence path`, `traversal Fence path`, `traversal Claude path`, and `zero Claude candidates`: preserve their existing status and use `pid-unavailable` because the valid expected identity does not select one live process.
- `traversal expected binding paths`: preserve `unavailable` and use `identity-mismatch` because the expected identity itself is invalid.
- `multiple Claude candidates`: preserve `conflicting` and use `pid-unavailable`.

Update `TestDarwinHookClaudePIDFromBindingRejectsWrapperEscapes` expectations as follows:

- `canonical selector`: `available`, `darwinHookReasonNone`.
- `traversal wrapper` and `symlinked wrapper identity`: preserve `unavailable` and use `identity-mismatch`.

Run:

```bash
set +e
/tmp/den-darwin-reason-pure-tests \
  '^(TestDarwinHookEnvironmentSelectionReasons|TestDarwinHookClaudePIDRequiresExactBoundPaths|TestDarwinHookClaudePIDFromBindingRejectsWrapperEscapes)$'
red_status=$?
set -e
test "$red_status" -ne 0
```

Expected RED: the selector signatures still return bare status strings or a literal reason assertion fails. Confirm the failure names the unimplemented reason mapping.

- [ ] **Step 6: GREEN — thread identity and PID reasons through the existing selectors**

Add these two pure boundary helpers and use them in the named production paths:

```go
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
```

In `dumpDarwinHookProcesses`:

1. Initialize fallback environment evidence as `unavailable` plus `pid-unavailable`.
2. Replace the direct pre-selector context check with `darwinHookPreselectionFailure` and assign its snapshot before returning.
3. Receive a reason-bearing selection snapshot from `darwinHookClaudeProcess`.
4. Preserve the current behavior: start capture only when selection status is `available`; otherwise assign the selection snapshot unchanged.

Change only the third return value of these signatures from `string` to `darwinHookEnvironmentSnapshot`:

```go
func darwinHookClaudeProcess(diagnosticContext context.Context, output string, processGroup int) (string, string, darwinHookEnvironmentSnapshot)
func darwinHookClaudePIDFromBinding(diagnosticContext context.Context, output string, processGroup int, manifest []byte, expectedFencePath string, read func(context.Context, string) ([]byte, bool, error), resolve func(string) (string, error)) (string, string, darwinHookEnvironmentSnapshot)
func darwinHookClaudePID(processes []darwinHookProcess, expectedFence, expectedClaude string) (string, string, darwinHookEnvironmentSnapshot)
```

Apply this exact first-applicable mapping without adding, deleting, merging, or reordering existing status branches:

| Existing branch | Snapshot returned |
| --- | --- |
| Manifest read error or truncation | `{status: "unavailable", reason: darwinHookReasonIdentityMismatch}` |
| Manifest binding status not `available` | same existing status plus `darwinHookReasonIdentityMismatch` |
| Zero combined Fence matches | `{status: "unavailable", reason: darwinHookReasonPIDUnavailable}` |
| Multiple combined Fence matches | `{status: "conflicting", reason: darwinHookReasonPIDUnavailable}` |
| Fence wrapper status not `available` | same existing status plus `darwinHookReasonIdentityMismatch` |
| Wrapper differs from manifest binding | `{status: "unavailable", reason: darwinHookReasonIdentityMismatch}` |
| Real-Claude identity status not `available` | same existing status plus `darwinHookReasonIdentityMismatch` |
| Invalid expected Fence or Claude identity passed to `darwinHookClaudePID` | `{status: "unavailable", reason: darwinHookReasonIdentityMismatch}` |
| Zero Fence matches inside `darwinHookClaudePID` | `{status: "unavailable", reason: darwinHookReasonPIDUnavailable}` |
| Multiple Fence matches inside `darwinHookClaudePID` | `{status: "conflicting", reason: darwinHookReasonPIDUnavailable}` |
| Zero Claude children | `{status: "unavailable", reason: darwinHookReasonPIDUnavailable}` |
| Multiple Claude children | `{status: "conflicting", reason: darwinHookReasonPIDUnavailable}` |
| Exactly one proven Claude child | `{status: "available"}` with zero-value `darwinHookReasonNone` |

Do not change `darwinHookBindingFromContents`, `darwinHookFenceWrapper`, `darwinHookClaudePathFromWrapper`, `darwinHookFenceProcess`, or their status rules. The reason is attached by their caller at the evidence boundary.

Run:

```bash
gofmt -w tests/native/acl_darwin_test.go
/tmp/den-darwin-reason-pure-tests \
  '^(TestDarwinHookEnvironmentSelectionReasons|TestDarwinHookClaudePIDRequiresExactBoundPaths|TestDarwinHookClaudePIDFromBindingRejectsWrapperEscapes|TestDarwinHookBindingRejectsEscapes|TestDarwinHookCanonicalNixStorePathRejectsEscapes)$'
```

Expected GREEN: every selector test passes with the original status and exact reason. Mutating an identity reason to `pid-unavailable`, a PID reason to `identity-mismatch`, or a `conflicting` status to `unavailable` must fail a literal table row.

- [ ] **Step 7: RED — add capture, timeout, cap-boundary, and parse reason tests**

Add `TestDarwinHookEnvironmentCaptureReasons` around the real `lockedBuffer`, `classifyDarwinHookEnvironmentCapture`, `parseDarwinHookEnvironment`, and `waitForDarwinHookEnvironment` behavior.

Use these literal classifier cases:

```go
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
```

For the command-error and error-plus-overflow rows, also pass the returned snapshot through `darwinHookEnvironmentSummary` and assert the complete result is exactly `snapshot=unavailable reason=capture-failed`. This proves that the raw error, command, path, value, count, credential, token, URL, host name, and arbitrary host data cannot reach reason output.

Add two real `lockedBuffer` subtests:

1. Write exactly `darwinHookObservationLimit` bytes, take a snapshot, assert `truncated == false`, and pass the result through `classifyDarwinHookEnvironmentCapture`. Use `command := strings.Repeat("c", darwinHookObservationLimit)` and the same bytes as output; expect `available` with no reason.
2. Write `darwinHookObservationLimit+1` bytes, take a snapshot, assert `truncated == true`, and classify it; expect `truncated` with `capture-failed`.

Add a caller-wait deadline subtest:

```go
diagnosticContext, cancel := context.WithCancel(context.Background())
cancel()
done := make(chan darwinHookEnvironmentSnapshot)
evidence := waitForDarwinHookEnvironment(diagnosticContext, darwinHookProcessEvidence{}, done)
requireDarwinHookEnvironmentResult(t, evidence.environment, "unavailable", darwinHookReasonCaptureFailed)
```

Extend `TestParseDarwinHookEnvironmentRejectsAmbiguity` so every existing conflicting case also asserts `darwinHookReasonParseAmbiguous`, while the proven empty environment asserts `darwinHookReasonNone`.

Run:

```bash
set +e
/tmp/den-darwin-reason-pure-tests \
  '^(TestDarwinHookEnvironmentCaptureReasons|TestParseDarwinHookEnvironmentRejectsAmbiguity)$'
red_status=$?
set -e
test "$red_status" -ne 0
```

Expected RED: capture and parse branches lack at least one required reason, and the wait-deadline branch lacks `capture-failed`.

- [ ] **Step 8: GREEN — complete capture precedence, parse reasons, and wait-timeout mapping**

Make `captureDarwinHookEnvironment` delegate its existing outputs to the pure classifier without changing the command, arguments, writer, deadline, wait delay, or cap:

```go
func captureDarwinHookEnvironment(diagnosticContext context.Context, pid, command string) darwinHookEnvironmentSnapshot {
	output := &lockedBuffer{limit: darwinHookObservationLimit}
	ps := exec.CommandContext(diagnosticContext, "/bin/ps", "eww", "-p", pid, "-o", "command=")
	ps.WaitDelay = 2 * time.Second
	ps.Stdout, ps.Stderr = output, output
	err := ps.Run()
	contents, truncated, available := output.Snapshot(diagnosticContext, darwinHookObservationLimit)
	return classifyDarwinHookEnvironmentCapture(command, contents, truncated, available, err)
}
```

Add the pure classifier with this exact precedence:

```go
func classifyDarwinHookEnvironmentCapture(command, contents string, truncated, available bool, err error) darwinHookEnvironmentSnapshot {
	if !available || err != nil {
		return darwinHookEnvironmentSnapshot{status: "unavailable", reason: darwinHookReasonCaptureFailed}
	}
	if truncated {
		return darwinHookEnvironmentSnapshot{status: "truncated", reason: darwinHookReasonCaptureFailed}
	}
	return parseDarwinHookEnvironment(command, contents)
}
```

This order makes an unavailable snapshot or command error win over overflow. Only a readable, successful, non-truncated capture reaches `parseDarwinHookEnvironment`.

In `parseDarwinHookEnvironment`, preserve every existing status branch and add only:

- `darwinHookReasonParseAmbiguous` to all `conflicting` returns.
- `darwinHookReasonNone` implicitly to the `available` return.

In `waitForDarwinHookEnvironment`, change only the deadline branch to assign the complete fixed snapshot:

```go
evidence.environment = darwinHookEnvironmentSnapshot{status: "unavailable", reason: darwinHookReasonCaptureFailed}
```

Run:

```bash
gofmt -w tests/native/acl_darwin_test.go
/tmp/den-darwin-reason-pure-tests \
  '^(TestDarwinHookEnvironmentCaptureReasons|TestParseDarwinHookEnvironmentRejectsAmbiguity|TestDarwinHookEnvironmentReasonOutputContract|TestDarwinHookEnvironmentReasonOutputSanitizesData)$'
```

Expected GREEN: all capture, cap, parse, output, and sanitization tests pass. Error-plus-overflow must stay `unavailable`; exactly 64 KiB must not report truncation.

- [ ] **Step 9: Run the complete focused pure reason suite**

Run all new and directly affected pure tests together:

```bash
/tmp/den-darwin-reason-pure-tests \
  '^(TestDarwinHookEnvironmentReasonOutputContract|TestDarwinHookEnvironmentReasonOutputSanitizesData|TestDarwinHookEnvironmentSelectionReasons|TestDarwinHookEnvironmentCaptureReasons|TestDarwinHookEnvironmentSummarySanitizesValues|TestParseDarwinHookEnvironmentRejectsAmbiguity|TestDarwinHookClaudePIDRequiresExactBoundPaths|TestDarwinHookClaudePIDFromBindingRejectsWrapperEscapes|TestDarwinHookBindingRejectsEscapes|TestDarwinHookCanonicalNixStorePathRejectsEscapes)$'
```

Expected: `ok command-line-arguments` with no test failures. This proves pure mapping and output behavior on the local host; it does not prove Darwin process capture at runtime.

- [ ] **Step 10: Inspect the one-file diff against the approved invariants**

Run:

```bash
set -euo pipefail
test "$(git diff --name-only)" = "tests/native/acl_darwin_test.go"
git diff --check
git diff --stat
git diff -U20 -- tests/native/acl_darwin_test.go
git diff --cached --quiet
```

Inspect the complete diff. Confirm all of these directly:

- The 45-second observation timer, 5-second evidence timeout, 60-second absolute deadline, 64 KiB limit, 2/3-second command timeouts, and wait delays are unchanged.
- The `ps eww` command and exact process identity chain are unchanged.
- The combined Fence predicate still requires the exact executable and nonempty `--settings` argument.
- Capture still starts only after exactly one proven real Claude PID and still runs concurrently with sampling.
- Available summaries retain the existing allowlist and sanitizers.
- Non-available summaries return after one fixed status/reason line.
- Sampling, lsof, policy evidence, streams, assertions, diagnostic ordering, TERM/KILL cleanup, drain, and dead-process confirmation are unchanged.
- No raw error, command, path, value, count, credential, token, URL, host name, or arbitrary host data is stored in or appended to the reason field.

Stop if the diff contains any behavior outside reason metadata and its focused pure tests.

- [ ] **Step 11: Run Darwin compilation and established cross-platform verification**

Run:

```bash
set -euo pipefail
gofmt -w tests/native/acl_darwin_test.go
GOOS=darwin GOARCH=arm64 go test -c -tags native -o /dev/null ./tests/native/
GOOS=darwin GOARCH=amd64 go test -c -tags native -o /dev/null ./tests/native/
go vet -tags native ./tests/native/
nix build .#checks.x86_64-linux.native-enforcement --no-link --print-build-logs
nix build .#checks.x86_64-linux.pure-launcher --no-link --print-build-logs
nix eval --raw .#checks.x86_64-darwin.native-enforcement.drvPath
printf '\n'
nix eval --raw .#checks.aarch64-darwin.native-enforcement.drvPath
printf '\n'
nix flake check --no-build
git diff --check
test "$(git diff --name-only)" = "tests/native/acl_darwin_test.go"
git diff --cached --quiet
test "$(git status --short)" = " M tests/native/acl_darwin_test.go"
```

Expected: every command exits 0. Pre-existing Nix evaluation warnings may be reported, but no new warning may be treated as passing evidence without investigation. Darwin cross-compilation proves type correctness only; do not claim Darwin runtime success.

- [ ] **Step 12: Remove the temporary runner and commit only the reviewed implementation file**

First remove the untracked helper and recheck scope:

```bash
rm -f /tmp/den-darwin-reason-pure-tests
test "$(git diff --name-only)" = "tests/native/acl_darwin_test.go"
git diff --check
git diff --cached --quiet
```

Read and follow the `commit` skill. Then stage and commit only the approved file:

```bash
git add tests/native/acl_darwin_test.go
git diff --cached --check
test "$(git diff --cached --name-only)" = "tests/native/acl_darwin_test.go"
git commit -m "test(darwin): classify snapshot failures"
```

Expected: one local commit contains only `tests/native/acl_darwin_test.go`. Do not push.

## Review Gate

After Task 1 commits, the SDD controller must:

1. Generate a review package from the implementation base through the new head.
2. Dispatch a fresh `reviewer` with fresh context, the approved specification, this task brief, the Task 4p implementation and CI reports, the complete one-file diff, and verification evidence.
3. Require both specification compliance and code-quality approval with no Critical or Important finding.
4. Route accepted findings through the SDD fix/re-review loop and rerun every affected focused and cross-platform check.
5. Run the final whole-branch review required by `subagent-driven-development`.

The reviewer must verify exact reason precedence, status preservation, fixed-only output, identity-before-capture, concurrency, the 64 KiB boundary, timing, assertions, and cleanup. Review must not infer a Darwin cause or propose a repair from the reason metadata.

## Execution Stop

After the local commit and clean reviews:

- Report the local commit, changed-file scope, RED/GREEN evidence, verification commands, and the limitation that Darwin runtime evidence still requires a new exact-head CI run.
- Do not push and do not rerun either existing workflow.
- Request separate explicit approval before a normal push.
- After any later approved push, inspect only the new exact-head runs. Report the fixed status and reason emitted by each Darwin job, preserve the no-rerun rule, and stop without a repair conclusion unless a separate design is approved.
