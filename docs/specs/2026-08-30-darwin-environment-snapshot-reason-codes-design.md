# Darwin Environment Snapshot Reason Codes Design

## Status

This is an approved design specification.

This specification authorizes no implementation, repair, push, CI action, workflow rerun, commit, staging, merge, PR close, or other PR action.

A later approved task can implement this design. That task can change only `tests/native/acl_darwin_test.go`.

## Context

Task 4p added a Darwin-only diagnostic for the effective environment of the proven real Claude process.

The diagnostic emits `DIAG-HOOK: Claude environment snapshot=<status>`.

Exact-head CI produced `snapshot=unavailable` in all four Darwin jobs. The status alone does not identify the failed evidence stage.

The diagnostic must remain fail-closed. An unavailable or conflicting snapshot must not authorize a repair conclusion.

## Evidence Boundary

The snapshot observes only the already-running real Claude process in the diagnostic process group.

The identity chain is: manifest, canonical Fence executable, Fence command, Claude wrapper, and real Claude executable.

The diagnostic must prove every link before it captures a process environment.

The capture uses bounded host-side process evidence. It must not read an unrelated process, a non-regular file, or an unbounded output stream.

The diagnostic prints no raw environment capture. It prints no environment value, error text, command, path, count, credential, token, URL, host name, or arbitrary host data in a reason diagnostic.

## Goals

- Keep the existing snapshot status vocabulary: `available`, `unavailable`, `truncated`, and `conflicting`.
- Add one fixed, sanitized reason field for every non-available snapshot result.
- Make the unavailable evidence stage clear without exposing sensitive data.
- Keep the current fail-closed evidence behavior.
- Add focused pure tests for the reason mapping and output sanitization during implementation.

## Non-Goals

This design does not diagnose or repair the Darwin Claude startup stall.

This design does not change production code, Nix code, launch inputs, arguments, environment, policy, profiles, timeouts, assertions, Linux behavior, workflows, or repair scope.

This design does not change proxy, locale, CA, Fence, Seatbelt, macOS-service, or Claude behavior.

This design does not make an unavailable or conflicting result pass an assertion or produce a causal conclusion.

## Scope

A later implementation can modify only `tests/native/acl_darwin_test.go`.

It can add pure tests in that file for each reason mapping and for reason-output sanitization.

It must not add another observation stage. It must not add retries, host-path allowances, writable trust copies, skipped assertions, or enforcement exceptions.

## Data Model

The snapshot result has these fields:

| Field | Type | Rules |
| --- | --- | --- |
| `status` | fixed status code | One of `available`, `unavailable`, `truncated`, or `conflicting`. |
| `reason` | optional fixed reason code | Empty for `available`. Required for every other status. |

The allowed reason codes are:

- `identity-mismatch`
- `pid-unavailable`
- `capture-failed`
- `parse-ambiguous`

No other reason code is valid. The implementation must not derive a reason from an error string or host data.

## Exact Reason Mapping

Apply the mappings in this order. The first applicable condition determines the result.

| Evidence condition | Status | Reason |
| --- | --- | --- |
| The existing pre-selector return sees an expired deadline after process listing and before manifest reading. | `unavailable` | `pid-unavailable` |
| An existing manifest-binding branch returns `unavailable`. | `unavailable` | `identity-mismatch` |
| The existing combined Fence predicate finds no matching process after manifest binding. | `unavailable` | `pid-unavailable` |
| The existing combined Fence predicate finds multiple matching processes after manifest binding. | `conflicting` | `pid-unavailable` |
| A wrapper or real-Claude identity branch returns `unavailable` after one Fence match. | `unavailable` | `identity-mismatch` |
| A wrapper or real-Claude identity branch returns `conflicting` after one Fence match. | `conflicting` | `identity-mismatch` |
| No matching live Claude PID exists after the Fence-command, wrapper, and Claude-identity proof. | `unavailable` | `pid-unavailable` |
| Multiple matching live Claude PIDs exist after the Fence-command, wrapper, and Claude-identity proof. | `conflicting` | `pid-unavailable` |
| The capture snapshot is unavailable, or the capture command returns an error. | `unavailable` | `capture-failed` |
| The diagnostic deadline expires while the caller waits for the started capture. | `unavailable` | `capture-failed` |
| The capture snapshot is available, the command has no error, and the bounded writer reports overflow. | `truncated` | `capture-failed` |
| The bounded capture returns empty output, or command or environment boundaries cannot be proven. | `conflicting` | `parse-ambiguous` |
| The identity chain, PID, capture, and parse are proven. | `available` | no reason field |

The implementation must add reason metadata without changing any existing status branch.

`identity-mismatch` includes an invalid, missing, non-canonical, or disagreeing manifest binding. The current manifest-binding helper has no `conflicting` branch.

After one Fence match, `identity-mismatch` also covers an invalid, missing, disagreeing, or ambiguous wrapper or real-Claude identity link. An ambiguous Fence wrapper or wrapper `exec` line keeps its existing `conflicting` status.

`pid-unavailable` applies when the existing pre-selector return sees an expired deadline. This return occurs after process listing and before manifest reading.

It also applies when the existing process selector does not produce one matching Fence or Claude PID. A deadline during manifest reading remains `identity-mismatch`.

The existing combined Fence predicate requires the exact expected Fence executable and a nonempty `--settings` argument. This design does not split that predicate or add another process probe.

Zero matching live PIDs keep the existing `unavailable` status. Multiple matching live PIDs keep the existing `conflicting` status.

`capture-failed` applies only after exactly one proven live Claude PID exists. It includes command failure, context expiration, unsafe file or stream handling, and unreadable capture output.

Command errors and unavailable snapshots take precedence over a writer overflow. The writer reports overflow only when output exceeds its remaining capacity. Complete output of exactly 64 KiB does not overflow.

`parse-ambiguous` applies after the bounded capture returns a readable result. It includes empty output, multiple lines, an unproven command prefix, or a missing command delimiter.

It also covers any nonempty environment text because the current whitespace format cannot prove entry boundaries.

## Data Flow

1. If the existing pre-selector return sees an expired deadline, emit `unavailable` with `pid-unavailable`.
2. Read the existing bounded manifest evidence and establish the manifest binding.
3. If manifest binding fails, preserve its existing `unavailable` status with `identity-mismatch`.
4. Apply the existing combined Fence predicate to the existing process evidence.
5. If no Fence process matches, emit `unavailable` with `pid-unavailable`.
6. If multiple Fence processes match, preserve `conflicting` with `pid-unavailable`.
7. Prove the selected Fence wrapper and the real Claude identity.
8. If this proof fails, preserve its existing status with `identity-mismatch`.
9. Parse existing process evidence for exactly one matching live Claude PID.
10. If no Claude PID matches, emit `unavailable` with `pid-unavailable`.
11. If multiple Claude PIDs match, preserve `conflicting` with `pid-unavailable`.
12. Start the bounded host-side environment capture for that PID within the existing concurrent observation work.
13. If the snapshot is unavailable or the command returns an error, emit `unavailable` with `capture-failed`.
14. If the bounded writer reports overflow, emit `truncated` with `capture-failed`.
15. Parse bounded capture output only when the capture succeeds without truncation.
16. If the output is empty or its boundaries are not proven, emit `conflicting` with `parse-ambiguous`.
17. If parsing is proven, emit `available` and the existing sanitized environment summaries.

The implementation must preserve the current allowlist and sanitizers. It must not infer a value from an ambiguous representation.

## Output Contract

Each snapshot emits one status summary.

For a non-available status, emit exactly one fixed reason field on the same line:

```text
DIAG-HOOK: Claude environment snapshot=unavailable reason=identity-mismatch
```

The only valid reason values are the four codes in this specification.

Available snapshots omit the reason field:

```text
DIAG-HOOK: Claude environment snapshot=available
```

The reason diagnostic must not include raw errors, commands, paths, values, counts, credentials, tokens, URLs, host names, or arbitrary host data.

Existing sanitized environment summaries can follow an available status. A non-available status must not cause value-derived summaries or listener conclusions.

## Error Handling

The diagnostic must classify errors at the earliest proven evidence boundary.

It must not replace a failed manifest binding with `pid-unavailable`, `capture-failed`, or `parse-ambiguous`.

It must not replace a failed combined Fence selection with `identity-mismatch`, `capture-failed`, or `parse-ambiguous`.

It must not replace a failed wrapper or real-Claude identity proof with `pid-unavailable`, `capture-failed`, or `parse-ambiguous`.

It must not replace a failed Claude PID selection with `capture-failed` or `parse-ambiguous`.

It must preserve the status of each existing identity and PID-selection branch. An identity ambiguity remains `conflicting`. Multiple matching PIDs remain `conflicting`.

An unavailable snapshot or command error takes precedence over writer overflow. Otherwise, writer overflow produces `truncated`. Complete output at exactly 64 KiB continues to parsing.

It must preserve `conflicting` when bounded output is empty or has unproven boundaries.

A timeout, command error, unsafe read, or unreadable output after PID selection maps to `capture-failed` without error text.

A reason code is diagnostic metadata only. It must not change test success, assertion behavior, cleanup, or enforcement.

## Security

The snapshot remains diagnostic-only and fail-closed.

The reason field contains a fixed code only. It cannot contain formatted errors, process output, or parsed values.

The identity proof remains required before any host-side environment capture. This rule prevents unrelated process inspection.

The capture remains bounded while bytes are read. Post-capture truncation is not sufficient.

An ambiguous environment representation remains conflicting. The implementation must not split whitespace or infer values when entry boundaries are not proven.

## Preserved Budgets and Behavior

The implementation must preserve all of these limits and behavior:

- The 45-second observation point.
- The 5-second evidence budget.
- The 60-second absolute deadline.
- The 64 KiB capture cap.
- Existing derived command timeouts and output caps.
- Existing process output, sampling, listener capture, stream output, assertions, and diagnostic ordering.
- Existing TERM and KILL cleanup, drain, and dead-process confirmation.

The implementation must not relax a timeout or extend an observation budget. It must not rerun a workflow.

## Testing Value Gate

Later implementation must add focused pure tests in `tests/native/acl_darwin_test.go`.

The tests must cover each fixed mapping:

- The existing pre-selector deadline return maps to `unavailable` and `pid-unavailable`.
- A deadline during manifest reading maps to `unavailable` and `identity-mismatch`.
- An existing unavailable manifest-binding branch maps to `unavailable` and `identity-mismatch`.
- No match from the combined Fence predicate maps to `unavailable` and `pid-unavailable`.
- Multiple matches from the combined Fence predicate map to `conflicting` and `pid-unavailable`.
- An existing unavailable wrapper or Claude-identity branch maps to `unavailable` and `identity-mismatch`.
- An existing conflicting wrapper or Claude-identity branch maps to `conflicting` and `identity-mismatch`.
- A missing Claude PID after the identity proof maps to `unavailable` and `pid-unavailable`.
- Multiple Claude PIDs after the identity proof map to `conflicting` and `pid-unavailable`.
- An unavailable snapshot or command error maps to `unavailable` and `capture-failed`.
- A diagnostic deadline while waiting for the started capture maps to `unavailable` and `capture-failed`.
- A writer overflow without a command error maps to `truncated` and `capture-failed`.
- Complete capture output at exactly 64 KiB continues to parsing.
- A command error and writer overflow together map to `unavailable` and `capture-failed`.
- Empty bounded capture output maps to `conflicting` and `parse-ambiguous`.
- Ambiguous bounded bytes map to `conflicting` and `parse-ambiguous`.
- A proven available snapshot has no reason.

The tests must also prove output sanitization. They must show that reason output cannot disclose a raw error, command, path, value, count, credential, token, URL, host name, or arbitrary host data.

Do not add a test that only matches source text or documentation wording.

## Verification

A later implementation must run focused Go formatting and Darwin cross-compilation for `tests/native/acl_darwin_test.go`.

It must run the applicable native package vet command and unchanged Linux gates.

It must evaluate both Darwin native-enforcement derivation paths and run `nix flake check --no-build`.

It must inspect `git diff --check`, the changed-file list, the index, and `git status`.

Darwin runtime behavior requires exact-head Darwin CI. The later task must report that limit honestly and must not rerun a workflow.

## Acceptance Criteria

The later implementation is acceptable only when all conditions are true:

- Only `tests/native/acl_darwin_test.go` changes.
- The four existing status codes remain unchanged.
- Every non-available snapshot has exactly one fixed reason code.
- Available snapshots omit the reason field.
- The exact mappings in this specification are used.
- Reason output contains no raw or arbitrary diagnostic data.
- The identity proof remains before environment capture.
- Capture remains concurrent, bounded, and fail-closed.
- The retained observation, evidence, deadline, cap, sampling, assertions, and cleanup behavior remain unchanged.
- Focused pure mapping and sanitization tests pass where the Darwin runtime can execute them.
- No production, Nix, launch, environment, policy, profile, timeout, assertion, Linux, workflow, or repair change occurs.

## Stop Conditions

Stop and request a new design decision before implementation if a result needs a new status, reason code, or raw diagnostic data.

Stop if correct classification needs another capture source or a changed launch input.

Stop if process identity cannot be proven without reading a broader host path or an unrelated process.

Stop if correct classification requires a larger cap, a longer budget, a retry, a timeout change, a new assertion, or weaker enforcement.

Stop after new CI evidence. Report the fixed status and reason only. Do not infer a repair without a separate approved design.

## Deferred Version Experiment

Record `github:numtide/llm-agents.nix` as a separate deferred version experiment.

Current Den uses `pkgs.claude-code` version `2.1.158` from pinned nixpkgs.

A read-only check found Claude Code `2.1.251` in `llm-agents.nix` for `aarch64-darwin` and Linux.

That check found no `x86_64-darwin` package output.

This experiment requires a separate Nix and launch design with approval. It is not a fix, dependency update, package change, or fallback for this design.
