# Darwin Launcher Unit CI Follow-up Design

## Status

Validated by the user on 2026-08-26.

## Goal

Make the exact PR head pass the four native CI jobs after the first recovery push exposed two more Darwin portability errors.

## Evidence

The first recovery push produced these matching runs:

- Pull request run `32945162952`
- Push run `32945160024`
- Head `690d9b8c1f402ff8cfb35e2cd2143b2461e3dfd2`

The Darwin descriptor tests passed at runtime before later steps failed. The new config-directory and stale-tree implementations are not the cause of these failures.

The logs show three error classes:

1. `TestResolvePodmanUsesRuntimeFallback` created a 104-byte socket path on Darwin. Darwin needs space for the terminating null byte, so `bind` returned `invalid argument`.
2. `launcher-unit` called `${pkgs.util-linux}/bin/script` on Darwin. The referenced Darwin package output did not contain that executable.
3. The pull request `x86_64-linux` job failed `credential_symlinks`. The push job passed the same test at the same head.

## Scope

### Socket fixture roots

Change only the six socket-binding test call sites from the long patterns to short, readable patterns:

- `den-container-socket-` becomes `den-c-`.
- `den-launch-socket-` becomes `den-l-`.

Keep the current unique temporary-directory helpers and cleanup behavior. Do not change production socket discovery.

The shorter container pattern removes 15 bytes from the failing path. The observed 104-byte path becomes 89 bytes under the same Darwin Nix build root.

### Portable `script` package

Use `pkgs.unixtools.script` for `launcher-unit` on all platforms. Nixpkgs resolves this attribute to:

- `script-shell_cmds-326` on Darwin.
- `script-util-linux-2.42` on Linux.

Use the same package in `nativeBuildInputs` and the explicit command path. Do not add host paths or relax the Nix sandbox.

### Linux failure classification

Do not change production code for the pull-request-only Linux failure. The push job passed the same test at the same commit.

Retain the downloaded failure evidence. Run the direct Linux check during local verification. If the next push reproduces the error, stop and start a separate systematic investigation.

## Testing Strategy

The failed Darwin runs are the RED evidence for both repairs.

The existing socket tests remain the behavior tests. Do not add a source-text assertion for the fixture prefix.

The Nix package change is static configuration. Do not add a test that reads Nix source text. Use evaluation and direct derivation builds instead.

Before each commit:

1. Run focused checks for the changed area.
2. Run Darwin evaluation or cross-platform package evaluation where local execution is unavailable.
3. Request a fresh independent review.

Before the second push:

1. Run the Go 1.24.9 internal and command suites.
2. Cross-compile the Darwin config-directory and temporary-directory tests for `amd64` and `arm64`.
3. Build `launcher-unit`, `fence-policy`, and the direct Linux native check.
4. Run flake evaluation.
5. Run the full flake check.
6. Request a final independent exact-head review.

After the push, monitor only the matching push and pull-request runs. Do not rerun a workflow.

## Commit and Push Policy

Create two focused implementation commits after this design commit:

1. `fix(test): reduce Darwin socket fixture path budget`
2. `fix(nix): use portable script package in launcher checks`

Do not amend, force-push, merge, or close PR #1. Make one additional normal push after all local checks and reviews pass.

## Acceptance Criteria

- The six socket-binding tests use the approved short patterns.
- The longest observed failing socket path has at least 15 bytes of new headroom.
- `launcher-unit` uses `pkgs.unixtools.script` in its input set and command path.
- The Go floor remains `go 1.24.0`.
- Linux production behavior does not change.
- The four native CI jobs pass at the new exact head.
- PR #1 remains open and unmerged.
