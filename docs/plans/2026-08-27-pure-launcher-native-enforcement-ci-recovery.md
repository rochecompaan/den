# Pure Launcher and Native Enforcement CI Recovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Recover PR #1 native CI through a diagnostic-only Darwin push, an evidence-based Darwin repair, and a separately justified Linux Fence repair without weakening sandbox enforcement.

**Architecture:** Publish the reviewed design and plan first. Add persistent, concise diagnostics only to the Darwin `pure-launcher` check, use the resulting four Darwin job logs to authorize the smallest platform-specific repair, and treat the intermittent Linux failure as a separate Fence-boundary investigation. Unknown repair code is deliberately not authorized before its required evidence and user design gate; the plan gives exact stop conditions and acceptance contracts for those later decisions.

**Tech Stack:** Bash, Nix and Nixpkgs, Go 1.24, Fence 0.1.58, GitHub Actions, `gh`, `git`

## Global Constraints

- Work only in `/home/roche/projects/den/.worktrees/den-claude-sandbox-design` on `feat/den-claude-sandbox`.
- Start the documentation gate from design commit `b935f42` and remote/PR head `a6da55429b39b263b1092dcb25238d175dd74b49`.
- Keep the Go floor at `go 1.24.0`.
- Keep all four CI platforms.
- Keep sandbox enforcement fail-closed.
- Do not add host paths.
- Do not skip checks or assertions.
- Do not weaken Linux production behavior.
- Keep the Fence multithreaded bootstrap allowance at one continue without Landlock and two with Landlock.
- Keep the third multithreaded bootstrap exec denied.
- Do not add retries that hide an unsafe process state.
- Do not amend or force-push.
- Do not rerun workflows.
- Do not merge or close PR #1.
- Request fresh independent review before each commit and push.
- If a review finding changes a diff, request another fresh independent review of the new final diff.
- Request user approval before each push.
- Use normal pushes only.
- Stop when new evidence needs a product, architecture, or security decision.

## Delivery Boundaries

This plan has four normal push gates:

1. The existing design commit plus this plan commit, with no implementation files.
2. Darwin diagnostics only.
3. The evidence-based Darwin repair only.
4. The separately justified Linux repair only.

Tasks 1 through 3 are fully executable from current evidence. Task 4 cannot authorize a Darwin repair until all four diagnostic Darwin jobs identify one common boundary. Task 5 cannot authorize a Linux repair until temporary instrumentation identifies the owner. Those are evidence gates, not missing implementation steps: stop rather than invent a repair.

---

### Task 1: Publish the documentation gate

**Files:**
- Existing: `docs/specs/2026-08-27-pure-launcher-native-enforcement-ci-design.md`
- Existing: `docs/plans/2026-08-27-pure-launcher-native-enforcement-ci-recovery.md`
- Modify: none

**Interfaces:**
- Consumes: approved design commit `b935f42` and the separately reviewed plan commit at `HEAD`.
- Produces: one documentation-only remote head that is the parent of every implementation commit.

- [ ] **Step 1: Verify the two-commit documentation range**

Run:

```bash
set -euo pipefail
cd /home/roche/projects/den/.worktrees/den-claude-sandbox-design
design=b935f42
plan=$(git rev-parse HEAD)
test "$(git branch --show-current)" = "feat/den-claude-sandbox"
test "$(git rev-parse "$plan^")" = "$(git rev-parse "$design")"
test "$(git diff-tree --no-commit-id --name-only -r "$design")" = \
  "docs/specs/2026-08-27-pure-launcher-native-enforcement-ci-design.md"
test "$(git diff-tree --no-commit-id --name-only -r "$plan")" = \
  "docs/plans/2026-08-27-pure-launcher-native-enforcement-ci-recovery.md"
git diff --quiet
git diff --cached --quiet
test -z "$(git status --porcelain)"
git diff --check a6da55429b39b263b1092dcb25238d175dd74b49..HEAD
```

Expected: Every command exits with status 0, and the worktree and index are clean.

- [ ] **Step 2: Verify that PR #1 is still open at the expected remote head**

Run:

```bash
set -euo pipefail
pr_json=$(mktemp)
trap 'rm -f "$pr_json"' EXIT
gh pr view 1 --repo rochecompaan/den \
  --json state,mergedAt,headRefName,headRefOid,url \
  > "$pr_json"
jq -e '
  .state == "OPEN" and
  .mergedAt == null and
  .headRefName == "feat/den-claude-sandbox" and
  .headRefOid == "a6da55429b39b263b1092dcb25238d175dd74b49"
' "$pr_json" >/dev/null
jq -r '"STATE=\(.state) MERGED=\(.mergedAt) BRANCH=\(.headRefName) HEAD=\(.headRefOid) URL=\(.url)"' \
  "$pr_json"
```

Expected: `STATE=OPEN`, `MERGED=null`, `BRANCH=feat/den-claude-sandbox`, and `HEAD=a6da55429b39b263b1092dcb25238d175dd74b49`.

Stop if any value differs.

- [ ] **Step 3: Obtain explicit approval for the committed plan**

Present the plan commit hash and path to the user. Do not combine this approval with push approval.

Expected: The user explicitly approves the committed plan.

- [ ] **Step 4: Obtain separate approval for the documentation-only push**

State that the push contains exactly the design and plan commits and will use a normal, non-force push.

Expected: The user explicitly approves this push.

- [ ] **Step 5: Guard and push the two documentation commits normally**

Run only after Step 4 approval:

```bash
set -euo pipefail
branch=feat/den-claude-sandbox
expected_remote=a6da55429b39b263b1092dcb25238d175dd74b49
test "$(git branch --show-current)" = "$branch"
git diff --quiet
git diff --cached --quiet
test -z "$(git status --porcelain)"
actual_remote=$(git ls-remote origin "refs/heads/$branch" | awk '{print $1}')
test "$actual_remote" = "$expected_remote"
git push origin "HEAD:refs/heads/$branch"
```

Expected: One normal push succeeds. Do not rerun or monitor an older workflow run as evidence for an implementation gate.

---

### Task 2: Add persistent Darwin phase and error diagnostics

**Files:**
- Modify: `nix/check-support/pure-launcher-darwin.nix:15-141`
- Test: disposable local fault injection against `nix/check-support/pure-launcher-darwin.nix`

**Interfaces:**
- Consumes: the existing Darwin check body and its eight ordered phases.
- Produces: `set_phase NAME`, one phase marker per phase, and one `pure-launcher failure:` record with `phase`, `line`, `status`, and shell-escaped `command` fields.
- Does not change: assertions, assertion order, fakes, launcher behavior, dependencies, workflow files, or Linux checks.

- [ ] **Step 1: Reconfirm the diagnostic-only scope**

Run:

```bash
set -euo pipefail
cd /home/roche/projects/den/.worktrees/den-claude-sandbox-design
docs_head=$(git rev-parse HEAD)
test "$(git branch --show-current)" = "feat/den-claude-sandbox"
test "$(git rev-parse origin/feat/den-claude-sandbox)" = "$docs_head"
git diff --quiet
git diff --cached --quiet
test -z "$(git status --porcelain)"
```

Expected: The local and remote branch heads equal the documentation head, and the worktree and index are clean.

- [ ] **Step 2: Record the existing deterministic RED evidence**

Record these run IDs without rerunning them:

```text
push:         32964703080
pull_request: 32964708559
x86_64-darwin: pure-launcher failed after the logged Go tests passed
aarch64-darwin: pure-launcher failed after the logged Go tests passed
```

Expected: The task report states that the existing logs do not identify the failed shell command.

- [ ] **Step 3: Replace the shell prelude with one nonzero-exit reporter**

Replace:

```nix
    set -eu
```

With this exact Bash prelude. The doubled single quotes before Bash parameter expansions are required Nix indented-string escapes.

```nix
    set -ETeu
    current_phase=initialization
    last_command=
    last_line=
    failure_command=
    failure_line=

    capture_command() {
      local line=''${BASH_LINENO[0]}
      if [[ $line == 1 || ''${FUNCNAME[1]:-} == report_failure ]]; then
        return 0
      fi
      last_line=$line
      last_command=$BASH_COMMAND
    }

    capture_failure() {
      local status=$1
      if [[ -z $failure_line ]]; then
        failure_line=$2
        failure_command=$3
      fi
      return "$status"
    }

    report_failure() {
      local status=$1
      trap - DEBUG ERR EXIT
      if (( status != 0 )); then
        local line=''${failure_line:-$last_line}
        local command=''${failure_command:-$last_command}
        printf 'pure-launcher failure: phase=%s line=%s status=%s command=%q\n' \
          "$current_phase" "$line" "$status" "$command" >&2
      fi
      exit "$status"
    }

    set_phase() {
      current_phase=$1
      printf 'pure-launcher phase: %s\n' "$current_phase" >&2
    }

    trap capture_command DEBUG
    trap 'capture_failure "$?" "$LINENO" "$BASH_COMMAND"' ERR
    trap 'report_failure "$?"' EXIT
```

`DEBUG` records source text and line number without printing trace output. `ERR` preserves the first ordinary command failure in functions. `EXIT` covers explicit assertion exits and calls the same reporter. `%q` keeps the command on one shell-escaped line and does not expand environment values.

- [ ] **Step 4: Add the eight phase markers without moving any existing command**

Insert these calls at the stated boundaries:

```nix
    set_phase "source-setup-and-go-tests"
```

Place it immediately before `export CGO_ENABLED=0`.

```nix
    set_phase "git-and-credential-fixture-setup"
```

Place it immediately before `cd "$root/worktree"`.

```nix
    set_phase "normal-launcher-execution"
```

Place it immediately before this first normal-launch call:

```nix
    run_sandbox "argument with spaces" "" --plugin-dir user-plugin --mcp-config user-mcp.json --strict-mcp-config
```

```nix
    set_phase "success-path-evidence-assertions"
```

Place it immediately before `test "$(grep -Fxc invoked "$root/fence.marker")" = 1`.

```nix
    set_phase "early-rejection-cases"
```

Place it immediately before the `expect_early_failure() {` definition.

```nix
    set_phase "simple-mode"
```

Place it immediately before `export CLAUDE_CODE_SIMPLE=1 DEN_FAKE_EXPECT_SIMPLE_SCRUB=1`.

```nix
    set_phase "pty-execution"
```

Place it immediately before `ptyRunner="$root/pty-runner"`.

```nix
    set_phase "cleanup-and-completion"
```

Place it immediately before `rm -rf "$root"`.

Expected: The old commands retain their original order and text.

- [ ] **Step 5: Parse and evaluate the final diagnostic expression**

Run:

```bash
set -euo pipefail
nix-instantiate --parse nix/check-support/pure-launcher-darwin.nix >/dev/null
nix eval --raw .#checks.x86_64-darwin.pure-launcher.drvPath
printf '\n'
nix eval --raw .#checks.aarch64-darwin.pure-launcher.drvPath
printf '\n'
```

Expected: Parsing and both Darwin evaluations succeed.

- [ ] **Step 6: Prove ordinary-command and explicit-exit reporting with disposable faults**

Run this from the worktree. It snapshots the final diagnostic file, injects one fault at a time before fixture credentials exist, builds a directly imported copy of the check for the local Nix system, checks the report, and restores the exact final file after each probe.

```bash
set -euo pipefail
check=nix/check-support/pure-launcher-darwin.nix
snapshot=$(mktemp)
cp "$check" "$snapshot"
snapshot_hash=$(sha256sum "$snapshot" | awk '{print $1}')
restore_check() {
  cp "$snapshot" "$check"
}
cleanup_probe() {
  restore_check
  rm -f "$snapshot"
}
trap cleanup_probe EXIT

inject_fault() {
  DEN_DIAGNOSTIC_PROBE=$1 python3 - "$check" <<'PY'
import os
from pathlib import Path
import sys

path = Path(sys.argv[1])
text = path.read_text()
anchor = '    set_phase "source-setup-and-go-tests"\n'
if text.count(anchor) != 1:
    raise SystemExit(f"expected one injection anchor, found {text.count(anchor)}")

kind = os.environ["DEN_DIAGNOSTIC_PROBE"]
if kind == "ordinary":
    fault = """    diagnostic_fault() {
      bash -c 'exit 23'
    }
    diagnostic_fault
"""
elif kind == "explicit":
    fault = """    if true; then
      exit 24
    fi
"""
else:
    raise SystemExit(f"unknown probe kind: {kind}")

path.write_text(text.replace(anchor, anchor + fault, 1))
PY
}

probe() {
  kind=$1
  expected_status=$2
  command_fragment=$3
  restore_check
  inject_fault "$kind"
  log=$(mktemp)
  if nix build --no-link --print-build-logs --impure --expr '
    let
      flake = builtins.getFlake (toString ./.);
      pkgs = import flake.inputs.nixpkgs { system = builtins.currentSystem; };
    in import ./nix/check-support/pure-launcher-darwin.nix {
      inputs = flake.inputs;
      inherit pkgs;
    }
  ' >"$log" 2>&1; then
    printf 'fault probe unexpectedly succeeded: %s\n' "$kind" >&2
    rm -f "$log"
    return 1
  fi

  record=$(python3 - "$log" <<'PY'
from pathlib import Path
import re
import sys

marker = "pure-launcher failure:"
payloads = {
    line[line.index(marker):].strip()
    for line in Path(sys.argv[1]).read_text(errors="replace").splitlines()
    if marker in line
}
if len(payloads) != 1:
    raise SystemExit(f"expected one unique failure payload, found {sorted(payloads)!r}")
record = payloads.pop()
if not re.fullmatch(
    r"pure-launcher failure: phase=\S+ line=[1-9][0-9]* "
    r"status=[1-9][0-9]* command=.+",
    record,
):
    raise SystemExit(f"incomplete failure payload: {record!r}")
print(record)
PY
  )
  grep -F 'phase=source-setup-and-go-tests' <<< "$record" >/dev/null
  grep -F "status=$expected_status" <<< "$record" >/dev/null
  grep -F 'command=' <<< "$record" | grep -F "$command_fragment" >/dev/null
  if grep -Eq 'REPOWOLF_TOKEN=|CLAUDE_CODE_OAUTH_TOKEN=|fixture-ca|agent\.log|fence\.log|repowolf\.log' "$log"; then
    printf 'fault report exposed forbidden fixture content: %s\n' "$log" >&2
    return 1
  fi
  rm -f "$log"

  restore_check
  test "$(sha256sum "$check" | awk '{print $1}')" = "$snapshot_hash"
}

probe ordinary 23 'bash'
probe explicit 24 'exit\ 24'
restore_check
test "$(sha256sum "$check" | awk '{print $1}')" = "$snapshot_hash"
trap - EXIT
rm -f "$snapshot"
```

Expected:

- Both Nix builds fail because of the injected fault.
- Each log has exactly one unique failure payload after stripping Nix live-log and error-tail prefixes; an identical transport replay is accepted, but distinct payloads fail the probe.
- The ordinary command record has status 23.
- The explicit assertion record has status 24.
- Both records have the named phase, a nonzero source line, and the failing command.
- No environment dump, token, credential, or fixture-log content appears.
- The final check file hash equals the pre-probe snapshot hash.

- [ ] **Step 7: Verify that the final diff is diagnostic-only**

Run:

```bash
set -euo pipefail
test "$(git diff --name-only)" = "nix/check-support/pure-launcher-darwin.nix"
git diff --check
git diff --word-diff=porcelain -- nix/check-support/pure-launcher-darwin.nix
```

Inspect the last command. Expected: Only the prelude and eight markers changed. No assertion, fixture, dependency, launcher command, or cleanup command changed.

- [ ] **Step 8: Request fresh independent review of the uncommitted diagnostic diff**

Give the reviewer:

- Design: `docs/specs/2026-08-27-pure-launcher-native-enforcement-ci-design.md`.
- Plan: this task.
- Base: the documentation head from Task 1.
- Head: the uncommitted one-file diff.
- The two fault-injection reports and final file hash check.
- The no-xtrace, no-environment-dump, no-behavior-change, no-Linux-change constraints.

Expected: No Critical or Important findings and no required correction.

If any accepted finding changes the file, repeat Steps 5 through 8 with a new fresh reviewer.

- [ ] **Step 9: Commit the final reviewed diagnostic diff**

Read and follow the `commit` skill, then run:

```bash
set -euo pipefail
git add nix/check-support/pure-launcher-darwin.nix
git diff --cached --check
test "$(git diff --cached --name-only)" = "nix/check-support/pure-launcher-darwin.nix"
git commit -m "ci: diagnose Darwin pure launcher failures"
```

Expected: One focused commit contains only the Darwin diagnostic file.

---

### Task 3: Push the Darwin diagnostic and classify its evidence

**Files:**
- Modify: none
- Evidence: the new matching push and pull-request workflow runs

**Interfaces:**
- Consumes: the reviewed diagnostic commit.
- Produces: four Darwin job outcomes across push and pull-request runs, each with either a complete diagnostic record or a stop condition.

- [ ] **Step 1: Obtain explicit approval for the diagnostic push**

Report the diagnostic commit hash, one-file diff, local proof, and retained diagnostics. State that this push can leave CI red and can encounter the known independent Linux flake.

Expected: The user explicitly approves this push.

- [ ] **Step 2: Guard and push the diagnostic commit normally**

Run only after approval:

```bash
set -euo pipefail
branch=feat/den-claude-sandbox
diagnostic_head=$(git rev-parse HEAD)
expected_remote=$(git rev-parse HEAD^)
test "$(git branch --show-current)" = "$branch"
git diff --quiet
git diff --cached --quiet
test -z "$(git status --porcelain)"
actual_remote=$(git ls-remote origin "refs/heads/$branch" | awk '{print $1}')
test "$actual_remote" = "$expected_remote"
git push origin "HEAD:refs/heads/$branch"
```

Expected: One normal push succeeds.

- [ ] **Step 3: Identify only the two runs for the exact diagnostic head**

Run:

```bash
set -euo pipefail
diagnostic_head=$(git rev-parse HEAD)
[[ $diagnostic_head =~ ^[0-9a-f]{40}$ ]]
evidence_dir="/tmp/den-darwin-diagnostic-$diagnostic_head"
umask 077
rm -rf -- "$evidence_dir"
mkdir -m 0700 "$evidence_dir"
deadline=$((SECONDS + 600))
while true; do
  candidate="$evidence_dir/runs.candidate.json"
  gh run list --repo rochecompaan/den --workflow checks.yml \
    --branch feat/den-claude-sandbox --limit 20 \
    --json databaseId,event,headSha,status,conclusion,url,createdAt \
    > "$candidate"
  push_count=$(jq --arg head "$diagnostic_head" \
    '[.[] | select(.headSha == $head and .event == "push")] | length' \
    "$candidate")
  pull_request_count=$(jq --arg head "$diagnostic_head" \
    '[.[] | select(.headSha == $head and .event == "pull_request")] | length' \
    "$candidate")
  if (( push_count > 1 || pull_request_count > 1 )); then
    printf 'ambiguous exact-head runs: push=%s pull_request=%s\n' \
      "$push_count" "$pull_request_count" >&2
    exit 1
  fi
  if (( push_count == 1 && pull_request_count == 1 )); then
    mv "$candidate" "$evidence_dir/runs.json"
    break
  fi
  if (( SECONDS >= deadline )); then
    printf 'exact-head runs did not register within 600 seconds: push=%s pull_request=%s\n' \
      "$push_count" "$pull_request_count" >&2
    exit 1
  fi
  sleep 10
done
jq -r --arg head "$diagnostic_head" \
  '.[] | select(.headSha == $head) | [.databaseId,.event,.status,.conclusion,.url] | @tsv' \
  "$evidence_dir/runs.json"
```

Expected: Exactly one `push` run and one `pull_request` run have `headSha=$diagnostic_head`.

Stop if either run is still missing after the bounded 600-second registration wait. Querying for registration is allowed; do not use `gh run rerun`.

- [ ] **Step 4: Watch both exact-head runs without reruns**

Run:

```bash
set -euo pipefail
diagnostic_head=$(git rev-parse HEAD)
evidence_dir="/tmp/den-darwin-diagnostic-$diagnostic_head"
push_run=$(jq -r --arg head "$diagnostic_head" \
  '.[] | select(.headSha == $head and .event == "push") | .databaseId' \
  "$evidence_dir/runs.json")
pull_request_run=$(jq -r --arg head "$diagnostic_head" \
  '.[] | select(.headSha == $head and .event == "pull_request") | .databaseId' \
  "$evidence_dir/runs.json")
set +e
gh run watch --repo rochecompaan/den "$push_run" --exit-status
push_status=$?
gh run watch --repo rochecompaan/den "$pull_request_run" --exit-status
pull_request_status=$?
set -e
printf 'push run %s watch status=%s\npull_request run %s watch status=%s\n' \
  "$push_run" "$push_status" "$pull_request_run" "$pull_request_status"
```

A nonzero watch status is evidence, not permission to rerun.

- [ ] **Step 5: Enumerate the four Darwin jobs and capture their logs**

Run:

```bash
set -euo pipefail
diagnostic_head=$(git rev-parse HEAD)
evidence_dir="/tmp/den-darwin-diagnostic-$diagnostic_head"
push_run=$(jq -r --arg head "$diagnostic_head" \
  '.[] | select(.headSha == $head and .event == "push") | .databaseId' \
  "$evidence_dir/runs.json")
pull_request_run=$(jq -r --arg head "$diagnostic_head" \
  '.[] | select(.headSha == $head and .event == "pull_request") | .databaseId' \
  "$evidence_dir/runs.json")
for run in "$push_run" "$pull_request_run"; do
  gh run view "$run" --repo rochecompaan/den --json jobs \
    > "$evidence_dir/$run-jobs.json"
  jq -e '
    [.jobs[].name] as $names |
    ($names | length) == 4 and
    ([
      "Native checks (x86_64-linux)",
      "Native checks (aarch64-linux)",
      "Native checks (x86_64-darwin)",
      "Native checks (aarch64-darwin)"
    ] - $names | length) == 0 and
    ([.jobs[] | select(
      .name == "Native checks (x86_64-darwin)" or
      .name == "Native checks (aarch64-darwin)"
    )] | length) == 2 and
    all(.jobs[];
      if (.name == "Native checks (x86_64-darwin)" or
          .name == "Native checks (aarch64-darwin)")
      then .conclusion == "failure"
      else true
      end)
  ' "$evidence_dir/$run-jobs.json" >/dev/null
  jq -r '.jobs[] |
    select(
      .name == "Native checks (x86_64-darwin)" or
      .name == "Native checks (aarch64-darwin)"
    ) |
    [.databaseId,.name,.conclusion] | @tsv
  ' "$evidence_dir/$run-jobs.json" > "$evidence_dir/$run-darwin-jobs.tsv"
  test "$(wc -l < "$evidence_dir/$run-darwin-jobs.tsv")" -eq 2
  while IFS=$'\t' read -r job name conclusion; do
    gh run view "$run" --repo rochecompaan/den --job "$job" --log \
      > "$evidence_dir/$run-$job.log"
    printf '%s\t%s\t%s\t%s\n' "$run" "$job" "$name" "$conclusion"
  done < "$evidence_dir/$run-darwin-jobs.tsv"
done
```

Expected: Each run has all four exact matrix job names exactly once, and the exact `x86_64-darwin` and `aarch64-darwin` jobs both concluded `failure`. Keep this directory available for Task 4, but do not commit it.

- [ ] **Step 6: Apply the Darwin evidence stop conditions**

Run:

```bash
set -euo pipefail
diagnostic_head=$(git rev-parse HEAD)
evidence_dir="/tmp/den-darwin-diagnostic-$diagnostic_head"
python3 - "$evidence_dir" <<'PY' | tee "$evidence_dir/records.tsv"
from pathlib import Path
import re
import sys

root = Path(sys.argv[1])
marker = "pure-launcher failure:"
pattern = re.compile(
    r"pure-launcher failure: phase=(\S+) line=([1-9][0-9]*) "
    r"status=([1-9][0-9]*) command=(.+)$"
)
logs = sorted(root.glob("*.log"))
if len(logs) != 4:
    raise SystemExit(f"expected four Darwin logs, found {len(logs)}")
for path in logs:
    lines = path.read_text(errors="replace").splitlines()
    payloads = {
        line[line.index(marker):].strip()
        for line in lines
        if marker in line
    }
    if len(payloads) != 1:
        raise SystemExit(f"{path}: expected one unique failure payload, found {sorted(payloads)!r}")
    payload = payloads.pop()
    match = pattern.fullmatch(payload)
    if not match:
        raise SystemExit(f"{path}: incomplete failure payload: {payload!r}")
    print("\t".join((path.name, *match.groups())))
PY
test "$(wc -l < "$evidence_dir/records.tsv")" -eq 4
```

Proceed to Task 4 only if all four jobs:

- Failed `pure-launcher`.
- Have exactly one unique failure payload after stripping transport prefixes and deduplicating identical replay lines.
- Include nonempty `phase`, numeric `line`, nonzero `status`, and nonempty `command`.
- Pass Task 4's stable comparison of phase, source line, and store-path-normalized command.

Stop and report the exact evidence if any Darwin job passes, omits a field, emits distinct failure payloads, or identifies another stable boundary. A Linux job failure does not invalidate complete Darwin evidence because the matrix uses `fail-fast: false`.

---

### Task 4: Design and execute only the evidence-based Darwin repair

**Files:**
- Modify: only the smallest Darwin-specific launcher, fake, fixture, or check-harness boundary named by Task 3 evidence.
- Test: an automated regression when production or reusable behavior changes; direct Nix evaluation/build checks when only the check harness changes.

**Interfaces:**
- Consumes: four complete, matching Darwin diagnostic records.
- Produces: one approved Darwin root-cause statement, one focused reviewed repair commit, and green Darwin jobs on both workflow triggers.

This task does not pre-authorize a code change. The exact boundary and correction must come from Task 3 evidence.

- [ ] **Step 1: Normalize and compare all four diagnostic records**

Run:

```bash
set -euo pipefail
diagnostic_head=$(git rev-parse HEAD)
evidence_dir="/tmp/den-darwin-diagnostic-$diagnostic_head"
test -f "$evidence_dir/runs.json"
python3 - "$evidence_dir" <<'PY'
from pathlib import Path
import re
import sys

root = Path(sys.argv[1])
marker = "pure-launcher failure:"
pattern = re.compile(
    r"pure-launcher failure: phase=(\S+) line=([1-9][0-9]*) "
    r"status=([1-9][0-9]*) command=(.+)$"
)
store_hash = re.compile(r"(?<=/nix/store/)[0-9a-z]{32}(?=-)")
collision_a = store_hash.sub("<store-hash>", "/nix/store/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-claude/bin/claude")
collision_b = store_hash.sub("<store-hash>", "/nix/store/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb-unrelated/bin/other")
if collision_a == collision_b:
    raise SystemExit("store-hash normalization erased command identity")
records = []
for path in sorted(root.glob("*.log")):
    payloads = {
        line[line.index(marker):].strip()
        for line in path.read_text(errors="replace").splitlines()
        if marker in line
    }
    if len(payloads) != 1:
        raise SystemExit(f"{path}: expected one unique failure payload, found {sorted(payloads)!r}")
    payload = payloads.pop()
    match = pattern.fullmatch(payload)
    if not match:
        raise SystemExit(f"{path}: incomplete failure payload: {payload!r}")
    phase, line, status, command = match.groups()
    normalized_command = store_hash.sub("<store-hash>", command)
    records.append((path.name, phase, line, status, command, normalized_command))

if len(records) != 4:
    raise SystemExit(f"expected four Darwin records, found {len(records)}")
boundaries = {(phase, line, normalized) for _, phase, line, _, _, normalized in records}
if len(boundaries) != 1:
    raise SystemExit(f"Darwin jobs disagree on stable boundary: {records!r}")
for path, phase, line, status, command, normalized in records:
    print("\t".join((path, phase, line, status, normalized, command)))
PY
```

Expected: Four rows and one shared `(phase, source line, store-hash-normalized command)` boundary. Only each 32-character Nix store hash is replaced; package names, executable suffixes, arguments, and each raw command remain available for identity and root-cause inspection.

- [ ] **Step 2: Trace the shared boundary and state one falsifiable root-cause hypothesis**

Use the diagnostic commit as the source of truth:

```bash
set -euo pipefail
diagnostic_head=$(git rev-parse HEAD)
evidence_dir="/tmp/den-darwin-diagnostic-$diagnostic_head"
git show "$diagnostic_head:nix/check-support/pure-launcher-darwin.nix" \
  > "$evidence_dir/pure-launcher-darwin.nix"
for system in x86_64-darwin aarch64-darwin; do
  nix derivation show ".#checks.$system.pure-launcher" \
    | jq -er '
        (.derivations // .) as $derivations |
        select(($derivations | length) == 1) |
        $derivations | to_entries[0].value.env.buildCommand |
        select(type == "string" and length > 0)
      ' \
    > "$evidence_dir/$system-buildCommand.sh"
  nl -ba "$evidence_dir/$system-buildCommand.sh"
done
```

The diagnostic `line` is a line in the realized `buildCommand`, not the literal line in the Nix source file. Use the numbered realized script for the exact line, then use the saved Nix source and reported command to trace the owning expression. Follow only the launcher, fake, fixture, or Nix expression reached by that boundary. Record:

- The common phase, command, line, and statuses.
- The exact called path and inputs on both Darwin architectures.
- Why the command fails after all logged Go tests pass.
- One root-cause hypothesis that predicts the four observed failures.
- The smallest Darwin-specific correction that would falsify or confirm it.
- Why the correction does not add a retry, host path, skipped assertion, or weaker sandbox behavior.

Apply `systematic-debugging` before proposing the fix. Stop if the evidence supports more than one materially different repair.

- [ ] **Step 3: Obtain user approval of the Darwin repair design**

Present the evidence and repair design before editing any file.

Expected: The user explicitly approves one exact repair boundary and validation contract.

- [ ] **Step 4: Implement the smallest approved repair with the Testing Value Gate**

If production or reusable behavior changes:

1. Apply `test-driven-development`.
2. Add a behavior-level regression at the owning boundary.
3. Demonstrate RED against the diagnostic head.
4. Make the smallest approved implementation change.
5. Demonstrate GREEN.

If only the Nix check harness changes, do not add a static source-text test. Use direct evaluation and build verification instead.

Do not touch Linux files in this task.

- [ ] **Step 5: Run focused and cross-platform checks**

Always run:

```bash
set -euo pipefail
nix-instantiate --parse nix/check-support/pure-launcher-darwin.nix >/dev/null
nix eval --raw .#checks.x86_64-darwin.pure-launcher.drvPath
printf '\n'
nix eval --raw .#checks.aarch64-darwin.pure-launcher.drvPath
printf '\n'
nix build .#checks.x86_64-linux.pure-launcher --no-link --print-build-logs
nix build .#checks.x86_64-linux.native-enforcement --no-link --print-build-logs
nix flake check --no-build
```

Also run the focused Go, shell, or Nix check that owns the repair. Expected: Every applicable local check passes, and the retained phase/error diagnostics remain present.

- [ ] **Step 6: Request fresh independent review of the final Darwin repair diff**

Give the reviewer:

- Base: the diagnostic commit.
- Head: the uncommitted repair diff.
- The four diagnostic records.
- The approved root-cause hypothesis and repair boundary.
- RED/GREEN evidence or direct Nix verification.
- The no-retry, no-host-path, no-assertion-skip, no-Linux-change constraints.

Expected: No Critical or Important findings and no required correction.

If an accepted finding changes the diff, rerun the affected checks and request another fresh review.

- [ ] **Step 7: Commit and push the reviewed Darwin repair through separate gates**

Read and follow the `commit` skill. Commit only the reviewed Darwin repair files with the focused message `fix(darwin): repair pure launcher boundary`. Then request explicit user approval for the push.

After approval, guard that the remote equals the repair commit's parent and push normally:

```bash
set -euo pipefail
branch=feat/den-claude-sandbox
repair_head=$(git rev-parse HEAD)
expected_remote=$(git rev-parse HEAD^)
test "$(git branch --show-current)" = "$branch"
git diff --quiet
git diff --cached --quiet
test -z "$(git status --porcelain)"
test "$(git ls-remote origin "refs/heads/$branch" | awk '{print $1}')" = "$expected_remote"
git push origin "HEAD:refs/heads/$branch"
```

- [ ] **Step 8: Inspect only matching exact-head CI**

Run this self-contained exact-head gate. It allows ten minutes for both workflow triggers to register but never reruns a workflow.

```bash
set -euo pipefail
repair_head=$(git rev-parse HEAD)
[[ $repair_head =~ ^[0-9a-f]{40}$ ]]
evidence_dir="/tmp/den-darwin-repair-$repair_head"
umask 077
rm -rf -- "$evidence_dir"
mkdir -m 0700 "$evidence_dir"
deadline=$((SECONDS + 600))
while true; do
  candidate="$evidence_dir/runs.candidate.json"
  gh run list --repo rochecompaan/den --workflow checks.yml \
    --branch feat/den-claude-sandbox --limit 20 \
    --json databaseId,event,headSha,status,conclusion,url,createdAt \
    > "$candidate"
  push_count=$(jq --arg head "$repair_head" \
    '[.[] | select(.headSha == $head and .event == "push")] | length' "$candidate")
  pull_request_count=$(jq --arg head "$repair_head" \
    '[.[] | select(.headSha == $head and .event == "pull_request")] | length' "$candidate")
  if (( push_count > 1 || pull_request_count > 1 )); then
    printf 'ambiguous exact-head runs: push=%s pull_request=%s\n' \
      "$push_count" "$pull_request_count" >&2
    exit 1
  fi
  if (( push_count == 1 && pull_request_count == 1 )); then
    mv "$candidate" "$evidence_dir/runs.json"
    break
  fi
  if (( SECONDS >= deadline )); then
    printf 'exact-head runs did not register within 600 seconds\n' >&2
    exit 1
  fi
  sleep 10
done

push_run=$(jq -r --arg head "$repair_head" \
  '.[] | select(.headSha == $head and .event == "push") | .databaseId' \
  "$evidence_dir/runs.json")
pull_request_run=$(jq -r --arg head "$repair_head" \
  '.[] | select(.headSha == $head and .event == "pull_request") | .databaseId' \
  "$evidence_dir/runs.json")
set +e
gh run watch --repo rochecompaan/den "$push_run" --exit-status
push_status=$?
gh run watch --repo rochecompaan/den "$pull_request_run" --exit-status
pull_request_status=$?
set -e
printf 'push watch status=%s pull_request watch status=%s\n' \
  "$push_status" "$pull_request_status"

for run in "$push_run" "$pull_request_run"; do
  gh run view "$run" --repo rochecompaan/den --json jobs \
    > "$evidence_dir/$run-jobs.json"
  jq -e '
    [.jobs[].name] as $names |
    ($names | length) == 4 and
    ([
      "Native checks (x86_64-linux)",
      "Native checks (aarch64-linux)",
      "Native checks (x86_64-darwin)",
      "Native checks (aarch64-darwin)"
    ] - $names | length) == 0 and
    all(.jobs[];
      if (.name == "Native checks (x86_64-darwin)" or
          .name == "Native checks (aarch64-darwin)")
      then .conclusion == "success"
      else true
      end)
  ' "$evidence_dir/$run-jobs.json" >/dev/null
  jq -r '.jobs[] | [.databaseId,.name,.conclusion] | @tsv' \
    "$evidence_dir/$run-jobs.json"
done
```

Expected:

- Exactly one push run and one pull-request run match the repair head.
- Each run contains the exact four required matrix job names.
- Both exact Darwin architecture jobs pass in both workflow triggers.
- Retained phase markers appear in successful logs where the build log exposes them.
- Linux results do not authorize a Linux repair without Task 5 evidence.

Stop after any failed Darwin result and report it exactly.

---

### Task 5: Investigate, design, and execute the separate Linux owner-boundary repair

**Files:**
- Temporary investigation only: local instrumented Fence or Den boundary; restore before any review.
- Allowed final Fence path: `patches/fence-0.1.58-den-tmpdir.patch`, `nix/lib/fence.nix`.
- Allowed final Den path when Den owns the redundant transition: the smallest owning launcher/runner file plus a boundary-level test.
- Possible Den regression files: `nix/check-support/native-enforcement.nix`, `nix/check-support/native-runner.sh`, `tests/native/native_test.go`, `tests/native/fixtures.go`, `scripts/check-native.sh`.

**Interfaces:**
- Consumes: the retained 20-run reproduction and temporary instrumentation of exec sequence, process identity, bootstrap state, and thread count.
- Produces: one owner decision, one red-green boundary regression, unchanged one/two bootstrap budgets, 100 consecutive serial packaged-runner passes, and one security-reviewed Linux repair commit.

This task starts only after the Darwin repair push is green. It does not infer ownership from the subtest name that happened to fail.

- [ ] **Step 1: Preserve and summarize the existing reproduction evidence**

The committed artifacts establish only this summary: 20 serial packaged-runner invocations produced 19 passes and one failure; the failing subtest was `custom_state_denies_defaults`, while CI had failed `policy_is_immutable`; the failed output contained:

```text
multithreaded exec cannot be safely continued in argv mode
Exec failed: operation not permitted
```

The exact command, elapsed time, and retained raw-output path are not recorded in the committed spec or plan. Require the operator who owns that artifact to supply them before Linux investigation:

```bash
set -euo pipefail
: "${DEN_NATIVE_20_RUN_LOG:?set to the retained 20-run raw-output file}"
: "${DEN_NATIVE_20_RUN_COMMAND:?set to the exact command used for the 20-run reproduction}"
: "${DEN_NATIVE_20_RUN_ELAPSED_SECONDS:?set to the measured elapsed seconds}"
test -f "$DEN_NATIVE_20_RUN_LOG"
test -s "$DEN_NATIVE_20_RUN_LOG"
grep -F 'multithreaded exec cannot be safely continued in argv mode' \
  "$DEN_NATIVE_20_RUN_LOG" >/dev/null
grep -F 'Exec failed: operation not permitted' "$DEN_NATIVE_20_RUN_LOG" >/dev/null
printf 'command=%s\npasses=19\nfailures=1\nelapsed_seconds=%s\nraw_output=%s\n' \
  "$DEN_NATIVE_20_RUN_COMMAND" "$DEN_NATIVE_20_RUN_ELAPSED_SECONDS" \
  "$DEN_NATIVE_20_RUN_LOG"
```

Expected: The supplied artifact is readable, contains both Fence errors, and its summary records the exact command, 19/1 exit count, elapsed time, and raw-output path. If the artifact or any metadata is unavailable, stop and ask the user; do not begin Task 5 instrumentation.

- [ ] **Step 2: Apply systematic debugging with temporary, noncommitted instrumentation**

Instrument the shared boundary to record only:

- Ordered exec candidates and argv.
- Seccomp notification process identity.
- Thread count or thread-count read error.
- Whether the candidate is a recognized bootstrap executable.
- Bootstrap budget before and after classification.
- Whether Landlock wrapping is active.

Do not print environment values, credentials, tokens, policy contents, or unrelated process data. Build the temporarily instrumented packaged runner and use this bounded serial capture loop. It retains every run and stops after 100 successful non-reproductions instead of running indefinitely.

```bash
set -euo pipefail
instrumented_head=$(git rev-parse HEAD)
[[ $instrumented_head =~ ^[0-9a-f]{40}$ ]]
umask 077
instrumented_dir=$(mktemp -d "/tmp/den-linux-instrument-$instrumented_head.XXXXXX")
instrumented_runner=$(nix build .#checks.x86_64-linux.native-enforcement \
  --no-link --print-build-logs --print-out-paths)
test -x "$instrumented_runner/bin/native-enforcement"
captured=0
: > "$instrumented_dir/summary.tsv"

for run in $(seq 1 100); do
  log="$instrumented_dir/run-$(printf '%03d' "$run").log"
  started=$(date +%s)
  set +e
  "$instrumented_runner/bin/native-enforcement" >"$log" 2>&1
  status=$?
  set -e
  elapsed=$(( $(date +%s) - started ))
  printf '%s\t%s\t%s\t%s\n' "$run" "$status" "$elapsed" "$log" \
    | tee -a "$instrumented_dir/summary.tsv"

  if (( status != 0 )) &&
     grep -Fq 'multithreaded exec cannot be safely continued in argv mode' "$log" &&
     grep -Fq 'Exec failed: operation not permitted' "$log"; then
    captured=$run
    break
  fi
  if (( status != 0 )); then
    printf 'unexpected instrumented failure: run=%s status=%s log=%s\n' \
      "$run" "$status" "$log" >&2
    exit "$status"
  fi
done

if (( captured == 0 )); then
  printf 'insufficient evidence: target failure not captured in 100 serial runs; logs=%s\n' \
    "$instrumented_dir" >&2
  exit 1
fi
printf 'captured target failure: run=%s logs=%s\n' "$captured" "$instrumented_dir"
```

Restore every instrumentation edit before designing the repair. After restoration, run:

```bash
set -euo pipefail
git diff --quiet
git diff --cached --quiet
test -z "$(git status --porcelain)"
```

Expected: All commands exit with status 0, including the check for untracked instrumentation artifacts.

If instrumentation does not capture the failure, stop and report insufficient evidence. Do not increase bootstrap authority or guess from the last failed subtest.

- [ ] **Step 3: Assign ownership from the recorded transition**

Choose exactly one path:

1. **Fence owns the error:** the trace shows incorrect Fence bootstrap classification or state accounting. Put the regression in Fence's `internal/sandbox` package and repair the Fence patch without increasing the budget.
2. **Den owns the error:** the trace shows Den creates a redundant bootstrap transition. Put the regression at the Den launch boundary and remove only that transition.
3. **Broader authority is required:** stop and request a new security design.

Broader authority includes a third continue, a new recognized bootstrap executable, a retry after unsafe thread state, bypassed thread-state verification, or relaxed candidate inspection.

- [ ] **Step 4: Present the Linux repair design for explicit user approval**

Present:

- The exact captured sequence.
- The owner decision.
- A regression test that fails before the repair.
- The minimal repair.
- Proof that one/two budgets and all deny cases remain unchanged.
- The full local verification and 100-run cost.

Expected: The user explicitly approves the owner, test, repair, and security acceptance contract before tracked edits.

- [ ] **Step 5: Use a red-green cycle at the owner boundary**

Apply `test-driven-development` and `verification-before-completion`.

For a Fence-owned repair:

- Add the approved owner regression to Fence's `internal/sandbox` tests in `patches/fence-0.1.58-den-tmpdir.patch`.
- Demonstrate that the owner regression fails with Fence 0.1.58 plus the existing Den patch before the repair hunk.
- Add the minimal Fence repair hunk.
- Demonstrate GREEN.

For a Den-owned repair:

- Add the approved behavior regression at the Den-owned launch boundary.
- Demonstrate RED before removing the redundant transition.
- Remove only the proven redundant transition.
- Demonstrate GREEN.

For either owner, add these missing fail-closed Fence tests to `patches/fence-0.1.58-den-tmpdir.patch`; they test reusable security decisions and pass the Testing Value Gate:

```text
TestEvaluateLinuxRuntimeExecDecisionForCandidate_NonBootstrapMultithreadedExecDenied
TestEvaluateLinuxRuntimeExecDecisionForCandidate_ThreadCountErrorDenied
TestEvaluateLinuxRuntimeExecDecision_ExecCandidateInspectionErrorsDenied
TestEvaluateLinuxRuntimeExecDecisionForCandidate_PolicyDeniedExecDenied
```

The candidate-inspection test must table-test an unreadable or missing executable path and unreadable argv and require `Allow == false`. The thread-count test must inject a read error and require `Allow == false`. The policy test must prove a configured deny rule wins before continue safety. The non-bootstrap test must use a multithreaded unrecognized executable and require denial.

Because the Fence patch changes for either owner:

- Update `expectedPatchHash` in `nix/lib/fence.nix`.
- Keep Fence at 0.1.58.
- Keep the current source-hash and upstream-version guards.
- Expand the focused `checkPhase` so the existing and new security tests run on every patched Fence build.

- [ ] **Step 6: Prove the Fence security invariants with an executable test selection**

Use this exact fixed test selection in `nix/lib/fence.nix`. If Fence owns the repair, add the separately approved owner-regression name to the same anchored expression.

```nix
      go test ./cmd/fence -count=1
      go test ./internal/sandbox -run '^(TestEnsureSandboxTMPDIRHonorsDenFenceTMPDIR|TestGenerateProxyEnvVars|TestMatchRuntimeExecPolicy_AllowOverridesDeny|TestEvaluateLinuxRuntimeExecDecisionForCandidate_BootstrapExecStillChecksContinueSafety|TestEvaluateLinuxRuntimeExecDecisionForCandidate_FirstBootstrapExecUsesConfiguredAllowance|TestEvaluateLinuxRuntimeExecDecisionForCandidate_BootstrapAllowanceCanCoverLandlockFlow|TestLinuxArgvExecMultithreadedBootstrapContinueBudget|TestIsLinuxBootstrapExecPath_OnlyAllowsStagedExecutables|TestEvaluateLinuxRuntimeExecDecisionForCandidate_NonBootstrapMultithreadedExecDenied|TestEvaluateLinuxRuntimeExecDecisionForCandidate_ThreadCountErrorDenied|TestEvaluateLinuxRuntimeExecDecision_ExecCandidateInspectionErrorsDenied|TestEvaluateLinuxRuntimeExecDecisionForCandidate_PolicyDeniedExecDenied)$' -count=1 -v
```

This selection proves:

- `TestLinuxArgvExecMultithreadedBootstrapContinueBudget`: budget 1 without Landlock and budget 2 with Landlock.
- `TestEvaluateLinuxRuntimeExecDecisionForCandidate_BootstrapAllowanceCanCoverLandlockFlow`: the third multithreaded bootstrap exec is denied.
- `TestIsLinuxBootstrapExecPath_OnlyAllowsStagedExecutables` plus `TestEvaluateLinuxRuntimeExecDecisionForCandidate_NonBootstrapMultithreadedExecDenied`: unrecognized multithreaded exec is denied.
- `TestEvaluateLinuxRuntimeExecDecisionForCandidate_ThreadCountErrorDenied`: thread-count read errors are denied.
- `TestEvaluateLinuxRuntimeExecDecision_ExecCandidateInspectionErrorsDenied`: path or argv inspection errors are denied.
- `TestEvaluateLinuxRuntimeExecDecisionForCandidate_PolicyDeniedExecDenied`: runtime-policy-denied exec is denied.

Run the patched package and integration checks:

```bash
set -euo pipefail
nix build .#checks.x86_64-linux.fence-capabilities --no-link --print-build-logs
nix build .#checks.x86_64-linux.fence-policy --no-link --print-build-logs
nix build .#checks.x86_64-linux.native-enforcement --no-link --print-build-logs
```

Expected: The changed Fence derivation executes the exact focused selection, every named test passes, and all three Nix builds pass. Stop if the patch omits any named test or `checkPhase` does not select it.

- [ ] **Step 7: Run 100 consecutive serial packaged native-enforcement checks**

Run on local `x86_64-linux` with no retry and no ignored failure:

```bash
set -euo pipefail
test "$(nix eval --impure --raw --expr builtins.currentSystem)" = x86_64-linux
umask 077
stress_dir=$(mktemp -d /tmp/den-native-100.XXXXXX)
runner=$(nix build .#checks.x86_64-linux.native-enforcement \
  --no-link --print-build-logs --print-out-paths)
test -x "$runner/bin/native-enforcement"
started=$(date +%s)
passed=0
: > "$stress_dir/summary.tsv"

for run in $(seq 1 100); do
  log="$stress_dir/run-$(printf '%03d' "$run").log"
  run_started=$(date +%s)
  set +e
  "$runner/bin/native-enforcement" >"$log" 2>&1
  status=$?
  set -e
  run_elapsed=$(( $(date +%s) - run_started ))
  printf '%s\t%s\t%s\n' "$run" "$status" "$run_elapsed" \
    | tee -a "$stress_dir/summary.tsv"
  if (( status != 0 )); then
    total_elapsed=$(( $(date +%s) - started ))
    printf 'FAILED run=%s status=%s passed=%s elapsed_seconds=%s log=%s\n' \
      "$run" "$status" "$passed" "$total_elapsed" "$log" >&2
    exit "$status"
  fi
  passed=$((passed + 1))
done

total_elapsed=$(( $(date +%s) - started ))
test "$passed" -eq 100
printf 'PASSED runs=%s failures=0 elapsed_seconds=%s runner=%s logs=%s\n' \
  "$passed" "$total_elapsed" "$runner" "$stress_dir"
```

Expected: `PASSED runs=100 failures=0`. Any nonzero run invalidates the stress result and returns the work to investigation. Keep the summary and all raw per-run logs available for review, but do not commit them. Budget approximately 15 to 17 minutes, 280 MiB peak memory, and the already shared 2.2 GiB cached Nix closure.

- [ ] **Step 8: Run final Linux and cross-platform verification**

Run:

```bash
set -euo pipefail
scripts/check-native.sh x86_64-linux
nix flake check --no-build
nix flake check --accept-flake-config --print-build-logs
nix eval --raw .#checks.x86_64-darwin.pure-launcher.drvPath
printf '\n'
nix eval --raw .#checks.aarch64-darwin.pure-launcher.drvPath
printf '\n'
```

Expected: Every command passes. The Darwin diagnostics and repair remain intact.

- [ ] **Step 9: Request fresh security-focused review of the final Linux diff**

Give the reviewer:

- Base: the green Darwin repair commit.
- Head: the uncommitted Linux repair diff.
- The 20-run reproduction and temporary instrumentation summary.
- The approved owner decision and repair design.
- RED/GREEN boundary regression output.
- All Fence security test output.
- The complete 100-run summary and raw-output location.
- The one/two budget and fail-closed constraints.

Expected: No Critical or Important findings and no required correction.

If an accepted finding changes the diff, repeat the affected security tests and the full 100-run gate, then request another fresh security review.

- [ ] **Step 10: Commit and push the reviewed Linux repair through separate gates**

Read and follow the `commit` skill. Commit only the approved Linux repair files with the focused message `fix(linux): repair native enforcement bootstrap`. Request explicit user approval for the push.

After approval, guard the exact state and push normally:

```bash
set -euo pipefail
branch=feat/den-claude-sandbox
linux_head=$(git rev-parse HEAD)
expected_remote=$(git rev-parse HEAD^)
test "$(git branch --show-current)" = "$branch"
git diff --quiet
git diff --cached --quiet
test -z "$(git status --porcelain)"
test "$(git ls-remote origin "refs/heads/$branch" | awk '{print $1}')" = "$expected_remote"
git push origin "HEAD:refs/heads/$branch"
```

Do not amend or force-push.

- [ ] **Step 11: Inspect exact-head final CI and stop on any error**

Run this self-contained exact-head gate. It waits only for workflow registration and completion; it does not rerun either workflow.

```bash
set -euo pipefail
linux_head=$(git rev-parse HEAD)
[[ $linux_head =~ ^[0-9a-f]{40}$ ]]
evidence_dir="/tmp/den-linux-repair-$linux_head"
umask 077
rm -rf -- "$evidence_dir"
mkdir -m 0700 "$evidence_dir"
deadline=$((SECONDS + 600))
while true; do
  candidate="$evidence_dir/runs.candidate.json"
  gh run list --repo rochecompaan/den --workflow checks.yml \
    --branch feat/den-claude-sandbox --limit 20 \
    --json databaseId,event,headSha,status,conclusion,url,createdAt \
    > "$candidate"
  push_count=$(jq --arg head "$linux_head" \
    '[.[] | select(.headSha == $head and .event == "push")] | length' "$candidate")
  pull_request_count=$(jq --arg head "$linux_head" \
    '[.[] | select(.headSha == $head and .event == "pull_request")] | length' "$candidate")
  if (( push_count > 1 || pull_request_count > 1 )); then
    printf 'ambiguous exact-head runs: push=%s pull_request=%s\n' \
      "$push_count" "$pull_request_count" >&2
    exit 1
  fi
  if (( push_count == 1 && pull_request_count == 1 )); then
    mv "$candidate" "$evidence_dir/runs.json"
    break
  fi
  if (( SECONDS >= deadline )); then
    printf 'exact-head runs did not register within 600 seconds\n' >&2
    exit 1
  fi
  sleep 10
done

push_run=$(jq -r --arg head "$linux_head" \
  '.[] | select(.headSha == $head and .event == "push") | .databaseId' \
  "$evidence_dir/runs.json")
pull_request_run=$(jq -r --arg head "$linux_head" \
  '.[] | select(.headSha == $head and .event == "pull_request") | .databaseId' \
  "$evidence_dir/runs.json")
gh run watch --repo rochecompaan/den "$push_run" --exit-status
gh run watch --repo rochecompaan/den "$pull_request_run" --exit-status

for run in "$push_run" "$pull_request_run"; do
  gh run view "$run" --repo rochecompaan/den --json jobs \
    > "$evidence_dir/$run-jobs.json"
  jq -e '
    [.jobs[].name] as $names |
    ($names | length) == 4 and
    ([
      "Native checks (x86_64-linux)",
      "Native checks (aarch64-linux)",
      "Native checks (x86_64-darwin)",
      "Native checks (aarch64-darwin)"
    ] - $names | length) == 0 and
    all(.jobs[]; .conclusion == "success")
  ' "$evidence_dir/$run-jobs.json" >/dev/null
  jq -r '.jobs[] | [.databaseId,.name,.conclusion] | @tsv' \
    "$evidence_dir/$run-jobs.json"
done

gh pr view 1 --repo rochecompaan/den \
  --json state,mergedAt,headRefOid,url \
  > "$evidence_dir/pr.json"
jq -er --arg head "$linux_head" \
  'select(.state == "OPEN" and .mergedAt == null and .headRefOid == $head) |
   "STATE=\(.state) MERGED=\(.mergedAt) HEAD=\(.headRefOid) URL=\(.url)"' \
  "$evidence_dir/pr.json"
```

Expected:

- Exactly one push run and one pull-request run match the Linux repair head.
- `x86_64-linux`: pass in both workflow triggers.
- `aarch64-linux`: pass in both workflow triggers.
- `x86_64-darwin`: remain green.
- `aarch64-darwin`: remain green.
- PR #1 remains open, unmerged, and at the Linux repair head.

Do not rerun a failed workflow. Report the exact new evidence and stop.
