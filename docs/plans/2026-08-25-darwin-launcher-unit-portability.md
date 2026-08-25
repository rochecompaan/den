# Darwin Launcher Unit Portability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Repair the four portability defects exposed by fresh Darwin `launcher-unit` builds without changing Linux behavior, sandbox settings, workflow coverage, or PR #1 state.

**Architecture:** Keep test-only socket paths short and make Git transport validation fail closed. Preserve the Go 1.24 floor while using pinned `golang.org/x/sys/unix` only in Darwin build-tagged adapters for descriptor-relative config probing and stale-tree removal. Share one vendored dependency source across every offline Nix consumer of the root Go module.

**Tech Stack:** Go 1.24, `golang.org/x/sys/unix` v0.35.0, Nix flakes, Bash, GitHub Actions, `gh`.

## Global Constraints

- Work only in `/home/roche/projects/den/.worktrees/den-claude-sandbox-design` on `feat/den-claude-sandbox`.
- Start implementation from `bd7988026edc84612d82ac5ec18f73889bd62ae3`; do not amend, rewrite, or force-push history.
- Keep PR #1 open and unmerged.
- Keep exactly one implementation writer in the shared worktree.
- Preserve the Go language floor at `go 1.24.0`.
- Keep Linux config-directory and stale-directory implementation behavior unchanged.
- Preserve Nix sandboxing and all current platform sandbox values.
- Preserve all four native matrix targets: `x86_64-linux`, `aarch64-linux`, `x86_64-darwin`, and `aarch64-darwin`.
- Do not add `__noChroot`, `allowed-impure-host-deps`, broader host access, skipped checks, retries that mask failures, or workflow changes.
- Do not use original-path `RemoveAll` as a Darwin descriptor-relative deletion substitute.
- Apply strict RED/GREEN TDD to production behavior. Current CI logs are accepted RED evidence for Darwin-only failures; add focused tests before implementation where a security boundary is not already isolated.
- Apply the Testing Value Gate: do not add tests that merely assert Nix source text, dependency versions, helper structure, or documentation.
- Obtain fresh independent task review after each commit and a broad final review before pushing.
- Make four focused commits, one per task. Include this plan and its design spec in Task 1's commit.
- Push once normally only after all focused/full checks and reviews are clean.

---

### Task 1: Make Git transport validation fail closed

**Files:**
- Modify: `nix/check-support/git-transport.nix:18-90`
- Add to commit: `docs/specs/2026-08-25-darwin-launcher-unit-portability-design.md`
- Add to commit: `docs/plans/2026-08-25-darwin-launcher-unit-portability.md`

**Interfaces:**
- Consumes: the existing `transport.log` containing exactly three fake SSH invocations.
- Produces: a derivation that succeeds only when packaged `grep` runs and every log line matches the allowed transport expression.

- [ ] **Step 1: Record the existing RED evidence**

Record in the task report that all current platform logs contain:

```text
git-transport-check> ...: grep: command not found
```

Also record that the old command is inside an `if`, so status 127 is misread as the expected no-match status and the derivation false-passes.

- [ ] **Step 2: Add the packaged dependency and fail-closed status handling**

Add `pkgs.gnugrep` to both `nativeBuildInputs` and the explicit `PATH`. Replace the old `if grep -v ...` block with status-aware handling equivalent to:

```sh
if unexpected=$(grep -v "^git@github.com git-\(upload\|receive\)-pack 'owner/repo.git'$" "$TMPDIR/transport.log"); then
  echo "Git used an unexpected transport helper" >&2
  printf '%s\n' "$unexpected" >&2
  exit 1
else
  grep_status=$?
  if test "$grep_status" -ne 1; then
    echo "Git transport validation failed with grep status $grep_status" >&2
    exit "$grep_status"
  fi
fi
```

Do not weaken the existing exact transport expression or invocation-count check.

- [ ] **Step 3: Verify the derivation behavior directly**

Run:

```sh
nix build .#checks.x86_64-linux.launcher-unit --no-link --print-build-logs
```

Expected: exit 0, no `grep: command not found`, and `git-transport-check` completes.

No new automated test is added because the derivation itself is the behavioral test and a source-text assertion would fail the Testing Value Gate.

- [ ] **Step 4: Review, commit, and leave the branch local**

Run a fresh task review against the Task 1 base/head range. Resolve every Critical or Important finding, then commit exactly the two documentation files and `nix/check-support/git-transport.nix`:

```text
fix(nix): make git transport validation fail closed
```

Do not push.

---

### Task 2: Shorten test-created launcher socket paths

**Files:**
- Modify: `internal/container/socket_test.go:13-177`
- Modify: `internal/launch/launch_test.go:317-345`

**Interfaces:**
- Consumes: `testing.T` and `os.MkdirTemp("", pattern)`.
- Produces: short unique directories used only for sockets that tests bind; all existing resolver and launch assertions remain unchanged.

- [ ] **Step 1: Re-run the focused RED test before editing**

Run:

```sh
go test ./internal/container -run '^TestResolveDockerPrecedenceAndCanonicalEndpoint$' -count=1
```

Expected in the current context-mode environment: FAIL at `net.Listen("unix", ...)` with `bind: invalid argument` because the nested `t.TempDir` path is too long.

- [ ] **Step 2: Add one focused test helper per package**

Use this package-local test helper shape in both affected packages:

```go
func shortSocketDir(t *testing.T, pattern string) string {
    t.Helper()
    directory, err := os.MkdirTemp("", pattern)
    if err != nil {
        t.Fatal(err)
    }
    t.Cleanup(func() {
        if err := os.RemoveAll(directory); err != nil {
            t.Errorf("remove socket directory: %v", err)
        }
    })
    return directory
}
```

Use a `den-container-socket-` pattern in `internal/container` and a `den-launch-socket-` pattern in `internal/launch`.

- [ ] **Step 3: Route only bound sockets through short roots**

In `internal/container/socket_test.go`, replace `root := t.TempDir()` with the short helper only in these tests:

```text
TestResolveDockerPrecedenceAndCanonicalEndpoint
TestResolveDockerDefaultSocketOrdering
TestResolveDockerRejectsInvalidEndpointsAndTargets
TestResolvePodmanDiscoveryOwnershipAndPorts
TestResolvePodmanUsesRuntimeFallback
```

In `TestRunAddsOnlyValidatedContainerEnvironment`, keep the existing main `root` for HOME and other fixtures, but create `socketPath` below a separate short socket directory.

Do not change production socket resolution or expectations.

- [ ] **Step 4: Verify focused GREEN and affected packages**

Run:

```sh
go test ./internal/container -run '^(TestResolveDockerPrecedenceAndCanonicalEndpoint|TestResolveDockerDefaultSocketOrdering|TestResolveDockerRejectsInvalidEndpointsAndTargets|TestResolvePodmanDiscoveryOwnershipAndPorts|TestResolvePodmanUsesRuntimeFallback)$' -count=1
go test ./internal/launch -run '^TestRunAddsOnlyValidatedContainerEnvironment$' -count=1
go test ./internal/container ./internal/launch -count=1
```

Expected: all commands exit 0.

Do not add a test that asserts helper implementation or path length. The existing socket behavior tests are the regressions.

- [ ] **Step 5: Review and commit**

Run a fresh task review against the Task 2 base/head range. Resolve every Critical or Important finding, then commit exactly the two test files:

```text
fix(test): shorten launcher socket fixture paths
```

Do not push.

---

### Task 3: Probe Darwin config directories by descriptor and vendor x/sys

**Files:**
- Modify: `go.mod`
- Create: `go.sum`
- Create: `nix/lib/den-go-vendor-hash.nix`
- Modify: `nix/packages/den-launcher.nix`
- Modify: `nix/check-support/native-enforcement.nix:181-198`
- Modify: `modules/checks/launcher-unit.nix`
- Modify: `nix/check-support/pure-launcher-darwin.nix`
- Modify: `nix/check-support/pure-launcher-linux.nix`
- Modify: `internal/configdir/configdir.go:179-200`
- Modify: `internal/configdir/access.go`
- Create: `internal/configdir/access_linux.go`
- Create: `internal/configdir/access_darwin.go`
- Create: `internal/configdir/access_darwin_test.go`
- Modify: `internal/configdir/handle_darwin.go`

**Interfaces:**
- Consumes: the validated `*os.File` directory handle already opened by `captureFinalState`.
- Produces: `directoryHandleWritable(file *os.File) bool` on Linux and Darwin.
- Darwin implementation uses only a random relative basename and descriptor-relative `unix.Openat`/`unix.Unlinkat`.
- Nix consumers use one shared vendor hash and one `den-launcher.goModules` source.

- [ ] **Step 1: Preserve and identify RED evidence**

Record the existing Darwin failures from `internal/configdir`, including `TestSelectACLPrivacy/owner_ACL` and `TestSelectCreatesMissingCustomDirectoryAt0700`, whose expected success paths return `custom configuration directory is not private`.

Before production edits, add `internal/configdir/access_darwin_test.go` with a Darwin-only behavior test that:

1. creates `original` and opens it with `openDirectoryHandle`;
2. renames it to `moved`;
3. creates a replacement `original` directory;
4. calls `directoryHandleWritable` with the still-open handle;
5. requires success;
6. requires no `.den-write-probe-*` artifact in either directory.

Name the break explicitly:

```go
func TestDirectoryHandleWritableUsesOpenedDirectoryAfterPathReplacement(t *testing.T)
```

The current production tree has no `directoryHandleWritable` implementation, so a Darwin compile before implementation must fail at that missing symbol. Current CI is the runtime RED for the old implementation.

- [ ] **Step 2: Pin the dependency without raising the Go floor**

Run:

```sh
go get golang.org/x/sys@v0.35.0
go mod tidy
```

Require `go.mod` to retain:

```go
go 1.24.0
```

and add:

```go
require golang.org/x/sys v0.35.0
```

- [ ] **Step 3: Add the platform seam without changing Linux behavior**

Keep `directoryWritable` and `directoryWritableWith` in `access.go` unchanged for their existing tests.

Add `access_linux.go`:

```go
//go:build linux

package configdir

import "os"

func directoryHandleWritable(file *os.File) bool {
    return directoryWritable(localDirectoryHandlePath(file))
}
```

In `configdir.go`, replace only:

```go
!directoryWritable(localDirectoryHandlePath(file))
```

with:

```go
!directoryHandleWritable(file)
```

- [ ] **Step 4: Implement the Darwin descriptor-relative probe**

In `access_darwin.go`:

- generate 12 random bytes with `crypto/rand` and hex-encode them after `.den-write-probe-`;
- try at most four names;
- call `unix.Openat(int(file.Fd()), name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)`;
- retry only `EEXIST`;
- after a successful open, always attempt both `unix.Unlinkat(int(file.Fd()), name, 0)` and `unix.Close(fd)`;
- return true only when create, unlink, and close all succeed;
- never pass a pathname containing `/` to the descriptor-relative calls.

Remove only the now-obsolete `localDirectoryHandlePath` function from `handle_darwin.go`. Keep `childDirectoryHandlePath` because Darwin ACL inspection still passes the open directory as child fd 9 to `/bin/ls -lde`.

- [ ] **Step 5: Wire one vendored module source through every offline Nix consumer**

Create `nix/lib/den-go-vendor-hash.nix` with this temporary fake-hash interface:

```nix
{ lib }:
lib.fakeHash
```

Import it as `import ../lib/den-go-vendor-hash.nix { lib = pkgs.lib; }` from package files and with the correct relative path from check-support files. Build once, then replace only `lib.fakeHash` with the reported fixed hash string while keeping the `{ lib }:` interface.

Import that shared value from both root-module `buildGoModule` derivations:

```text
nix/packages/den-launcher.nix
nix/check-support/native-enforcement.nix (nativeTests only)
```

For direct root-module test derivations, import `den-launcher`, link `${den-launcher.goModules}` as `vendor` inside the copied source, and add `-mod=vendor` to the Go test command:

```text
modules/checks/launcher-unit.nix
nix/check-support/pure-launcher-darwin.nix
nix/check-support/pure-launcher-linux.nix
```

Do not alter test selection or Linux runtime behavior.

- [ ] **Step 6: Verify GREEN, compatibility, and offline Nix builds**

Run:

```sh
gofmt -w internal/configdir/access_linux.go internal/configdir/access_darwin.go internal/configdir/access_darwin_test.go internal/configdir/configdir.go internal/configdir/handle_darwin.go
go test ./internal/configdir ./internal/launch -count=1
GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go test -c -o /tmp/den-configdir-darwin-amd64.test ./internal/configdir
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go test -c -o /tmp/den-configdir-darwin-arm64.test ./internal/configdir
GOTOOLCHAIN=go1.24.9 go test ./internal/... ./cmd/... -count=1
nix build .#packages.x86_64-linux.claude --no-link --print-build-logs
nix build .#checks.x86_64-linux.launcher-unit --no-link --print-build-logs
nix flake check --no-build
```

Expected: every command exits 0. The two Darwin commands prove compilation only; runtime GREEN remains a final Darwin CI requirement.

- [ ] **Step 7: Review and commit**

Run a fresh security-focused task review against the Task 3 base/head range. Require review of descriptor cleanup, collision handling, Go 1.24 compatibility, Nix vendor reuse, and unchanged Linux behavior. Resolve every Critical or Important finding, then commit the Task 3 files:

```text
fix(configdir): use Darwin-safe directory handle writes
```

Do not push.

---

### Task 4: Remove Darwin stale trees with descriptor-relative syscalls

**Files:**
- Create: `internal/tempdir/stale_darwin.go`
- Create: `internal/tempdir/remove_darwin.go`
- Modify: `internal/tempdir/stale_other.go`
- Modify: `internal/tempdir/remove_other.go`
- Delete: `internal/tempdir/descriptor_path_darwin.go`
- Modify: `internal/tempdir/tempdir_test.go`
- Create: `internal/tempdir/remove_opened_linux_test.go`
- Create: `internal/tempdir/stale_darwin_test.go`
- Preserve unchanged: `internal/tempdir/stale.go`
- Preserve unchanged: `internal/tempdir/remove_linux.go`

**Interfaces:**
- Consumes: validated root path, invoking uid, age threshold, lease name, temporary-name validation, and 96-bit tombstone names.
- Produces: Darwin `removeStale` with the Linux algorithm's descriptor, lock, rename, identity, and recursive-deletion guarantees.
- Produces: `removeTreeAt(parentFD int, name string, childFD int) error` and Darwin recursive helpers.

- [ ] **Step 1: Add security-focused Darwin RED tests before implementation**

Create `stale_darwin_test.go` with `//go:build darwin` and real filesystem tests for these breaks:

```go
func TestRemoveStaleDarwinRejectsLeaseSymlink(t *testing.T)
func TestRemoveStaleDarwinPreservesTemporaryNameSymlinkTarget(t *testing.T)
func TestRemoveTreeAtDarwinPreservesReplacementAtTombstone(t *testing.T)
```

The lease-symlink test must replace the candidate's lease with a symlink and require the candidate to remain. The temporary-name-symlink test must require both the symlink and its non-temporary target contents to remain. The tombstone-replacement test must hold the original child fd, move the original directory to another name, create a replacement at the supplied tombstone name, call `removeTreeAt`, and require the replacement to remain because final identity no longer matches.

Current Darwin CI supplies RED for the existing five cleanup tests; the new tests isolate the no-follow and final-unlink boundaries.

- [ ] **Step 2: Separate the Darwin and fallback implementations**

Change the old fallback build tags to:

```go
//go:build !linux && !darwin
```

in both `stale_other.go` and `remove_other.go`.

Delete `descriptor_path_darwin.go`; Darwin production code must have no `/dev/fd/<fd>/<child>` path construction.

Do not modify `stale.go` or `remove_linux.go`.

- [ ] **Step 3: Port stale selection and tombstoning to Darwin**

Implement `stale_darwin.go` using `golang.org/x/sys/unix` and the Linux sequence:

1. validate the root and derive the per-user parent;
2. take a no-follow parent snapshot with `os.Lstat`;
3. open the parent with `unix.Open(...O_DIRECTORY|O_NOFOLLOW|O_CLOEXEC)`;
4. compare snapshot, fd stat, and current no-follow parent stat; require private ownership;
5. enumerate through a duplicated parent fd;
6. open each temporary-name candidate with `Openat(...O_DIRECTORY|O_NOFOLLOW|O_CLOEXEC)`;
7. validate ownership, mode, age, and identity;
8. open the lease with `Openat(...O_RDWR|O_NOFOLLOW|O_CLOEXEC)` and acquire nonblocking `Flock`;
9. create a 96-bit random tombstone name;
10. rename with `unix.RenameatxNp(..., unix.RENAME_EXCL)`, retrying only collision errors for at most four names;
11. open the moved tombstone with no-follow directory flags and compare its identity to the candidate;
12. call `removeTreeAt`.

Treat `ENOENT`, `ELOOP`, and `ENOTDIR` as a raced-away or unsafe candidate and preserve it. Propagate other unexpected errors.

- [ ] **Step 4: Implement recursive Darwin deletion with final identity checks**

In `remove_darwin.go`:

- enumerate a duplicated directory fd;
- first try each child with `Openat(...O_DIRECTORY|O_NOFOLLOW|O_CLOEXEC)`;
- recurse when it is a directory;
- on `ENOTDIR` or `ELOOP`, unlink the name with `Unlinkat(..., 0)` without opening it as a file;
- before `Unlinkat(..., AT_REMOVEDIR)`, compare `Fstatat(parentFD, name, ..., AT_SYMLINK_NOFOLLOW)` with `Fstat(childFD)`;
- if identity differs or the name is absent, preserve the replacement and return nil;
- apply that same check immediately before final tombstone removal.

Never use `os.RemoveAll` or reconstruct the original candidate pathname.

- [ ] **Step 5: Keep existing behavioral coverage and retire the obsolete helper test**

Keep the existing generic tests for age, lease locking/release, SIGKILL residue, valid tombstones, ownership, and cleanup.

Move `TestRemoveOpenedDirectoryContentsPreservesReplacementPath` unchanged from `tempdir_test.go` into `remove_opened_linux_test.go` with `//go:build linux`. This preserves direct regression coverage for the unchanged Linux helper. The Darwin replacement-path contract is covered by `TestRemoveTreeAtDarwinPreservesReplacementAtTombstone`.

- [ ] **Step 6: Verify GREEN and unchanged Linux behavior**

Run:

```sh
gofmt -w internal/tempdir/stale_darwin.go internal/tempdir/remove_darwin.go internal/tempdir/stale_darwin_test.go internal/tempdir/remove_opened_linux_test.go internal/tempdir/stale_other.go internal/tempdir/remove_other.go internal/tempdir/tempdir_test.go
go test ./internal/tempdir -count=1
GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go test -c -o /tmp/den-tempdir-darwin-amd64.test ./internal/tempdir
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go test -c -o /tmp/den-tempdir-darwin-arm64.test ./internal/tempdir
GOTOOLCHAIN=go1.24.9 go test ./internal/... ./cmd/... -count=1
nix build .#checks.x86_64-linux.launcher-unit --no-link --print-build-logs
```

Expected: every command exits 0 and `git diff` shows no changes to `stale.go` or `remove_linux.go`.

- [ ] **Step 7: Review and commit**

Run a fresh security-focused task review against the Task 4 base/head range. Require review of symlink rejection, lease locking, collision handling, replacement preservation, recursive file-type handling, and the final identity check. Resolve every Critical or Important finding, then commit:

```text
fix(tempdir): remove Darwin stale trees by descriptor
```

Do not push.

---

## Final verification, review, and CI

After all four task reviews are clean:

- [ ] Run formatting and the complete Go suite with the module floor:

```sh
gofmt -w internal/container/socket_test.go internal/launch/launch_test.go internal/configdir/*.go internal/tempdir/*.go
GOTOOLCHAIN=go1.24.9 go test ./internal/... ./cmd/... -count=1
```

- [ ] Compile all root packages for both Darwin architectures:

```sh
GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go test -c -o /tmp/den-darwin-amd64.test ./internal/tempdir
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go test -c -o /tmp/den-darwin-arm64.test ./internal/tempdir
GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go test -c -o /tmp/den-configdir-darwin-amd64.test ./internal/configdir
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go test -c -o /tmp/den-configdir-darwin-arm64.test ./internal/configdir
```

- [ ] Run focused Nix and full flake verification:

```sh
nix build .#checks.x86_64-linux.launcher-unit --no-link --print-build-logs
nix flake check --no-build
nix flake check --accept-flake-config --print-build-logs
```

- [ ] Run a broad fresh-context reviewer against base `bd7988026edc84612d82ac5ec18f73889bd62ae3` and current HEAD. Resolve every Critical or Important finding with one fix worker and one scoped re-review.

- [ ] Verify exactly four new commits, a clean worktree/index, unchanged Linux production files, and PR #1 still open/unmerged.

- [ ] Push once normally:

```sh
git push origin feat/den-claude-sandbox
```

- [ ] Find and monitor one `push` run and one `pull_request` run at the same head SHA. Require all four jobs to pass in both runs. Do not rerun, merge, or close PR #1.

- [ ] If Darwin still fails, retrieve exact logs, return to systematic debugging, and stop before any additional fix scope that lacks explicit approval.
