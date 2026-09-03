# Darwin Launcher Unit CI Follow-up Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the two remaining Darwin `launcher-unit` portability errors and make the four native CI jobs pass.

**Architecture:** Keep both repairs at their existing test and Nix boundaries. Reduce only test fixture path length, and use Nixpkgs' portable `script` package without host paths or sandbox changes.

**Tech Stack:** Go 1.24.9, Nix, Nixpkgs, GitHub Actions, `gh`

## Global Constraints

- Work only in `/home/roche/projects/den/.worktrees/den-claude-sandbox-design` on `feat/den-claude-sandbox`.
- Start from design commit `9924c5caa50b0b67a0ebf0f73f99b1b692af9514`.
- Keep the Go floor at `go 1.24.0`.
- Keep all four CI platforms.
- Do not change production socket discovery or Linux production behavior.
- Do not add host paths or relax the Nix sandbox.
- Do not add source-text tests for fixture prefixes or Nix expressions.
- Use the existing failed Darwin runs as RED evidence.
- Request a fresh independent review before each implementation commit.
- Do not amend, force-push, merge, close PR #1, or rerun a workflow.
- Make one additional normal push only after all local checks and reviews pass.
- If the final CI fails, stop and report the exact failure. Do not push again.

---

### Task 1: Reduce Darwin socket fixture path budget

**Files:**
- Modify: `internal/container/socket_test.go:14,47,71,101,165`
- Modify: `internal/launch/launch_test.go:319`
- Test: `internal/container/socket_test.go`
- Test: `internal/launch/launch_test.go`

**Interfaces:**
- Consumes: `shortSocketDir(t *testing.T, pattern string) string` in each test package.
- Produces: Unique socket roots with `den-c-` and `den-l-` patterns.

- [ ] **Step 1: Make sure that the checkout has the expected state**

Run:

```bash
cd /home/roche/projects/den/.worktrees/den-claude-sandbox-design
test "$(git branch --show-current)" = "feat/den-claude-sandbox"
test "$(git rev-parse HEAD)" = "9924c5caa50b0b67a0ebf0f73f99b1b692af9514"
git diff --quiet
git diff --cached --quiet
```

Expected: All commands exit with status 0.

- [ ] **Step 2: Reproduce the container socket error with the current pattern**

Create a 44-byte temporary root and run the failing behavior test:

```bash
container_tmp=$(python3 - <<'PY'
import os
path = "/tmp/" + ("c" * 39)
assert len(path.encode()) == 44
os.makedirs(path, exist_ok=True)
print(path)
PY
)
TMPDIR="$container_tmp" GOTOOLCHAIN=go1.24.9 \
  go test ./internal/container \
  -run '^TestResolvePodmanUsesRuntimeFallback$' -count=1
```

Expected: FAIL at `net.Listen` with a Unix socket path error. The long pattern adds 15 bytes more than `den-c-`.

- [ ] **Step 3: Reproduce the launch socket error with the current pattern**

Create a 67-byte temporary root and run the failing behavior test:

```bash
launch_tmp=$(python3 - <<'PY'
import os
path = "/tmp/" + ("l" * 62)
assert len(path.encode()) == 67
os.makedirs(path, exist_ok=True)
print(path)
PY
)
TMPDIR="$launch_tmp" GOTOOLCHAIN=go1.24.9 \
  go test ./internal/launch \
  -run '^TestRunAddsOnlyValidatedContainerEnvironment$' -count=1
```

Expected: FAIL at `net.Listen` with a Unix socket path error.

- [ ] **Step 4: Replace the six long fixture patterns**

In `internal/container/socket_test.go`, change all five calls:

```go
root := shortSocketDir(t, "den-c-")
```

In `internal/launch/launch_test.go`, change the one call:

```go
socketPath := filepath.Join(shortSocketDir(t, "den-l-"), "docker.sock")
```

Do not change either `shortSocketDir` helper.

- [ ] **Step 5: Run both calibrated tests again**

Run the commands from Steps 2 and 3 with the same temporary roots.

Expected: Both tests PASS. The container path has at least 15 bytes of new headroom.

- [ ] **Step 6: Run focused Go checks**

Run:

```bash
GOTOOLCHAIN=go1.24.9 go test ./internal/container ./internal/launch -count=1
GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 GOTOOLCHAIN=go1.24.9 \
  go test -c -o /tmp/den-container-darwin-amd64.test ./internal/container
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 GOTOOLCHAIN=go1.24.9 \
  go test -c -o /tmp/den-container-darwin-arm64.test ./internal/container
GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 GOTOOLCHAIN=go1.24.9 \
  go test -c -o /tmp/den-launch-darwin-amd64.test ./internal/launch
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 GOTOOLCHAIN=go1.24.9 \
  go test -c -o /tmp/den-launch-darwin-arm64.test ./internal/launch
```

Expected: All commands PASS.

- [ ] **Step 7: Request a fresh focused review**

Give the reviewer:

- The design and this task.
- Base `9924c5caa50b0b67a0ebf0f73f99b1b692af9514`.
- The uncommitted two-file diff.
- The calibrated RED/GREEN evidence.
- The exact six-call-site constraint.

Expected: No Critical or Important findings remain.

- [ ] **Step 8: Commit Task 1**

```bash
git add internal/container/socket_test.go internal/launch/launch_test.go
git commit -m "fix(test): reduce Darwin socket fixture path budget"
```

Expected: The commit contains exactly two files.

---

### Task 2: Use the portable Nix `script` package

**Files:**
- Modify: `modules/checks/launcher-unit.nix:14-22`

**Interfaces:**
- Consumes: `pkgs.unixtools.script`, which selects a native `script` derivation for each platform.
- Produces: A `launcher-unit` derivation that invokes the packaged `script` executable on Linux and Darwin.

- [ ] **Step 1: Record the existing Darwin RED evidence**

Use these exact errors from push run `32945160024`:

```text
/nix/store/...-util-linux-2.42-bin/bin/script: No such file or directory
builder failed with exit code 127
```

Expected: The task report identifies this as the RED state. Do not rerun the GitHub workflow.

- [ ] **Step 2: Evaluate the portable package on all four platforms**

Run:

```bash
root=$PWD
for system in x86_64-darwin aarch64-darwin x86_64-linux aarch64-linux; do
  nix eval --raw --impure --expr "
    let
      f = builtins.getFlake \"path:$root\";
      system = \"$system\";
      pkgs = import f.inputs.nixpkgs { inherit system; };
    in pkgs.unixtools.script.outPath
  "
  printf '\n'
done
```

Expected:

- Darwin paths contain `script-shell_cmds-326`.
- Linux paths contain `script-util-linux-2.42`.

- [ ] **Step 3: Replace the package and select native command syntax**

Add these bindings to the existing `let` block:

```nix
script = pkgs.unixtools.script;
scriptInvocation =
  if pkgs.stdenv.hostPlatform.isDarwin then
    "${script}/bin/script -q /dev/null ./process-harness pty"
  else
    "${script}/bin/script -qfec './process-harness pty' /dev/null";
```

Change the derivation input from:

```nix
nativeBuildInputs = [ pkgs.go pkgs.util-linux pkgs.procps pkgs.python3 pkgs.jq ];
```

To:

```nix
nativeBuildInputs = [ pkgs.go script pkgs.procps pkgs.python3 pkgs.jq ];
```

Change the command from:

```nix
${pkgs.util-linux}/bin/script -qfec './process-harness pty' /dev/null > pty.out
```

To:

```nix
${scriptInvocation} > pty.out
```

The Darwin provider uses positional command arguments. The Linux provider keeps the util-linux `-c` syntax. Do not add `/usr/bin/script` or another host path.

- [ ] **Step 4: Evaluate both Darwin checks**

Run:

```bash
nix eval --raw .#checks.x86_64-darwin.launcher-unit.drvPath
printf '\n'
nix eval --raw .#checks.aarch64-darwin.launcher-unit.drvPath
printf '\n'
```

Expected: Both derivation paths evaluate without an error.

- [ ] **Step 5: Build the direct Linux check**

Run:

```bash
nix build .#checks.x86_64-linux.launcher-unit --no-link --print-build-logs
nix flake check --no-build
```

Expected: Both commands PASS.

- [ ] **Step 6: Request a fresh focused review**

Give the reviewer:

- The design and this task.
- The Task 1 commit as the base.
- The uncommitted one-file diff.
- The Darwin RED log and four-platform package evaluation.
- The no-host-path and no-sandbox-relaxation constraints.

Expected: No Critical or Important findings remain.

- [ ] **Step 7: Commit Task 2**

```bash
git add modules/checks/launcher-unit.nix
git commit -m "fix(nix): use portable script package in launcher checks"
```

Expected: The commit contains exactly one file.

---

### Task 3: Classify the Linux duplicate-event error

**Files:**
- No tracked file changes.

**Interfaces:**
- Consumes: The push and pull-request logs for head `690d9b8c1f402ff8cfb35e2cd2143b2461e3dfd2`.
- Produces: A stop-or-proceed decision for the final push gate.

- [ ] **Step 1: Record the conflicting same-head evidence**

Record:

```text
pull_request x86_64-linux: TestFilesystemEnforcement/credential_symlinks failed
push x86_64-linux: success
head: 690d9b8c1f402ff8cfb35e2cd2143b2461e3dfd2
```

Expected: The report does not classify a one-event error as a deterministic product regression.

- [ ] **Step 2: Build the direct Linux native check once**

Run:

```bash
nix build .#checks.x86_64-linux.native-enforcement --no-link --print-build-logs
```

Expected: PASS.

If this command fails, stop. Apply `systematic-debugging` before a code change.

- [ ] **Step 3: Make sure that no tracked file changed**

Run:

```bash
git diff --quiet
git diff --cached --quiet
```

Expected: Both commands exit with status 0.

Do not create a commit for this task.

---

### Task 4: Final verification, review, push, and CI monitoring

**Files:**
- No additional tracked file changes.

**Interfaces:**
- Consumes: Both reviewed implementation commits and the approved documentation commits.
- Produces: One normally pushed exact head with monitored push and pull-request CI runs.

- [ ] **Step 1: Run the complete Go verification**

Run:

```bash
files=$(gofmt -l \
  internal/container/socket_test.go \
  internal/launch/launch_test.go \
  internal/configdir \
  internal/tempdir)
test -z "$files"
GOTOOLCHAIN=go1.24.9 go test ./internal/... ./cmd/... -count=1
```

Expected: Formatting is clean and all Go tests PASS.

- [ ] **Step 2: Cross-compile the Darwin security tests**

Run:

```bash
GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 GOTOOLCHAIN=go1.24.9 \
  go test -c -o /tmp/den-configdir-darwin-amd64.test ./internal/configdir
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 GOTOOLCHAIN=go1.24.9 \
  go test -c -o /tmp/den-configdir-darwin-arm64.test ./internal/configdir
GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 GOTOOLCHAIN=go1.24.9 \
  go test -c -o /tmp/den-tempdir-darwin-amd64.test ./internal/tempdir
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 GOTOOLCHAIN=go1.24.9 \
  go test -c -o /tmp/den-tempdir-darwin-arm64.test ./internal/tempdir
```

Expected: All four binaries compile.

- [ ] **Step 3: Run the direct Nix checks**

Run:

```bash
nix build .#checks.x86_64-linux.launcher-unit --no-link --print-build-logs
nix build .#checks.x86_64-linux.fence-policy --no-link --print-build-logs
nix build .#checks.x86_64-linux.native-enforcement --no-link --print-build-logs
```

Expected: All three builds PASS.

- [ ] **Step 4: Run the flake checks**

Run:

```bash
nix flake check --no-build
nix flake check --accept-flake-config --print-build-logs
```

Expected: Both commands PASS.

- [ ] **Step 5: Make sure that the branch is ready for review**

Run:

```bash
test "$(git branch --show-current)" = "feat/den-claude-sandbox"
git diff --quiet
git diff --cached --quiet
git log --oneline 690d9b8c1f402ff8cfb35e2cd2143b2461e3dfd2..HEAD
```

Expected: The worktree and index are clean. The range contains the design, plan, and two implementation commits.

- [ ] **Step 6: Request a final fresh exact-head review**

Give the reviewer:

- Base `690d9b8c1f402ff8cfb35e2cd2143b2461e3dfd2`.
- The exact local head.
- The design, this plan, full diff, and verification evidence.
- Both prior CI run links and root-cause evidence.
- The user-approved second-push exception.

Expected: The reviewer returns `Ready for normal push? Yes` with no Critical or Important findings.

- [ ] **Step 7: Guard and make one normal push**

Run:

```bash
expected_remote=690d9b8c1f402ff8cfb35e2cd2143b2461e3dfd2
branch=feat/den-claude-sandbox
test "$(git branch --show-current)" = "$branch"
git diff --quiet
git diff --cached --quiet
actual_remote=$(git ls-remote origin "refs/heads/$branch" | awk '{print $1}')
test "$actual_remote" = "$expected_remote"
git push origin "HEAD:refs/heads/$branch"
```

Expected: One non-force push succeeds.

- [ ] **Step 8: Monitor only matching exact-head runs**

Use `gh run list` to identify the push and pull-request runs whose `headSha` equals the pushed head. Watch both run IDs with `gh run watch --exit-status`.

Expected:

- `x86_64-linux`: PASS.
- `aarch64-linux`: PASS.
- `x86_64-darwin`: PASS.
- `aarch64-darwin`: PASS.

Do not use `gh run rerun`.

- [ ] **Step 9: Make sure that PR #1 remains open and unmerged**

Run:

```bash
gh pr view 1 --repo rochecompaan/den \
  --json state,mergedAt,headRefOid,url \
  --jq '"STATE=\(.state) MERGED=\(.mergedAt) HEAD=\(.headRefOid) URL=\(.url)"'
```

Expected: `STATE=OPEN` and `MERGED=null`.
