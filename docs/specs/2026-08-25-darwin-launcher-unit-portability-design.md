# Darwin Launcher Unit Portability Design

## Summary

Fresh Darwin `launcher-unit` builds exposed four defects that predate Task 8 on PR #1:

1. test-created Docker and Podman socket paths exceed Darwin's Unix-socket path limit;
2. `git-transport-check` omits `grep` and therefore false-passes its final assertion;
3. config-directory write probing treats `/dev/fd/<directory-fd>` as a child-path namespace;
4. Darwin stale-directory cleanup uses the same unsupported `/dev/fd/<fd>/<child>` traversal.

The repair keeps the Go 1.24 floor, Nix sandboxing, all four CI targets, and Linux behavior unchanged. Darwin child operations become descriptor-relative through a pinned `golang.org/x/sys/unix` dependency.

## Scope

### In scope

- Fail closed when the Git transport validator cannot execute `grep`.
- Use short unique test roots only for Unix sockets created by tests.
- Probe Darwin config-directory writability through the already validated directory descriptor.
- Port the Linux stale-directory tombstone algorithm to Darwin with Darwin descriptor-relative syscalls.
- Vendor the new Go dependency into every Nix derivation that builds or tests the root Go module offline.
- Add focused behavioral and security regressions for the production changes.

### Out of scope

- Changes to Linux config-directory or stale-directory behavior.
- Changes to policy generation, Fence behavior, native runner order, workflow matrix values, or sandbox settings.
- Original-path `RemoveAll` fallbacks on Darwin.
- A Go version-floor increase.
- General refactoring of configdir, tempdir, or Nix check architecture.
- Changes to PR #1 state other than normal new commits and one normal push after verification.

## Root causes

### Socket fixtures

`testing.T.TempDir` nests the test name and sequence below the Nix build root. Six tests bind Unix sockets under those nested paths. Darwin rejects paths near or above its `sockaddr_un.sun_path` limit with `bind: invalid argument`.

The tests do not need the socket under their main fixture root. A direct `os.MkdirTemp("", "den-*-socket-")` directory is short enough while remaining unique and automatically removable.

### Git transport validation

`nix/check-support/git-transport.nix` replaces `PATH` with Git, coreutils, and diffutils paths, then invokes bare `grep`. `grep` is absent. Because the invocation is an `if` condition, `set -e` does not stop the derivation, and the check incorrectly reaches `touch "$out"`.

The derivation must include `pkgs.gnugrep`, and it must distinguish status 1 (no nonmatching lines) from execution errors greater than 1.

### Config-directory writability

The selected config directory is opened with `O_DIRECTORY|O_NOFOLLOW`, then validated by descriptor identity, ownership, mode, ACL, and effective writability. Linux can create a child through `/proc/self/fd/<fd>`. Darwin's `/dev/fd/<fd>` is not an equivalent traversable child namespace.

The Darwin implementation will call `unix.Openat` and `unix.Unlinkat` on the validated descriptor. Linux will retain the existing `/proc/self/fd` implementation behind a build-tagged helper.

### Darwin stale cleanup

The Linux cleanup algorithm opens the validated parent and candidate with no-follow descriptor-relative calls, locks the lease, atomically renames the candidate to a random tombstone, verifies identity, recursively removes by descriptor, and unlinks the verified tombstone.

The current Darwin fallback opens children through `/dev/fd` paths, so lease opening and recursive removal are not portable. Darwin will receive a platform implementation of the Linux algorithm using `x/sys/unix`:

- `Open`, `Openat`, `Fstatat`, and `Fstat` for no-follow identity binding;
- `Flock` for the lease;
- `RenameatxNp(..., RENAME_EXCL)` with bounded collision retries;
- `Unlinkat` for files and final directory removal;
- an identity check immediately before every directory unlink, including the final tombstone unlink.

The old pathname fallback remains available only for platforms other than Linux and Darwin.

## Dependency and Nix design

The module will require `golang.org/x/sys v0.35.0`. That release declares Go 1.23 and supports the Go 1.24 floor.

One Nix file will own the root module's vendor hash. Both root-module `buildGoModule` derivations will import it:

- `nix/packages/den-launcher.nix`;
- `nix/check-support/native-enforcement.nix` (`nativeTests`).

Direct offline Go test derivations will reuse `den-launcher.goModules`, link it as `vendor`, and run with `-mod=vendor`:

- `modules/checks/launcher-unit.nix`;
- `nix/check-support/pure-launcher-darwin.nix`;
- `nix/check-support/pure-launcher-linux.nix`.

The Linux test wiring changes only module sourcing, not tested behavior.

## Security invariants

1. No Darwin child creation or deletion uses `/dev/fd/<fd>/<child>`.
2. Config-directory probing operates on the already validated no-follow descriptor.
3. Probe names are random relative basenames, opened with `O_EXCL|O_NOFOLLOW|O_CLOEXEC`, and removed relative to the same descriptor.
4. Parent, candidate, moved tombstone, and final tombstone names are identity-checked without following symlinks.
5. A candidate lease is opened with `O_NOFOLLOW|O_CLOEXEC` and locked before rename.
6. Recursive deletion opens directories before files, never opens FIFOs or devices as files, and uses `Unlinkat` for non-directories.
7. A replacement at a candidate or tombstone name is preserved when identity no longer matches.
8. Random tombstones use 96 bits and `RENAME_EXCL`; collisions are retried only when reported as collisions.
9. Linux implementation files and runtime behavior remain unchanged.
10. Errors do not disclose protected filesystem paths beyond existing contracts.

## Test strategy

### Existing RED evidence

- Six Darwin socket tests fail with `bind: invalid argument`.
- Thirty-five config-selection/lifecycle markers fail after `Select` returns `custom configuration directory is not private`.
- Five Darwin stale cleanup tests leave old content behind.
- `git-transport-check` logs `grep: command not found` and false-passes.

### Focused tests

- Socket tests use short roots and retain their existing resolver and environment assertions.
- Darwin configdir tests prove the opened directory remains the probe target after the original pathname is replaced and that no probe artifact remains.
- Darwin tempdir tests prove candidate symlinks, lease symlinks, and candidate/tombstone replacements are preserved.
- Existing lease, age, ownership, tombstone-name, cleanup, ACL, rollback, and lifecycle tests continue to run.

### Compatibility and full verification

- Compile affected packages for Darwin amd64 and arm64.
- Run the root Go suite with Go 1.24.9.
- Run focused Nix builds and `nix flake check --no-build`.
- Run the full flake check.
- Obtain fresh independent review for each wave and a final branch review.
- Push once without force after all local evidence and reviews are clean.
- Require both push and pull-request runs to pass at the same head SHA while PR #1 remains open and unmerged.
