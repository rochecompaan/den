# Den Pi Support Design

**Status:** Draft. User approval pending

**Date:** 2026-09-05

**Branch:** `feat/den-pi-sandbox`

**Base:** `810c6eba1685d58497b432747c7f03f6f78eb756`

## Process gates

The section-by-section design discussion approved the scope and decisions in this document. It did not approve the final written specification.

The work must pass these gates in order:

1. Complete self-review and independent adversarial review of this specification.
2. Commit only the reviewed specification.
3. Get explicit user approval for the committed specification.
4. Write, review, and commit a separate implementation plan.
5. Get explicit user approval for the committed implementation plan.
6. Start implementation only after both approvals.

No implementation plan or production-code change can start while this specification awaits approval.

## Summary

Den will add Pi as a second sandboxed coding agent. Users will run the normal `pi` command.

The first release will package `@earendil-works/pi-coding-agent` version 0.84.4 from a fixed npm source. Den will wrap the complete Pi process in Fence.

Pi will use RepoWolf for all GitHub API, Git, and SSH operations. Pi will use isolated configuration and session directories.

Den will supply no starter skills, prompts, themes, packages, or convenience extensions. Users can add immutable resources through Nix configuration.

A trusted project can load its local Pi resources from the repository. A mandatory Darwin security extension is the only bundled extension.

The existing Claude package and public API will not change.

## Goals

This work must:

- provide a reproducible Pi 0.84.4 package.
- provide the normal `pi` command.
- run the complete Pi process inside Fence.
- preserve RepoWolf as the only GitHub API, Git, and SSH route.
- isolate Pi configuration, credentials, trust decisions, and sessions from host Pi state.
- support user-selected resources from immutable Nix store paths.
- preserve Pi project trust for repository resources.
- prevent runtime package resolution and mutation.
- preserve argv-aware command enforcement for Pi's built-in shell tool and user `!` shell entry point on Darwin.
- keep Den telemetry optional, sanitized, and outside synchronization paths.
- support Linux and Darwin on x86-64 and ARM64.
- preserve all existing Claude behavior and checks.

## Non-goals

This work will not:

- reproduce the Roche Pi wrapper.
- bundle Superpowers or other starter resources.
- import host Pi configuration.
- import host `auth.json`.
- put credential values in Nix configuration or derivations.
- support Pi self-updates or runtime package installation.
- trust projects automatically.
- add a public `programs.den.agents.*` interface.
- add provider-specific network policy to Den.
- set provider-specific or model-specific Pi environment variables.
- change the Den telemetry model.
- change the default Claude package output.
- support Windows.
- merge PR #1.
- modify protected Claude worktrees.
- modify EC2 Mac infrastructure or Terraform state.
- inspect `infra/ec2-mac-debug/terraform.tfstate`.

## Approved approach

Den will make a targeted generalization of the existing agent adapter seam.

`mkAgentSandbox` already accepts an agent adapter. The launcher manifest also has a generic `Agent` record.

The implementation will remove only the remaining Claude-specific assumptions that block Pi. It will not redesign the public module hierarchy.

A separate Pi implementation duplicates sandbox logic. A complete public multi-agent redesign adds migration risk without a current requirement.

## Public interface

Den will add these flake outputs:

- `packages.${system}.pi`
- `lib.${system}.mkPi`

The Home Manager and devenv modules will add `programs.den.pi`. Pi will remain disabled by default in both modules.

The initial module shape is:

```nix
programs.den.pi = {
  enable = true;
  agentDir = null;
  sessionDir = null;
  extraPkgs = [ ];

  resources = {
    skills = [ ];
    promptTemplates = [ ];
    themes = [ ];
    extensions = [ ];
    packages = [ ];
  };

  docker = {
    enable = false;
    package = pkgs.docker-client;
    composePackage = pkgs.docker-compose;
    socketPath = null;
    hostPorts = [ ];
  };

  podman = {
    enable = false;
    package = pkgs.podman;
    composePackage = pkgs.podman-compose;
    socketPath = null;
    hostPorts = [ ];
  };
};
```

`agentDir` and `sessionDir` accept `null` or an absolute path. The direct `mkPi` constructor accepts the same values without `enable`.

Each resource entry must be a Nix path or package that becomes an absolute store path. Local Nix paths enter the store before use. String package sources, URLs, and mutable host paths are not accepted.

`packages.default` will remain the Claude package. Existing `mkClaude` and `programs.den.claude` interfaces will remain unchanged.

## Upstream Pi package

Den's pinned nixpkgs does not provide `pkgs.pi`. Den will add a dedicated Pi derivation.

The derivation will use the published npm tarball for `@earendil-works/pi-coding-agent` version 0.84.4. The source and dependency closure will use fixed hashes.

The build will not use network access. The result will include the normal `pi` executable and its required runtime files.

The derivation will apply one fixed, hash-checked Den hardening patch. The patch will introduce an immutable Den-build package lock. This lock will not derive from a mutable environment value.

The patch will make the Den-built Pi binary reject `install`, `remove`, `uninstall`, `update`, `list`, and `config`. It will reject them before they can load project-trust extensions, change trust state, or mutate settings or package state. It will also guard every Pi 0.84.4 package mutation path that extension code can reach. This includes paths exposed through `DefaultPackageManager` and runtime-visible JavaScript properties. The guarded paths include package installation, removal, update, settings mutation, npm and Git execution, and managed package-directory mutation. The guard will run before each side effect.

Package resolution and reload will behave offline according to the immutable package lock, even if extension code changes or deletes `process.env.PI_OFFLINE`. These rules will apply to direct and nested execution of the immutable Pi binary.

The same patch will capture the canonical selected session root before extension loading. Pi's common runtime session-switch path will resolve each requested session file to a canonical existing regular file under that root. It will open the canonical path, not the untrusted alias. A path outside the root, a sibling-prefix path, or a symbolic-link escape will fail. A changed root identity will also fail. The guard will run before extension events, file reads, or session replacement. This guard will cover interactive, RPC, and extension-initiated session switches. Later changes to `process.env.PI_CODING_AGENT_SESSION_DIR` will not change the captured root.

The Pi adapter will assert version 0.84.4 and the hardening-patch hash. An unreviewed source, version, or patch change will stop evaluation or the build.

Credential files and credential environment values will not enter this derivation.

## Fence dependency

Pi will use the same shared Fence package as Claude. This work will not upgrade or fork that dependency.

The package will remain Den's pinned and patched Fence 0.1.58. Its source hash will remain `sha256-ACe3N4bXYJW6QDQHtRChFWOTXTZTbEUbZ4d8cuFRqMY=`. Its Den patch hash will remain `4be4f0266a0a79da10002893752ea8185915f6ecfb146513946bde8a96e41e2a`.

The existing version, source-hash, patch-hash, and capability assertions will remain active. The Pi adapter will require the existing `claudePreToolUse`, `denFenceTmpdir`, `strictDenyRead`, and platform enforcement capabilities. Evaluation will fail if the shared package does not provide them.

A capability test will exercise the exact 0.1.58 helper protocol while `FENCE_SANDBOX=1`. It will prove that an allowed command returns the successful no-change result and that a blocked command returns a denial. It will also prove that this helper path does not initialize proxy listeners. The existing patched Fence tests must pass. Claude native fixtures must retain their behavior and assertion strength on all four systems.

A later Fence version, source, or patch change will require a separate reviewed dependency change. It is not part of Pi support.

## Agent adapter architecture

A new `nix/lib/mk-pi.nix` will define Pi facts. These facts include:

- the agent name.
- the immutable executable.
- the output command name.
- the required runtime packages.
- mandatory arguments.
- reserved user arguments and commands.
- state-directory environment bindings.
- the immutable resource arguments.
- the Darwin security extension.

`nix/lib/mk-agent-sandbox.nix` will continue to assemble the shared sandbox. It will derive the package name, manifest name, main program, and wrapper name from the adapter.

The shared builder will not contain Pi-specific conditionals. Agent-specific validation will remain in focused packages.

## Launcher manifest

The manifest is the versioned boundary between Nix and `den-launcher`. The schema will advance to version 2.

Version 2 will describe:

- the agent name.
- the agent executable.
- the wrapper command name.
- mandatory arguments.
- reserved user arguments and commands.
- one or more state-directory bindings.
- the immutable resource arguments and Pi runtime package directory.
- an optional platform security adapter.
- the existing Fence, RepoWolf, policy, closure, container, and platform fields.

Each state binding will identify its environment variable, explicit path, inherited path, and Den-owned default. The launcher will validate every field before state mutation.

The Claude adapter will emit a version 2 manifest with the same effective behavior as version 1.

## Pi state isolation

Pi will use two writable state directories:

- `PI_CODING_AGENT_DIR` for Pi configuration, `auth.json`, trust decisions, and Pi-owned state.
- `PI_CODING_AGENT_SESSION_DIR` for session files.

Each directory will use this selection order:

1. The explicit constructor or module value.
2. The corresponding inherited Pi environment variable.
3. The Den-owned default.

The default agent directory will be `$HOME/.local/state/den/pi/agent`. The default session directory will be `$HOME/.local/state/den/pi/sessions`. Here, `$HOME` is the inherited runtime home.

The launcher will require absolute custom paths. It will canonicalize each path and reject final-component symbolic links.

The launcher will validate the owner, mode, ACL, parent directories, and protected-path overlap. It will reject overlapping agent and session directories.

The launcher will resolve the invoking account home separately from the inherited runtime home. It will reject overlap with `.pi/agent` under either home, including canonical and symbolic-link-resolved aliases. When the two homes differ, Fence will deny both host Pi paths.

Pi also discovers global skills from `.agents/skills` under the host home independently of `PI_CODING_AGENT_DIR`. Fence will deny `.agents` under both homes. The project-trust exception applies only to `.agents/skills` inside the validated repository, never to these host-global paths.

The launcher will create a missing selected directory with mode `0700`. It will record path identity and ACL state before Fence starts.

The launcher will revalidate each directory immediately before process start. A changed path, owner, mode, ACL, or ancestor will stop the launch. The patched Pi runtime will also verify the captured session-root identity before every runtime session switch.

If startup fails, Den will remove only unchanged directories that Den created for that launch. Den will commit all selected directories after Fence starts the child process.

The Fence policy will grant writes only to the validated state directories, the worktree, and the per-launch scratch directory.

## Credential handling

Den will not define Nix options for credential values. Den will not read or copy `auth.json` from `.pi/agent` under the invoking account home or inherited runtime home.

Provider environment variables will remain runtime values. The launcher will not serialize them into the manifest, policy, logs, or telemetry.

Pi can create and use `auth.json` inside the selected agent directory. Fence will block access to both host Pi directories and both host-global `.agents` directories.

Existing environment scrubbing will continue to remove GitHub credentials and unsafe Git or SSH transport variables. This rule keeps RepoWolf as the repository route.

## Configured resources

Den will support these configured resource classes:

- skills.
- prompt templates.
- themes.
- extensions.
- Pi packages.

Den will supply no entries by default. Each configured entry must resolve to a Nix store path.

The accepted store-path shapes are:

- An extension entry is a `.ts` or `.js` file, or a directory that Pi 0.84.4 accepts as an extension source.
- A skill entry is a `SKILL.md` file or a directory that Pi 0.84.4 scans for skills.
- A prompt-template entry is a Markdown file or a directory of prompt templates.
- A theme entry is a JSON theme file or a directory of themes.
- A Pi-package entry is a directory with a valid `package.json` `pi` manifest or Pi's convention directories. Its runtime dependencies must already exist in the store closure.

Nix evaluation will validate option types and convert local Nix paths to store paths. The package build will validate realized file types, required metadata, manifest paths, and recognized resources. A missing or malformed configured resource will fail the build before the wrapper is available.

The adapter will preserve option-list order, but it will not define a fictional order between resource types. Pi 0.84.4 loads and de-duplicates extensions, skills, prompt templates, and themes independently.

The adapter will construct these effective per-type sequences:

1. Extensions: direct configured extensions, then extensions from configured Pi packages, then Pi's enabled trusted-project and user extension sources.
2. Skills: skills from configured Pi packages, then Pi's enabled trusted-project and user sources, then direct configured skills.
3. Prompt templates: templates from configured Pi packages, then Pi's enabled trusted-project and user sources, then direct configured templates.
4. Themes: themes from configured Pi packages, then Pi's enabled trusted-project and user sources, then direct configured themes.

Within each subgroup, entries will retain their Nix option-list order. A permitted `--no-*` discovery flag will omit the corresponding trusted-project and user subgroup without changing the order of mandatory configured resources. Duplicate canonical store paths in one configured class will fail the build. Pi 0.84.4's first-loaded resource wins a same-name collision within one type, and Pi reports the later resource as the loser. The mandatory Darwin security adapter remains outside this ordinary extension order and retains final ownership of its protected shell entry points.

The Pi adapter will add the validated resources as mandatory CLI inputs. Direct resources will map to `--extension`, `--skill`, `--prompt-template`, and `--theme`. Configured Pi packages will map to local store-path `--extension` sources. Pi will load their manifest or convention resources without npm, Git, or network resolution. Compatibility tests will freeze the effective sequence, winner, loser, and diagnostic for every resource type.

`PI_PACKAGE_DIR` will point to the immutable Pi 0.84.4 runtime root that contains Pi's own package metadata and assets. It will not point to a third-party resource bundle or writable package cache.

Den will set `PI_OFFLINE=1`. This setting preserves Pi's documented offline behavior and provides defense in depth. The immutable Den-build package lock, not this mutable environment value, is the package-safety authority.

The launcher will reject package-management mutations before Pi starts. The hardening patch will make Pi skip absent npm and Git sources without consulting mutable process state. Mandatory resources use local store paths. `PI_PACKAGE_DIR` is not a package-installation control.

Normal model requests will remain subject to Fence policy. Den will not set `PI_SKIP_VERSION_CHECK` or `PI_TELEMETRY` separately.

The selected agent directory remains writable because Pi stores configuration, credentials, and trust decisions there. The supported global-resource interface is the immutable Nix interface.

Den will not mount or copy resources from `.pi/agent` or `.agents` under either host home.

## Project resources and trust

Pi 0.84.4 has a built-in project-trust gate. Den will preserve this gate.

Interactive Pi will prompt before it loads untrusted project configuration or resources. Pi will store saved decisions in the isolated agent directory.

Non-interactive modes will preserve Pi's `defaultProjectTrust` behavior. The `--approve` and `--no-approve` options will remain available.

Before trust, Pi can load context files, global extensions, and mandatory command-line extensions. This behavior is part of Pi's trust model.

After trust, Pi can load direct repository resources from `.pi` and `.agents/skills`. This exception applies only inside the validated repository. It does not include `.agents/skills` under either host home.

Project package declarations cannot cause runtime resolution or installation. Pi 0.84.4 skips absent npm and Git package sources in offline mode. Local project package paths can load only after project trust and remain inside Fence.

Configured Nix package resources are mandatory store inputs. Their absence or invalid shape fails the package build instead of causing a runtime installation.

Fence remains the outer boundary for all trusted project content. Trust does not grant more filesystem, process, command, or network access.

## Argument and command validation

A focused Pi argument package will validate user arguments before state mutation. Its grammar will match Pi 0.84.4 and will use this policy:

| Input class | Pi 0.84.4 forms | Den policy |
| --- | --- | --- |
| Session directory | `--session-dir <dir>` | Reject. Den installs the validated session directory. |
| Direct session file | `--session <path-or-id>`, `--fork <path-or-id>`, `--export <file>` | Reject `--export` and all path-like `--session` or `--fork` values. Permit only hexadecimal partial or full UUID values for `--session` and `--fork`. The patched common runtime switch guard separately confines interactive, RPC, and extension paths. |
| Session lookup | `--continue`, `-c`, `--resume`, `-r`, `--session-id <id>`, `--no-session` | Permit. These forms use the validated session directory. `--session-id` must contain a UUID. |
| Resource source | `--extension <source>`, `-e <source>`, `--skill <path>`, `--prompt-template <path>`, `--theme <path>` | Reject user-supplied forms. Den owns the mandatory resource arguments. |
| Discovery control | `--no-extensions`, `-ne`, `--no-skills`, `-ns`, `--no-prompt-templates`, `-np`, `--no-themes` | Permit only while Pi 0.84.4 compatibility tests prove that mandatory CLI resources and the Darwin security adapter still load. |
| Project trust | `--approve`, `-a`, `--no-approve`, `-na` | Permit. |
| Package management | First positional command `install`, `remove`, `uninstall`, `update`, `list`, or `config` | Reject. The Den hardening patch also rejects these commands inside every direct or nested execution of the immutable Pi binary. Pi 0.84.4's `list` path performs project-trust and extension bootstrap, so Den does not classify it as read-only. |
| Option terminator | `--` | Preserve Pi semantics. Package dispatch examines the first token before normal option parsing, so `pi -- install` is a prompt, not a package command. |

The package-command classification will be deny-by-default. Pi 0.84.4 has no allowed package command. A package command added by a later Pi version will remain blocked until a reviewed version change classifies it.

Den prepends mandatory Pi arguments before user arguments. Pi 0.84.4 dispatches a package command only when it is the first process argument. The validator must therefore reject a user package-command token before Pi starts. It must not let Pi reinterpret that token as a prompt after Den's mandatory resource arguments.

Pi 0.84.4 does not accept equals forms for these built-in options or combined short options. Den will still reject equals, attached-value, and combined spellings that target a reserved long or short name. Other unknown options will remain available for configured extensions and Pi's own diagnostics.

The immutable Pi runtime will be closure-only and absent from the controlled child `PATH`. An `extraPkgs` entry that supplies a `pi` command will fail the package build. The wrapper remains the only `pi` command on the sandbox path.

The compatibility suite will exercise every table row, missing values, repeated options, and the option terminator. It will also exercise nested wrapper calls and direct execution of the immutable Pi binary. Package tests will cover global and project declarations, unchanged trust, settings, and package directories, and zero requests to package-resolution servers. They will prove that wrapper and direct-binary `list` attempts stop before project-trust extensions load.

A hostile extension fixture will import `DefaultPackageManager`, change or delete `process.env.PI_OFFLINE`, and attempt public and runtime-visible internal package mutations. It will also trigger resource reload after changing the environment. Every attempt must fail before settings, managed package directories, subprocesses, or network activity change.

A later Pi upgrade must update the parser, hardening patch, and compatibility tests before it changes the pinned version.

## Runtime flow

The generated `pi` wrapper will call `den-launcher` with the immutable manifest. The launcher will use this sequence:

1. Load and validate the complete manifest.
2. Validate the Pi arguments and command.
3. Load RepoWolf runtime configuration.
4. Resolve the invoking account home directory.
5. Select and validate the Pi agent directory.
6. Select and validate the Pi session directory.
7. Resolve optional container sockets.
8. When the platform is Linux, do the Linux Fence feature preflight.
9. Build the controlled child environment.
10. Create private policy and scratch directories.
11. Prepare the RepoWolf CA file.
12. Generate a strict per-launch Fence policy.
13. Revalidate both Pi state directories.
14. Revalidate the mandatory Darwin security configuration.
15. Start Fence around the immutable Pi executable.
16. Commit newly created state directories after process start.
17. Return the Pi or Fence exit status.

The launcher will preserve standard input, standard output, standard error, terminal behavior, signals, and process-group behavior.

## Controlled environment

The launcher will remove inherited values for these Den-controlled variables:

- `PI_CODING_AGENT_DIR`.
- `PI_CODING_AGENT_SESSION_DIR`.
- `PI_PACKAGE_DIR`.
- `PI_OFFLINE`.
- `PATH`.
- `REPOWOLF_*`.
- GitHub credential variables.
- unsafe Git and SSH transport variables.

The launcher will reinstall the validated Pi directories and immutable Pi runtime package directory. It will also install the existing RepoWolf values and controlled `PATH`.

Other provider variables will remain runtime values. Den will not add a provider allowlist.

Den will set `PI_OFFLINE=1` for package safety. Den will not set provider, model, or separate update and telemetry variables.

Fence will remain the network-policy authority for normal model traffic.

## RepoWolf and Fence

RepoWolf will remain the only route for GitHub API, Git, and SSH operations. The Pi work does not add another Git or network path.

The launcher will preserve the current Git URL rewrites, SSH command, CA handling, and GitHub credential scrubbing.

Fence will wrap the complete Pi process. This boundary includes Pi tools, user resources, trusted project resources, and extension child processes.

The existing Fence policy remains the source of network decisions. Pi support will not add provider-domain options to Den.

Filesystem denials will continue to take precedence over grants. Pi support will deny `.pi/agent` and host-global `.agents` under both the invoking account home and inherited runtime home, including canonical aliases.

Den telemetry will remain optional, sanitized, non-fatal, and outside RepoWolf synchronization paths.

## Darwin security extension

Darwin cannot provide argv-aware command policy for child processes from the outer process boundary alone. Existing Claude support uses a mandatory `PreToolUse` hook for this reason.

Den will supply one mandatory Pi security extension on Darwin. The extension will be an immutable Nix store resource.

The extension will cover Pi's built-in `bash` tool and native `user_bash` event, which handles user `!` shell commands. For each command, it will invoke the store-pinned Fence `--claude-pre-tool-use` helper with the private policy file. The extension will send a Claude-compatible `PreToolUse` envelope with the command and current working directory. This helper name describes its input protocol; its command-policy evaluation is not specific to the Claude agent.

Den's pinned Fence 0.1.58 checks the command against the selected policy before it observes `FENCE_SANDBOX=1`. Inside Den's outer Fence, an allowed command produces the helper's successful no-change result. A blocked command produces its deny response. The helper does not initialize a Fence manager, start HTTP or SOCKS proxies, or create a nested operating-system sandbox.

The extension will execute the original command only after the helper returns its expected successful no-change result. It will reject an explicit denial. It will also fail closed on a spawn error, non-zero status, or malformed output. A rewritten command or evidence that `FENCE_SANDBOX=1` is absent will also fail closed.

The Pi adapter will install this enforcement outside ordinary extension precedence. After resource loading, the security adapter must remain the final owner of the `bash` tool and the terminal `user_bash` handler.

The extension will reject blocked command forms before execution. It will not add user features or convenience tools.

User arguments, user resources, and project resources cannot disable, replace, or bypass enforcement for these two shell entry points.

This guarantee does not cover direct process creation inside arbitrary extension code. Such processes remain inside outer Fence filesystem, process, and network controls, but do not receive argv-aware command policy on Darwin.

The launcher will protect and revalidate all files that control this extension. Native tests will include hostile user and project extensions that try to replace both covered shell entry points.

The outer Fence boundary will still apply to native Pi file tools and all extension code.

## Linux behavior

Linux will retain the existing Fence feature preflight. The preflight must prove that the required network namespace and runtime execution controls are available.

Pi will use the same generated policy and RepoWolf environment as Claude. Pi does not need the Darwin command-policy extension on Linux.

Linux tests will cover x86-64 and ARM64.

## Darwin behavior

Darwin will use the existing ACL validation, policy generation, terminal lifecycle, and RepoWolf integration.

The mandatory Pi extension will provide shell-command enforcement. The outer Fence boundary will provide filesystem, process, and network isolation.

Darwin tests will cover Intel and Apple Silicon. The work will use the existing CI runners only.

## Error handling

If Den detects an invalid manifest, argument, path, resource, RepoWolf configuration, or Fence precondition, it will stop before Pi starts.

Errors will identify the invalid class without printing secrets or untrusted contents. Den will not print provider credentials, `auth.json`, RepoWolf tokens, or resource contents.

A pre-start error will trigger safe rollback for directories that Den created. A rollback error will change the launch result to an error.

After Pi starts, Den will return the child status. Cleanup errors for private temporary directories will not replace the child status.

Signal termination will keep the current `128 + signal` behavior.

## Test strategy

Production behavior will use test-driven development. Each behavior will start with a failing test.

The implementation will add one evidence-backed change at a time. Each change will use the smallest code required to pass its current test.

Static Nix values, hashes, workflow text, and documentation will use direct evaluation, syntax, and build validation. New tests will not restate static configuration.

### Phase 1: Agent adapter seam

Add failing tests for adapter-derived package names, manifest names, wrapper names, and main programs.

Add manifest version 2 tests before the schema changes. Add Claude regression tests before shared behavior changes.

Run focused Go tests and Nix adapter checks after each change.

### Phase 2: Pi package and module API

Add Nix checks for:

- the normal `pi` executable.
- the complete runtime closure.
- the unchanged shared Fence 0.1.58 version, source hash, Den patch hash, and required capabilities.
- `lib.${system}.mkPi`.
- `packages.${system}.pi`.
- Home Manager module behavior.
- devenv module behavior.
- disabled-by-default module behavior.
- unchanged Claude and default package outputs.

Use direct Nix evaluation and builds to validate the source, version, hashes, and module declarations.

### Phase 3: State, environment, and resources

Add focused tests for:

- agent and session path precedence.
- default Den-owned paths.
- absolute-path requirements.
- symbolic-link rejection.
- owner and mode validation.
- Linux and Darwin ACL validation.
- overlap with `.pi/agent` and host-global `.agents` under distinct invoking and runtime homes.
- canonical and symbolic-link aliases of both host homes.
- agent and session directory overlap.
- path-swap detection.
- valid interactive, RPC, and extension session switches within the selected session root.
- rejection of out-of-root, sibling-prefix, and symbolic-link session switches before file reads or extension events.
- unchanged runtime session containment after an extension changes the session-directory environment value.
- atomic rollback and commit behavior.
- environment scrubbing and replacement.
- runtime-only provider values.
- forced `PI_OFFLINE=1` and removal of inherited overrides.
- immutable package-lock behavior after extension code changes or deletes `process.env.PI_OFFLINE`.
- rejection of package mutation through exported and runtime-visible package-manager methods.
- immutable resource closure construction.
- per-type configured-package, direct-resource, user-resource, and trusted-project precedence.
- same-name winner, loser, and collision diagnostics for each resource type.
- argument and package-command rejection.
- trusted and untrusted project resources.
- prevention of runtime package resolution and installation.

Sensitive fixtures will contain dummy values only. Tests will assert that error output does not contain those dummy values.

### Phase 4: Linux native enforcement

Run native Pi fixtures on `x86_64-linux` and `aarch64-linux`.

The fixtures will prove:

- Pi starts without real provider credentials.
- the worktree and selected state directories have the intended access.
- Pi configuration, credentials, and global `.agents` resources under both host homes are inaccessible.
- distinct invoking and runtime homes, including canonical aliases, remain protected.
- unrelated host and store paths remain inaccessible.
- Fence enforces filesystem, process, command, and network policy.
- RepoWolf is the only GitHub API, Git, and SSH route.
- immutable configured resources load.
- untrusted project resources do not load.
- trusted project resources load.
- package resolution, package mutation, and argument bypass attempts stop.

The fixtures will not contact a real model provider.

### Phase 5: Darwin native enforcement

Run native Pi fixtures on `x86_64-darwin` and `aarch64-darwin`.

The fixtures will repeat the state, filesystem, network, and RepoWolf checks. They will also prove:

- the mandatory Pi security extension loads.
- Pi shell entry points use the pinned Fence 0.1.58 proxy-free command-policy helper inside the outer Fence boundary.
- the helper initializes no HTTP or SOCKS proxy listener.
- one allowed command executes exactly once, while one blocked command does not execute.
- helper failure, malformed output, command rewriting, or a missing `FENCE_SANDBOX=1` condition fails closed.
- user arguments cannot disable the extension.
- project configuration cannot replace the extension.
- user and project extensions cannot replace the protected `bash` tool or intercept `user_bash` before policy enforcement.
- direct extension process creation remains inside outer Fence filesystem, process, and network controls.
- hostile project extensions remain inside outer Fence.
- ACL and path-swap protections remain active.

These tests will use existing CI infrastructure. They will not use EC2 Mac or Terraform resources.

### Phase 6: Full regression and CI

Run these commands before completion:

```sh
go mod download
go test ./...
nix flake check --no-build
nix flake check --accept-flake-config --print-build-logs
./scripts/check-native.sh
```

If the plan identifies their output names, the implementation can add focused Nix build commands.

The existing CI matrix will remain the cross-platform acceptance authority:

- `x86_64-linux`.
- `aarch64-linux`.
- `x86_64-darwin`.
- `aarch64-darwin`.

`scripts/check-native.sh` must build the Pi package and execute the Pi native suite on every matrix system. `nix/check-support/native-enforcement.nix` must export the Pi fixture inputs and host runner entry point instead of leaving them implicit behind Claude-only inputs.

The native driver must require explicit completion markers from both the Claude and Pi suites. A missing Pi derivation, fixture, runner input, or completion marker is a failure, not a skip.

Existing Claude native fixtures must remain green without weaker assertions.

## Documentation requirements

Update `README.md` with:

- package installation.
- direct `mkPi` construction.
- Home Manager configuration.
- devenv configuration.
- agent and session directory precedence.
- credential handling.
- configured immutable resources.
- trusted project resources.
- package restrictions.
- Darwin security behavior.
- RepoWolf and Fence boundaries.

Examples must not contain credential values. The documentation must state that Fence controls network policy.

## Compatibility

This work is additive at the public API. Existing Claude users will not need a migration.

`packages.default` will continue to select Claude. `programs.den.pi.enable` will default to `false`.

The manifest change is internal. Den builds the launcher and manifest together.

Pi support will not weaken existing Claude tests or security behavior.

## Protected resources

Implementation and validation must not inspect or modify:

- `/home/roche/projects/den/.worktrees/den-claude-sandbox-publish`.
- `/home/roche/projects/den/.worktrees/den-claude-sandbox-design`.
- PR #1.
- EC2 Mac infrastructure.
- Terraform state.
- `infra/ec2-mac-debug/terraform.tfstate`.

All work will stay in `/home/roche/projects/den/.worktrees/den-pi-sandbox` on `feat/den-pi-sandbox`.

## Acceptance criteria

When all these statements are true, the design is complete:

- Den builds Pi 0.84.4 reproducibly from fixed sources.
- Users run the sandbox through the normal `pi` command.
- Pi is available through package, library, Home Manager, and devenv interfaces.
- Pi never reads or writes `.pi/agent` or host-global `.agents` under either the invoking account home or runtime home.
- Pi stores configuration, credentials, trust, and sessions in validated isolated directories, and every runtime session switch remains inside the selected session directory.
- Configured resources come from Nix store paths.
- Trusted projects can load direct repository resources.
- Runtime package resolution and installation cannot occur.
- RepoWolf remains the only GitHub API, Git, and SSH route.
- Den's unchanged, pinned, and patched Fence 0.1.58 encloses Pi and all loaded resources.
- Pi's built-in shell tool and user `!` commands use the mandatory Darwin security extension and Fence's proxy-free command-policy helper.
- Credential values do not enter derivations, logs, tests, documentation, or telemetry.
- Linux and Darwin checks pass on x86-64 and ARM64.
- Existing Claude checks remain green.
- Protected worktrees, PR #1, EC2 Mac resources, and Terraform state remain untouched.
