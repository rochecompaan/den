# Darwin Claude Startup Host Fixture Design

**Status:** Approved for implementation

**Date:** 2026-08-23

## Summary

The Darwin Claude startup check currently runs its scenarios during a Nix derivation build. The check calls the host executable `/bin/ls` for native ACL inspection.

The CI workflow replaces Nix's baseline `allowed-impure-host-deps` value with `/bin/ls`. This replacement removes required baseline entries such as `/bin/sh`, so both Darwin jobs fail.

The fix will keep Nix sandboxing and package the Darwin startup fixture as an executable. The existing native runner will execute that fixture as the invoking host user. Linux startup checks will remain unchanged.

```text
Nix sandbox:
  build Claude
  build the platform claude-startup output
  build the native-enforcement runner

Invoking host user:
  run claude-settings-merge
  allocate a unique DEN_NATIVE_HOST_ROOT
  run the Darwin claude-startup fixture
  start the privileged resolver helper
  run the native Go suite
```

The workflow and native driver will no longer read or replace `allowed-impure-host-deps`.

## Problem

`nix/check-support/claude-startup-darwin.nix` executes `/bin/ls` inside a Nix derivation. It declares `/bin/ls` through `__impureHostDeps`.

Both Darwin startup derivations therefore request `/bin/ls` from the host. The workflow supplies that path with this Nix daemon setting:

```text
allowed-impure-host-deps = /bin/ls
```

This setting replaces the installer's baseline list. It does not append to that list. The resulting list excludes `/bin/sh`, which other Darwin builds require.

`extra-allowed-impure-host-deps` is not a suitable replacement. Support depends on the installed Nix version, and the current CI environment does not provide a reliable append operation.

The Darwin startup scenarios also use fixed `/private/tmp/den-task12-*` paths. A direct host move can let concurrent or interrupted runs remove shared host paths.

## Goals

The change must:

- preserve Nix sandboxing.
- preserve all four native CI targets.
- preserve the Linux Claude startup check without behavioral changes.
- build the Darwin startup fixture in Nix.
- execute the Darwin startup scenarios as the invoking host user.
- preserve every current Darwin startup and ACL assertion.
- use a unique runner-owned host root for every native run.
- preserve host execution of `claude-settings-merge`.
- preserve native Go enforcement tests.
- remove the workflow and driver dependency on `allowed-impure-host-deps`.
- fail before resolver startup and native Go tests when the Darwin startup fixture fails.
- provide CI logs that show each required build and execution stage.

## Non-goals

This change will not:

- port the startup scenarios to Go.
- change the Linux startup implementation.
- proxy `/bin/ls` into a Nix build sandbox.
- add `__noChroot`.
- broaden access to host paths.
- reduce startup, policy, resource, redaction, or ACL coverage.
- change production launcher behavior.
- change the public module or package API.
- merge PR #1.

## Architecture

### Platform startup outputs

`nix/check-support/claude-startup.nix` will retain its platform selection.

On Linux, `checks.claude-startup` will remain the current derivation. Its startup scenarios will continue to run during the derivation build.

On Darwin, `checks.claude-startup` will become a build-only executable package. Building it will package the scenario runner, fake Claude wrappers, ACL diagnostic probe, manifests, and runtime dependencies. Building it will not execute the startup scenarios or declare `/bin/ls` as an impure host dependency.

The Darwin executable will use `/bin/ls` only when the native runner executes it on the host. This preserves the production ACL probe semantics without changing the Nix daemon policy.

### Native enforcement package

`nix/check-support/native-enforcement.nix` will package the Darwin startup executable only for Darwin systems. It will export the executable to `native-runner.sh` through a private environment variable.

Linux native runners will not require or execute a Darwin startup fixture. Darwin native runners will fail at startup if the packaged executable is missing or is not executable.

This fixture is internal check infrastructure. It will not become a flake package or public module option.

### Native runner order

The native runner will use this order on Darwin:

1. Execute `claude-settings-merge` as the invoking host user.
2. Allocate a unique runner-owned root.
3. Export that root as `DEN_NATIVE_HOST_ROOT`.
4. Execute the packaged Darwin Claude startup fixture.
5. Start the privileged resolver helper.
6. Execute the native Go suite.
7. Stop the resolver helper and remove the runner-owned root.

The runner will print these stage messages:

```text
executing Claude settings merge fixture as the invoking host user
executing Darwin Claude startup fixture as the invoking host user
```

The existing driver will continue to print the build stages:

```text
building Claude for <system>
building non-native check claude-startup for <system>
building native runner for <system>
executing native runner as the invoking host user
```

A startup fixture error will stop the runner immediately. The runner will not start the resolver helper or execute the native Go suite after that error.

### Host-root ownership

The native runner will remain the only owner of its temporary root. It will create the root beneath `XDG_RUNTIME_DIR`, or beneath the invoking user's cache directory when `XDG_RUNTIME_DIR` is unset.

The Darwin startup fixture will use a dedicated child beneath `DEN_NATIVE_HOST_ROOT`. It can remove and recreate only that child. It must not use or remove fixed `/private/tmp/den-task12-*` paths.

The native runner will remove its complete root on normal exit, test failure, or handled termination. Existing cleanup status rules will continue to preserve the primary test error.

## Runtime manifest adapter

The packaged fake Claude wrappers contain immutable base manifests. Darwin host execution needs paths beneath the runtime value of `DEN_NATIVE_HOST_ROOT`, which is unknown during the Nix build.

A private adapter will create runtime manifests from those packaged base manifests. The adapter can change only:

- `explicitConfigDir`, when a scenario uses an explicit configuration directory.
- `aclProbe`, to select the packaged Darwin ACL diagnostic probe.

The adapter will normalize the base and runtime manifests by replacing these two fields with fixed comparison values. Normalization will preserve whether each field is present. The normalized documents must be equal. Any other change will stop the fixture before Claude starts.

Inherited configuration scenarios will continue to select their directory through `CLAUDE_CONFIG_DIR`. Their base and runtime manifests must both omit `explicitConfigDir`, or retain the same empty representation.

The adapter will be private to the Darwin startup fixture. Production manifest generation will remain unchanged.

## Darwin startup coverage

The host fixture must preserve all current scenario groups and assertions.

### Default state

The fixture will start with an empty home directory and no selected custom configuration directory. It will preserve assertions for default Claude state writes and expected policy grants.

### Custom state

The fixture will cover directories inside and outside the worktree. Each directory will run through both selection methods:

- explicit `configDir` in the runtime manifest.
- inherited `CLAUDE_CONFIG_DIR`.

The fixture will preserve directory mode and owner assertions. It will also preserve the absence of default-state write grants in custom mode.

### Resource integrity

The fixture will preserve user-managed skill, plugin, hook, settings, and MCP resources. It will compare their content before and after startup.

### Policy behavior

The fixture will preserve policy assertions for the selected configuration directory, account-home paths, and absolute deny paths. It will also preserve the requirement that policy paths do not contain unresolved home-relative forms.

### Rejected paths and launch ordering

The fixture will preserve rejection coverage for:

- canonical path overlap inside the worktree.
- canonical path overlap outside the worktree.
- final symbolic links inside the worktree.
- final symbolic links outside the worktree.
- failures that must occur before Fence starts.

The fixture will preserve marker assertions that prove rejected inputs do not reach Fence or the fake agent.

### Redaction and ACL diagnostics

The fixture will preserve token-redaction assertions. It will also preserve sanitized ACL diagnostics for failed launches.

The diagnostic probe will continue to report command status, target type, owner resolution, and scrubbed output. It must not expose the token, invoking user's name, or temporary host paths.

## Completion contract

After every Darwin startup scenario succeeds, the fixture will create this artifact:

```text
$DEN_NATIVE_HOST_ROOT/claude-startup.complete
```

The fixture will create the artifact only after its final assertion succeeds.

The native Go suite will require this artifact on Darwin before it runs its tests. Linux will not require it.

This artifact proves that the packaged host fixture completed in the same runner-owned root. The environment alone is not sufficient proof.

## Native driver and workflow

### Native driver

`scripts/check-native.sh` will remove the Darwin `nix config show allowed-impure-host-deps` check.

The driver will continue to:

1. match the requested system to `builtins.currentSystem`.
2. evaluate the flake.
3. build Claude.
4. build each non-native check.
5. require and build `claude-startup`.
6. build the native runner.
7. execute the native runner as the invoking host user.

The driver will fail if `claude-startup` is absent from the non-native check set. It will run the derivation-graph guard after non-native builds and before the native-runner build.

### Workflow

`.github/workflows/checks.yml` will preserve these targets:

- `x86_64-linux`.
- `aarch64-linux`.
- `x86_64-darwin`.
- `aarch64-darwin`.

The workflow will remove only matrix fields and installer settings related to `allowed-impure-host-deps`. Platform sandbox settings will remain.

A structured workflow check will parse the workflow configuration. It will require all four targets and reject an installer override for `allowed-impure-host-deps`. The check will not use brittle source-text matching.

Actionlint will continue to validate workflow syntax and expressions.

## Impure dependency guard

The native check path will inspect the recursive derivation graph for the relevant startup and native-runner outputs.

The guard will parse `nix derivation show --recursive` JSON. It will inspect recognized impure-host-dependency fields and reject `/bin/ls` as a complete dependency value.

The guard will not scan source text, builder scripts, or all derivation strings. Legitimate host-runtime command literals such as `/bin/ls` will therefore remain valid.

Recursive inspection will catch `/bin/ls` when it returns in a direct derivation or any packaged child derivation.

## Error handling

The Darwin fixture will use strict shell error handling. Each failed command or assertion will return a nonzero status.

The completion artifact will not exist after a partial run. The native runner will propagate the fixture status and skip later Darwin stages.

The runner will continue to clean its host root after fixture failure. Resolver cleanup rules will remain unchanged for errors that occur after resolver startup.

Sanitized ACL diagnostics will remain available in fixture output when a Claude startup command fails.

## Test plan

### Behavioral regression

`tests/check-native-driver.sh` will gain successful fake-runner flows for all four systems. Each flow will require this order:

1. Claude build.
2. `claude-startup` build.
3. native-runner build.
4. native-runner execution.

The fake `nix` command will fail any `nix config show allowed-impure-host-deps` call.

Before production edits, the new test must fail against the current driver. This is the required TDD red result.

### Darwin completion behavior

The Darwin fixture tests will prove that the completion artifact appears only after all startup scenarios succeed.

The native Go suite will reject a Darwin run when the artifact is absent. A forced startup failure must prove that resolver startup and native Go execution do not continue.

### Derivation graph behavior

A focused test will provide structured derivation JSON with:

- no impure dependency.
- `/bin/ls` as a runtime command literal.
- `/bin/ls` in a direct impure-host-dependency field.
- `/bin/ls` in a nested derivation's impure-host-dependency field.

Only the last two cases must fail.

### Workflow behavior

Direct validation will parse the workflow and inspect the Nix installer configuration. It will require the four exact native targets and reject an `allowed-impure-host-deps` override.

Actionlint will validate the resulting workflow. No automated test will assert raw YAML text.

### Focused verification

Focused verification will cover:

- the native driver regression test.
- shell syntax and lint checks for changed shell files.
- Nix evaluation of both Linux and Darwin startup outputs.
- the derivation-graph guard.
- structured workflow validation.
- Actionlint.
- the current host's focused Nix checks.

### Full verification

Full local verification will run the repository's complete flake check. Platform-limited checks will run on the current host, and CI will provide native Darwin execution.

Both duplicate GitHub Actions runs must complete: the push-triggered run and the pull-request-triggered run.

## Mutation tests

The implementation is acceptable only if these temporary mutations produce the specified failures:

1. Add `/bin/ls` to an impure-host-dependency field. The derivation-graph guard must fail.
2. Remove Darwin startup packaging. The required check or native-runner build must fail.
3. Remove Darwin startup execution. The completion contract or native suite must fail.
4. Force the startup fixture to fail. Resolver startup and native Go tests must not run.
5. Restore the workflow daemon override. Structured workflow validation must fail.
6. Remove one native matrix target. Structured workflow validation must fail.
7. Route Darwin startup through the Linux implementation. Darwin package or host execution validation must fail.

Each mutation will be reverted after its expected failure. The final clean tree will receive the complete verification run.

## Alternatives considered

### Port all startup scenarios to Go

This option places all native behavior in one test binary. It requires a broad rewrite of mature shell scenarios and creates unnecessary regression risk.

### Proxy only `/bin/ls` into the derivation

This option keeps scenario execution in the Nix build. It splits ACL validation between a proxy and the tested launch path, which weakens the meaning of the native ACL check.

### Append to the daemon dependency list

This option depends on unsupported or version-dependent Nix configuration. It does not provide a reliable CI fix.

## Acceptance criteria

The change is complete when all of these results are available:

1. Linux startup behavior remains unchanged.
2. Darwin builds Claude, the `claude-startup` executable, and the native runner in Nix.
3. Darwin executes `claude-settings-merge` and `claude-startup` as the invoking host user.
4. Darwin executes the startup fixture before resolver startup and native Go tests.
5. Every Darwin startup, policy, resource, redaction, overlap, symbolic-link, pre-Fence, and ACL diagnostic assertion remains.
6. Every Darwin startup path stays beneath a unique `DEN_NATIVE_HOST_ROOT`.
7. The Darwin startup derivation graph does not declare `/bin/ls` as an impure host dependency.
8. The workflow and native driver do not read or replace `allowed-impure-host-deps`.
9. Nix sandboxing remains enabled with the existing platform settings.
10. All four native matrix targets remain present.
11. Focused tests, full local verification, and all mutation tests produce the expected results.
12. Fresh independent review has no unresolved blocking findings.
13. Both push and pull-request CI runs pass at the pushed branch head.
14. PR #1 remains open and unmerged.
