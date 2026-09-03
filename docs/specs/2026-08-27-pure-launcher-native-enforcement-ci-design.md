# Pure Launcher and Native Enforcement CI Design

## Status

The user validated this design on 2026-08-27.

## Goal

Make PR #1 pass all four native CI jobs without weaker sandbox enforcement.

Use separate evidence and repair gates for the deterministic Darwin error and the intermittent Linux error.

## Base State

The worktree is `/home/roche/projects/den/.worktrees/den-claude-sandbox-design`.

The branch is `feat/den-claude-sandbox`.

The local, remote, and PR head is `a6da55429b39b263b1092dcb25238d175dd74b49`.

The worktree and index were clean when this design started.

PR #1 is open and is not merged.

## CI Evidence

The applicable CI runs are:

- Push run `32964703080`
- Pull request run `32964708559`

The push run had these results:

- `aarch64-linux` passed.
- `x86_64-linux` failed `TestFilesystemEnforcement/policy_is_immutable`.
- Both Darwin jobs failed `pure-launcher` after all logged Go tests passed.

The pull request run had these results:

- Both Linux jobs passed.
- Both Darwin jobs failed the same `pure-launcher` derivation.

No workflow was rerun after these results.

The socket-path and portable-`script` commits did not change either new failing check. Those commits let CI reach these later checks.

## Failure Classification

### Darwin

The Darwin `pure-launcher` error is deterministic on both architectures.

`nix/check-support/pure-launcher-darwin.nix` uses silent shell assertions with `set -eu`. The current log does not identify the failed command.

A Darwin-native diagnostic push is necessary before a repair design can identify the root cause.

### Linux

The Linux `native-enforcement` error is intermittent.

Twenty local runs of the packaged runner produced one error. The failed test moved to `TestFilesystemEnforcement/custom_state_denies_defaults`.

Fence 0.1.58 reported these errors:

- `multithreaded exec cannot be safely continued in argv mode`
- `Exec failed: operation not permitted`

The evidence points to Fence bootstrap state. Fence exhausts its recognized multithreaded bootstrap continues before the staged shell starts the fixture agent.

The test names are not the root cause. Their movement shows that a shared launch boundary fails intermittently.

## Global Constraints

- Keep the Go floor at `go 1.24.0`.
- Keep all four CI platforms.
- Keep sandbox enforcement fail-closed.
- Do not add host paths.
- Do not skip checks or assertions.
- Do not weaken Linux production behavior.
- Do not increase the Fence bootstrap allowance.
- Do not add retries that hide an unsafe process state.
- Do not amend or force-push.
- Do not rerun workflows.
- Do not merge or close PR #1.
- Request fresh independent review before each commit and push.
- Request user approval before each push.

## Documentation Gate

Complete the documentation gate before implementation starts.

1. Write and self-review this design spec.
2. Request fresh independent review of the design spec.
3. Apply accepted findings.
4. If a finding changes the diff, repeat fresh review until the final design diff has no required corrections.
5. Commit the final reviewed design spec.
6. Request user approval of the committed design spec.
7. Use `writing-plans` to write the implementation plan.
8. Self-review the implementation plan.
9. Request fresh independent review of the implementation plan.
10. Apply accepted findings.
11. If a finding changes the diff, repeat fresh review until the final plan diff has no required corrections.
12. Commit the final reviewed implementation plan.
13. Request user approval of the committed implementation plan.
14. Request user approval for one documentation-only push.
15. Push both documentation commits normally.

Do not combine a documentation commit with an implementation commit.

## Delivery Sequence

After the documentation push, use three normal implementation pushes with separate review and CI evidence.

1. Add Darwin diagnostics only.
2. Repair the Darwin root cause after the new logs identify it.
3. Repair the Linux Fence error after a separate security review.

The documentation push makes four future normal pushes in total. Do not combine platform repairs in one commit or push.

For each implementation gate, use this sequence:

1. Make the approved change.
2. Review the change in the parent session.
3. Request a fresh independent review of the uncommitted diff.
4. Apply accepted review findings.
5. If a finding changes the diff, repeat fresh review until the final diff has no required corrections.
6. Run the applicable checks.
7. Commit the reviewed diff.
8. Request user approval for the push.
9. Push normally.
10. Inspect only the new matching push and pull request runs.

## Gate 1: Darwin Diagnostics

### Scope

Change only `nix/check-support/pure-launcher-darwin.nix`.

Do not change the assertions, their order, the fakes, launcher behavior, dependencies, workflow, or Linux checks.

### Phase Output

Write one concise marker to standard error before each existing phase:

1. Source setup and Go tests
2. Git and credential fixture setup
3. Normal launcher execution
4. Success-path evidence assertions
5. Early-rejection cases
6. Simple mode
7. PTY execution
8. Cleanup and completion

### Error Output

Use one error reporter for ordinary command errors and explicit assertion exits.

The reporter must write these fields:

- Current phase
- Source line
- Exit status
- Failing shell command

The reporter must preserve the original nonzero status.

Do not enable full shell tracing. Do not print the environment, fixture logs, credentials, or token values.

Keep the phase and error output after the Darwin repair. This output makes later check errors actionable.

### Pre-push Diagnostic Proof

Use disposable local fault injection before the diagnostic commit. Do not include a fault in the final diff.

Cover these error paths:

- An ordinary command error inside a function or subshell
- An explicit assertion exit

For each path, make sure that the report contains the phase, command, source line, and original exit status.

Use redacted fixture values during this proof. Make sure that the report contains no environment dump, credential, token, or fixture-log content.

Restore the final check file after the proof. Then compare the final diff with the approved diagnostic scope.

The implementation plan must give the exact fault-injection and restoration commands.

### Diagnostic Acceptance

The first push is successful as a diagnostic step when the new Darwin logs identify the exact failed phase, command, line, and status.

The first push does not need to make CI green.

A Linux error does not invalidate the Darwin evidence because the matrix jobs run independently with `fail-fast: false`.

## Gate 2: Darwin Repair

Compare both Darwin architectures and both workflow triggers after the diagnostic push.

Proceed only when all four Darwin jobs report every required field and identify the same command or assertion boundary.

Trace that boundary through its launcher, fake, or fixture path. State one root-cause hypothesis and the supporting evidence.

Then present the repair design for user approval before code changes.

Stop if a Darwin job passes, omits a required field, or identifies a different boundary. Treat every different result as separate evidence.

The repair must not add retries, skip assertions, expose host paths, or relax sandbox behavior.

Use the smallest platform-specific change that corrects the identified cause.

Add an automated regression test when the repair changes production or reusable behavior.

If the error is only in the Nix check harness, use direct Nix evaluation and build checks. Do not add tests that assert static Nix source text.

Before the Darwin repair push:

- Run the focused checks for the changed boundary.
- Run applicable Linux checks for cross-platform regressions.
- Request fresh independent review.
- Request user push approval.

Both Darwin CI architectures must pass after this push. The Linux Fence repair remains out of scope for this gate.

## Gate 3: Linux Fence Repair

### Existing Security Boundary

Fence 0.1.58 gives one multithreaded bootstrap continue without Landlock.

Fence gives two continues when it uses the Landlock wrapper.

Existing Fence tests require the third multithreaded bootstrap exec to fail. This limit must remain unchanged.

Fence must continue to deny these cases:

- An unrecognized multithreaded exec
- A multithreaded exec after the bootstrap budget is exhausted
- A process whose thread state cannot be read safely
- An exec candidate whose path or arguments cannot be inspected safely
- An exec that the configured runtime policy denies

### Root-Cause Evidence

Use local temporary instrumentation to record the failing exec sequence, process identity, bootstrap state, and thread count.

Do not commit or push this temporary instrumentation.

Put the focused regression test at the boundary that owns the error.

If Fence owns the error, add a Fence test that models the incorrect transition or state accounting.

If Den creates a redundant transition, add a Den test that models that launch behavior.

Demonstrate that the focused test fails before the repair. Run the Fence security tests and packaged-runner stress gate for either repair path.

Do not infer the repair only from the test name that happens to fail.

### Allowed Repair Targets

Use this order of preference:

1. Correct Fence bootstrap classification or state accounting without another allowed continue.
2. If Den creates a redundant bootstrap transition, remove that Den-owned transition.
3. If success requires broader authority, stop and request a new security design.

Broader authority includes these changes:

- A third multithreaded bootstrap continue
- A new recognized bootstrap executable
- A retry after an unsafe thread-state result
- A bypass of thread-state verification

### Fence Patch Integration

A Fence repair can modify `patches/fence-0.1.58-den-tmpdir.patch`.

If the patch changes, also update:

- The expected patch hash in `nix/lib/fence.nix`
- The focused `checkPhase` selection in `nix/lib/fence.nix`

The focused check must run the new regression test.

Keep Fence at version 0.1.58. Keep the current source-hash and upstream-version guards.

### Linux Verification

Use a red-green test cycle for the focused regression at the owner boundary.

Run the Fence security tests for either repair path. If the Fence patch changes, run the patched tests.

These tests must prove that:

- The budgets remain one and two.
- The third bootstrap continue remains denied.
- Non-bootstrap multithreaded exec remains denied.
- Thread-count errors remain denied.
- Exec-candidate inspection errors remain denied.
- Policy-denied execs remain denied.

Build the packaged `native-enforcement` runner.

Run the packaged runner successfully 100 consecutive times on local `x86_64-linux`. Run the tests serially without retries or ignored errors.

Any error invalidates the stress result and returns the work to investigation.

Retain the available raw output from the 20-run reproduction. Capture the command, exit counts, elapsed time, and output for the 100-run acceptance.

Do not commit logs that contain credentials, tokens, private paths, or other secret values.

Three measured baseline runs took 9.1 to 10.0 seconds each. One hundred serial runs need approximately 15 to 17 minutes.

The measured peak memory was 272 to 280 MiB. The cached Nix closure was 2.2 GiB and is shared by all runs.

Run the applicable local Linux checks after the stress run. Then request a fresh security-focused review.

Both Linux CI architectures must pass after the Linux push. Darwin CI must remain green.

## Testing Value Gate

Do not add tests for workflow YAML, static Nix values, dependency versions, patch hashes, or documentation text.

Use syntax checks, evaluation, direct builds, existing checks, and matching CI for those changes.

Add tests for production behavior, reusable logic, security decisions, and regressions that can recur.

## Stop Conditions

Stop and ask the user before code changes if new evidence requires a product, architecture, or security decision.

Stop after a failed push and report the exact new evidence. Do not rerun a workflow or make an unreviewed follow-up push.

Stop the Linux repair if it needs more bootstrap authority or weaker thread-state enforcement.

## Commit and Push Policy

The design spec gets its own documentation commit. Do not push that commit without user approval.

Each implementation gate gets a focused commit after fresh independent review.

Use three separately approved implementation pushes. Do not amend prior commits or combine the platform repairs.
