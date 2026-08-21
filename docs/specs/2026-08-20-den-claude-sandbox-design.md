# Den Claude Sandbox Design

**Status:** Approved for specification review

**Date:** 2026-08-20

## Summary

Den is a dendritic Nix flake and package family for sandboxed coding agents. The first release packages Claude Code behind Fence and RepoWolf.

Users run the normal `claude` command. The command starts this process chain:

```text
Den launcher -> Fence -> Claude Code
                         `-> RepoWolf gh and Git SSH clients
```

Den has no `den` executable and no runtime dispatcher. Future agent packages use their normal executable names, such as `codex` or `opencode`.

Fence is the local filesystem, process, command, and network enforcement boundary. RepoWolf is the repository authorization boundary for GitHub operations. Claude Code runs with `--dangerously-skip-permissions` because Fence supplies the permission boundary.

## Goals

The first release must:

- provide a normal `claude` executable that always runs inside Fence.
- provide the same customized Claude artifact through package, Home Manager, devenv, and library interfaces.
- preserve writable Claude state in a selected absolute configuration directory.
- preserve user-managed skills, plugins, hooks, and MCP servers from the selected Claude configuration directory.
- send GitHub API and Git traffic through RepoWolf.
- block direct GitHub and other Git-host traffic.
- support sandbox-only extra packages.
- support optional Docker and Podman client access.
- support Linux and macOS on x86-64 and ARM64.
- provide behavioral checks for the launcher, policy, modules, and package matrix.
- document the security boundary and its limits.

## Non-goals

The first release does not:

- provide or manage a RepoWolf server.
- provide a `den` dispatcher.
- support agents other than Claude Code.
- supply skills, plugins, Context Mode, CodeGraph, or other MCP servers.
- accept custom resource bundles or marketplace references.
- expose direct GitHub credentials, unrestricted `gh`, or normal Git SSH credentials.
- permit direct GitHub, GitLab, or Bitbucket network access.
- support remote Docker or Podman TCP endpoints.
- start Docker, Podman, or Podman machines.
- provide a VM-grade isolation boundary.
- make container daemon access safe.

## Flake architecture

Den uses `flake-parts` and `import-tree`. Its dendritic module layout follows the patterns in `/home/roche/projects/pi/roche-pi`.

The flake supports these systems:

- `x86_64-linux`
- `aarch64-linux`
- `x86_64-darwin`
- `aarch64-darwin`

The public outputs are:

```text
packages.<system>.claude
packages.<system>.default
lib.<system>.mkClaude
homeModules.den
devenvModules.den
checks.<system>.*
```

`packages.<system>.default` equals `packages.<system>.claude`.

`mkAgentSandbox` is the reusable internal factory. Agent adapters provide executable, argument, environment, and writable-state details. `mkClaude` is the stable public constructor for the Claude adapter. A future adapter can use the factory without adding a dispatcher.

## Main components

### `mkAgentSandbox`

`mkAgentSandbox` constructs one sandboxed agent artifact. It owns behavior that is common to all adapters:

- the Fence package and base policy.
- runtime RepoWolf validation.
- private policy generation.
- Nix closure exposure.
- `PATH` construction.
- Git transport rewriting.
- Docker and Podman capability wiring.
- argument, signal, and exit-status forwarding.
- temporary-file cleanup.

The factory accepts an adapter and user customization. The adapter supplies the underlying executable, mandatory arguments, runtime packages, and writable state paths.

The factory keeps RepoWolf shims before every other package in `PATH`. The fixed base tools include Fence, `pkgs.gitMinimal`, Bash, and Coreutils. Adapter tools follow the base tools. `extraPkgs` comes last.

### Claude adapter

The Claude adapter supplies:

- the pinned Claude Code package.
- the mandatory `--dangerously-skip-permissions` argument.
- the immutable macOS `den-fence` hook and settings artifact.
- Claude state paths that Fence can write.

The adapter passes all non-reserved user arguments unchanged and in their original order. It rejects user values for Den-owned flags: `--settings`, `--permission-mode`, and `--dangerously-skip-permissions`. This rule prevents duplicate-option precedence from changing mandatory security behavior.

Den supplies no skills, plugins, MCP servers, or non-security hooks. Its only hook is the `den-fence` security hook. User-managed skills, plugins, hooks, and MCP servers from the selected Claude configuration directory remain available. Den does not validate or package their resource definitions. They run inside Fence and remain subject to its filesystem, command, process, and network policy.

Den reserves only the `den-fence` hook identifier. The launcher fails before Fence starts when user hook configuration disables or replaces this security hook. It does not reserve plugin or MCP identifiers.

Den selects Claude's writable configuration directory with this precedence:

1. the explicit `configDir` constructor or module option.
2. inherited `CLAUDE_CONFIG_DIR`.
3. the default `~/.claude` directory.

An explicit or inherited value must be an absolute path. The launcher rejects relative values. It resolves all existing parent components before it applies ownership, permission, and overlap checks. It rejects a symbolic final component.

A missing custom directory is created with mode `0700`. An existing custom directory must be owned by the invoking user. Its owner must have read, write, and execute access, and it must grant no group or other permissions. Den fails instead of changing permissions on an existing directory.

A custom directory must have no effective ACL grant for another principal. Den inspects a new directory after creation because a parent ACL can add inherited entries. If a new directory fails this check, Den removes it and stops.

Each canonical ancestor must prevent replacement by another principal. Group, other, or ACL write access is invalid unless the ancestor is a sticky directory owned by root or the invoking user. The launcher records the custom directory's device and inode, then repeats all checks immediately before Fence starts. A changed path fails closed.

Default mode means that the fallback in step 3 selected `~/.claude`. In this mode, Fence grants write access to `~/.claude/` and the legacy state paths `~/.claude.json` and `~/.config/claude/`.

Custom mode means that the explicit option or inherited variable selected the path. Den compares canonical paths for the custom directory, the three default paths, and all credential paths denied by Fence. The custom directory must be disjoint from each protected path. It cannot equal, contain, or be contained by one. This rule rejects broad values such as `/` and `$HOME`.

In custom mode, the launcher sets `CLAUDE_CONFIG_DIR` to the canonical custom path. Fence grants Claude-state write access to that path. It also adds explicit write denials for `~/.claude/`, `~/.claude.json`, and `~/.config/claude/`. These denials take precedence over the worktree grant.

`CLAUDE_CONFIG_DIR` relocates Claude settings, session history, plugins, and other filesystem-backed state. It relocates file-backed stored credentials on Linux. It does not relocate authentication from environment variables. macOS credentials can remain in Keychain and are not isolated by this option.

Den does not replace `HOME`. It does not add skills, plugins, MCP configuration, non-security hooks, or resource runtimes to the Claude package.

### RepoWolf client

Den consumes the credential-free RepoWolf client from a pinned RepoWolf source. The client artifact contains:

- `repowolf-client`.
- `gh`, linked to `repowolf-client`.
- `repowolf-git-ssh`, linked to `repowolf-client`.

The client closure must not contain the RepoWolf service, normal GitHub CLI, OpenSSH credentials, RepoWolf service configuration, private keys, or provider credentials.

The client must build for all four Den systems. The RepoWolf source and lock file provide the immutable revision.

## Future resource bundles

The first release supplies no resource set. It has no `resourceBundles` option and no public resource-bundle constructor.

A later revision will move non-security Claude resources into separate Nix packages. Each bundle will contain plugins, skills, MCP metadata, resource hooks, and required runtime packages. Context Mode and CodeGraph will also belong to the default bundle.

The revision will add `packages.<system>.claudeResources` and `lib.<system>.mkClaudeResourceBundle`. Fence, RepoWolf, mandatory Den settings, and security hooks will remain inside Den. A resource bundle cannot replace them.

The future `mkClaude` API will accept one ordered `resourceBundles` list. Its default list will contain `packages.<system>.claudeResources`. Users will append packages to extend the defaults. Users will omit the default package to replace the resource set.

Bundles will declare plugins with marketplace references such as `context7@claude-plugins-official`. Nix inputs and lock data must pin each marketplace source. The bundle builder will produce an immutable Claude seed directory for `CLAUDE_CODE_PLUGIN_SEED_DIR`.

Den will union resource declarations across all bundles. It will deduplicate identical normalized definitions. Nix evaluation will fail when one identifier has different definitions.

Den will not run `claude plugin install` or update seed-managed resources at runtime. The later design must define the package schema, generated settings, seed precedence, source locks, runtime dependencies, and validation checks.

## Public configuration API

### Direct construction

The public constructor has this shape:

```nix
inputs.den.lib.${system}.mkClaude {
  configDir = null;
  extraPkgs = [ pkgs.neovim ];

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
}
```

All arguments are optional. `mkClaude { }` produces the same artifact as `packages.<system>.claude`.

`configDir` is either `null` or an absolute path string. Its default is `null`, which permits runtime inheritance from `CLAUDE_CONFIG_DIR` before the normal fallback. A non-null value takes precedence over the inherited variable.

For project-specific state, devenv or direnv computes an absolute value before launch. For example, a shell can export `CLAUDE_CONFIG_DIR="$PWD/.devenv/state/claude"`. The state directory must be excluded from Git because it can contain history, plugins, and credentials. Den does not resolve relative configuration paths against the working directory or Git root.

`extraPkgs` is a list of Nix packages. The constructor does not add them to a host or user package set. Their store paths remain host-visible through Nix, but Den adds them only to the sandbox launcher, sandbox `PATH`, and Fence policy. RepoWolf shims remain before them in `PATH`, even if an extra package provides `gh`, `ssh`, or Git helpers.

Each `socketPath` is either `null` or an absolute path. `null` enables runtime discovery. Each `hostPorts` value is a list of unique integers from 1 through 65535.

### Home Manager module

`homeModules.den` provides:

```nix
programs.den.claude = {
  enable = true;
  configDir = null;
  extraPkgs = [ pkgs.neovim ];

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

When enabled, the module calls `mkClaude` with the option values and adds only the resulting Claude artifact to `home.packages`.

### devenv module

`devenvModules.den` provides the same `programs.den.claude` option tree. When enabled, it calls `mkClaude` and adds the result to the devenv package list.

The two modules do not maintain separate wrapper logic. The package output, both modules, and direct construction all use the same factory and Claude adapter.

## Runtime flow

The launcher performs these steps for each invocation:

1. Read `REPOWOLF_ENDPOINT`, `REPOWOLF_TOKEN`, and `REPOWOLF_CA_FILE`.
2. Validate all three values without printing their contents.
3. Parse the endpoint as an HTTPS origin. Reject user information, non-root paths, queries, fragments, opaque URLs, and non-HTTPS schemes.
4. Require a lowercase ASCII DNS hostname without a trailing dot. Reject Unicode hostnames, IP literals, and hostnames that match a denied Git host.
5. Validate the RepoWolf token with the client-compatible `rw1_` format. Never include the token in an error or trace.
6. Inspect the CA path with `lstat`. Reject a missing path, a symbolic link, a non-regular file, or an unreadable file.
7. Convert the accepted CA path to an absolute path for the generated policy.
8. Select the Claude configuration directory from explicit `configDir`, inherited `CLAUDE_CONFIG_DIR`, or the default, in that order.
9. In default mode, resolve the normal paths for Fence without applying custom-mode overlap, ownership, mode, ACL, or final-symlink rules.
10. In custom mode, require an absolute path and apply canonical overlap, final-symlink, owner, mode, ACL, and safe-ancestor checks.
11. Create a missing custom directory with mode `0700`, inspect its effective ACL, and remove it if the privacy checks fail.
12. On macOS, inspect every Claude settings scope that applies to the launch. Reject a `den-fence` disable or replacement, and preserve unrelated user hooks.
13. Discover and validate enabled container sockets.
14. Create separate policy and scratch directories with mode `0700`. Remove inherited `TMPDIR` and `DEN_FENCE_TMPDIR`, then set both variables to the validated scratch directory.
15. On Linux, run the pinned `${fence}/bin/fence --linux-features` and require the exact `Network namespace` table row to report `ok`. Missing, unavailable, duplicate, malformed, or unknown feature output fails before policy generation.
16. Generate the Fence policy with mode `0600`, then change it to `0400` before Fence starts.
17. Add the policy file and its parent directory to Fence's highest-precedence write-deny list.
18. Export the policy path as the internal `DEN_FENCE_POLICY_FILE` variable only for the outer Fence process and mandatory macOS hook.
19. On macOS, attach the immutable `den-fence` settings artifact through the mandatory `--settings` value without editing user settings.
20. Build a controlled sandbox environment and `PATH`. Set `CLAUDE_CONFIG_DIR` when a custom directory is selected.
21. Add process-local Git configuration that rewrites GitHub HTTPS URLs to RepoWolf SSH URLs and clears credential helpers.
22. Set `GIT_SSH_COMMAND` to the immutable `repowolf-git-ssh` path.
23. Revalidate the custom directory's canonical path, device, inode, owner, mode, ACL, and ancestors immediately before Fence starts.
24. Start `${fence}/bin/fence --settings "$DEN_FENCE_POLICY_FILE" --` with the Claude command.
25. Start Claude with mandatory Den arguments and each unchanged non-reserved user argument.
26. Preserve standard input, standard output, standard error, foreground process-group state, and terminal resize behavior.
27. Forward `SIGINT`, `SIGTERM`, `SIGHUP`, and `SIGQUIT`. Preserve `SIGWINCH`, `SIGTSTP`, and `SIGCONT` job-control behavior.
28. Return the Claude or Fence exit status, including the `128 + signal` convention for signal termination.
29. Remove both private directories after Fence exits. Cleanup errors do not replace the child exit status.

The launcher replaces host `PATH` and inherited Git transport configuration. It removes `GH_TOKEN`, `GITHUB_TOKEN`, `GH_ENTERPRISE_TOKEN`, `GITHUB_ENTERPRISE_TOKEN`, `SSH_AUTH_SOCK`, `GIT_ASKPASS`, `SSH_ASKPASS`, `GIT_SSH`, `GIT_SSH_COMMAND`, `GIT_CONFIG_GLOBAL`, `GIT_CONFIG_SYSTEM`, `GIT_CONFIG_PARAMETERS`, `GIT_CONFIG_COUNT`, and inherited `GIT_CONFIG_KEY_*` and `GIT_CONFIG_VALUE_*` entries. It then installs only Den's controlled Git environment and sets `GIT_TERMINAL_PROMPT=0`.

Other normal user variables remain available, including Claude authentication, locale, terminal, and editor variables. The three RepoWolf variables remain available because RepoWolf clients require them. No other variable becomes a RepoWolf prerequisite.

The launcher must not enable shell tracing while secrets are present. Error messages can name an invalid environment variable, but cannot include its value.

`SIGKILL` cannot trigger process cleanup. On each launch, Den removes stale Den-owned temporary directories that belong to the invoking user and pass a conservative age threshold. The README must document this exception.

## RepoWolf Git routing

Den sets process-local Git configuration through Git's environment configuration interface. It does not run `git config` and does not modify global, user, worktree, or repository configuration.

The rewrite maps this prefix:

```text
https://github.com/
```

To this RepoWolf-compatible SSH prefix:

```text
git@github.com:
```

For example:

```text
https://github.com/owner/repository.git
```

becomes:

```text
git@github.com:owner/repository.git
```

Git then calls `repowolf-git-ssh` through `GIT_SSH_COMMAND`. The helper sends only supported Git smart-protocol operations to RepoWolf.

Direct SSH and HTTPS access to GitHub remain blocked by Fence. The network policy is the final control if repository configuration tries another transport.

## Fence policy

### Policy source and generation

Den vendors its static base policy in the repository. Its provenance is `sixfeetup/engineering-handbook` commit `4be05d63af92cf79231313a20df22b3c144795d0`, file `ai-tooling/sandboxing/fence.json`. The vendored Den file is the implementation input, so an external branch cannot change a build.

The Den policy starts from that snapshot. It removes unrelated AI providers and direct Git hosts. It adds the exact service, deny, and precedence rules in this specification. It does not import `jail.nix` permissions.

The initial Fence package remains nixpkgs `fence` 0.1.58 from Den's locked nixpkgs input and source revision. Den applies only `patches/fence-0.1.58-den-tmpdir.patch` to upstream `internal/sandbox/utils.go`, with minimal upstream-level tests carried by that patch. The patch makes `GenerateProxyEnvVars` honor Den's validated `DEN_FENCE_TMPDIR` scratch path for both outer Fence and nested `fence -c`. An absent variable preserves upstream behavior. A present value must name an absolute, clean, existing, non-symbolic directory; invalid values resolve to a fixed nonexistent fail-closed path and never to `/tmp`.

The package must provide `--settings`, `--claude-pre-tool-use`, `fence -c`, `--linux-features`, `--expose-host-path`, Linux `command.runtimeExecPolicy = "argv"`, strict read fields, macOS `network.allowUnixSockets`, Linux `network.allowLocalOutboundPorts`, and the Den TMPDIR patch. Evaluation fails for an unknown version, capability, patch count, or patch hash.

Each launch adds only runtime values that cannot exist in the Nix store policy:

- the hostname from `REPOWOLF_ENDPOINT`.
- the accepted CA file path.
- validated container socket paths.
- requested host ports.

The generated file never contains `REPOWOLF_TOKEN`.

### Network policy

The policy denies unmatched outbound traffic by default. Den permits only these static service domains:

- `api.anthropic.com`, `*.anthropic.com`, `claude.ai`, and `*.claude.ai`.
- `registry.npmjs.org` and `*.npmjs.org`.
- `registry.yarnpkg.com`.
- `pypi.org` and `files.pythonhosted.org`.
- `crates.io`, `static.crates.io`, and `index.crates.io`.
- `proxy.golang.org` and `sum.golang.org`.
- `formulae.brew.sh`.

Each launch also permits the exact RepoWolf broker hostname.

The deny list contains `github.com`, `*.github.com`, `githubusercontent.com`, `*.githubusercontent.com`, `gitlab.com`, `*.gitlab.com`, `bitbucket.org`, and `*.bitbucket.org`. These entries block web, API, raw content, archive, object, release-asset, and SSH endpoints.

Denied domains take precedence over allowed domains. The launcher rejects a RepoWolf endpoint whose hostname matches a denied Git-host entry. Den does not allow a wildcard that re-enables a denied Git host.

Metadata endpoints and denied telemetry from the reference policy remain blocked, including `169.254.169.254`, `metadata.google.internal`, `instance-data.ec2.internal`, and `statsig.anthropic.com`.

### Filesystem policy

The normal policy grants:

- read and execute access to required Nix store closures.
- write access to the launch working tree.
- write access to the separate Den scratch directory.
- write access to the selected Claude configuration directory and only the applicable legacy paths.
- custom-mode write denials for all three default Claude paths.
- read-only access to the validated CA file and policy file.
- an explicit write denial for the policy file and its parent directory.
- no general access to credential directories.

The policy retains explicit read denials for SSH private keys, GnuPG, cloud credentials, Kubernetes credentials, and Docker credentials. It also denies package registry credentials, netrc files, and Git credential stores.

Filesystem denials take precedence over grants. Den resolves each dynamic path before policy generation. A symbolic link under the worktree, selected Claude configuration directory, scratch directory, or socket path does not grant access to a denied target. The launcher rejects a dynamic path when safe resolution cannot prove its target.

Nix store closures are read-only. `extraPkgs` closures are executable.

### Command policy and macOS hook

Linux uses Fence's argv-aware descendant command policy when the host supports it. Before policy generation, Den runs the pinned Fence feature probe and requires the `Network namespace` status to be exactly `ok`; unknown, malformed, missing, duplicate, unavailable, and unsupported results fail closed rather than accepting Fence's direct-network fallback. Fence also fails closed when it cannot apply the configured argv policy.

macOS uses Fence's whole-process `sandbox-exec` boundary for filesystem and network enforcement. macOS cannot apply argv-aware multi-token command rules to every descendant process.

The Claude adapter therefore loads a store-managed `PreToolUse` hook for `Bash` on macOS. Its command runs `${fence}/bin/fence --claude-pre-tool-use --settings "$DEN_FENCE_POLICY_FILE"`. It denies blocked commands and rewrites allowed Bash commands through `fence -c` with that policy.

The hook does not replace whole-agent wrapping. It strengthens command checks for Claude Bash tool calls. It does not inspect Claude's native file tools, which remain inside the outer Fence boundary.

Den loads the hook declaratively from the store. It does not run `fence hooks install` and does not edit user settings. User-managed hooks remain available, but they cannot disable or replace `den-fence`.

## Docker support

Docker support is disabled by default.

When enabled, Den adds the configured Docker client and Compose packages to the sandbox closure. It never exposes the unrestricted host versions through path fallback.

Socket selection uses this order:

1. `docker.socketPath`, when configured.
2. a valid Unix `DOCKER_HOST` value.
3. a rootless socket at `$XDG_RUNTIME_DIR/docker.sock`.
4. `$HOME/.docker/run/docker.sock` for Docker Desktop.
5. `/run/docker.sock` or `/var/run/docker.sock`.

`docker.socketPath` accepts only an absolute filesystem path. `DOCKER_HOST` accepts only `unix:///absolute/path` with no authority, query, fragment, or encoded path separator. Den rejects TCP, SSH, HTTP, `npipe`, relative, and malformed endpoints.

The selected path must resolve to a Unix socket. Den resolves symbolic links for explicit, environment, and discovered paths. It exposes only the final socket path and updates `DOCKER_HOST` to `unix:///final/absolute/path`.

Fence grants read-write access only to the selected socket. On macOS, the generated network policy also lists that socket in `network.allowUnixSockets`.

Den forwards or sets `DOCKER_HOST` to the validated Unix endpoint. It does not expose `~/.docker` or Docker credential files.

## Podman support

Podman support is disabled by default.

When enabled, Den adds the configured Podman and Podman Compose packages to the sandbox closure.

Socket selection uses this order:

1. `podman.socketPath`, when configured.
2. a valid Unix `CONTAINER_HOST` value.
3. `$XDG_RUNTIME_DIR/podman/podman.sock`.
4. `/run/user/<uid>/podman/podman.sock`.

On Linux, Den sets `XDG_RUNTIME_DIR` to `/run/user/<uid>` when the variable is unset. The selected Podman socket must be owned by the invoking user and must be a Unix socket. This keeps the default path rootless.

On macOS, the user must provide `CONTAINER_HOST` or `podman.socketPath` when no conventional runtime socket exists. Den does not start or inspect a Podman machine.

`podman.socketPath` accepts only an absolute filesystem path. `CONTAINER_HOST` accepts only `unix:///absolute/path` with no authority, query, fragment, or encoded path separator. Den rejects remote, relative, and malformed endpoints.

Den resolves symbolic links for explicit, environment, and discovered paths. It exposes only the final socket path. Den sets `CONTAINER_HOST` to `unix:///final/absolute/path` and forwards the accepted `XDG_RUNTIME_DIR` value.

Fence grants read-write access only to the selected socket. On macOS, the generated network policy also lists that socket in `network.allowUnixSockets`.

## Container host-port access

Each enabled container option can list `hostPorts`. Den combines and deduplicates the Docker and Podman lists.

An empty list grants no host-loopback access. A non-empty list enables Fence local outbound access.

On Linux, Den writes the exact combined ports to `network.allowLocalOutboundPorts`. Fence bridges only those host-loopback ports.

On macOS, Fence ignores `allowLocalOutboundPorts` and permits all localhost ports when local outbound access is enabled. Module documentation and the README must show this platform limitation next to the option.

A non-empty `hostPorts` list is invalid when its parent container integration is disabled.

## Container security warning

**WARNING:** Enable Docker or Podman socket access only for trusted work. A daemon socket can create containers, mount host paths, and reach resources outside Fence.

Docker daemon access can provide effective host-root control. Rootless Podman still gives broad control over the user's containers, files, and network. Either socket can materially weaken or bypass the sandbox.

Host-port access also expands the network boundary. On macOS, one requested port expands access to all host-loopback ports because of the Fence platform model.

## Error handling

The launcher fails before Claude starts for these conditions:

- a required RepoWolf variable is missing or empty.
- the endpoint is not an HTTPS origin.
- the broker hostname is noncanonical, is an IP literal, or matches a denied Git host.
- a user argument conflicts with a Den-owned security flag.
- a user hook configuration disables or replaces `den-fence`.
- an explicit or inherited Claude configuration directory is relative.
- a custom configuration path overlaps a default Claude path or denied credential path.
- a custom configuration path has a symbolic final component.
- an existing custom directory has the wrong owner, lacks owner access, grants group or other permissions, or has a non-owner ACL grant.
- a custom directory has an ancestor that another principal can replace.
- the custom directory changes identity before Fence starts.
- the selected Claude configuration path cannot be resolved safely, created, or used as a directory.
- the token format is invalid.
- the CA path is missing, unreadable, symbolic, or not a regular file.
- a configured socket path is not absolute.
- an endpoint names a non-Unix container transport.
- an enabled container socket cannot be found or is not a socket.
- a Podman socket is not owned by the invoking user.
- a host port is invalid.
- private policy creation fails.
- Fence cannot apply the policy.

Each error names the failed requirement and a corrective action. Errors do not print environment values, tokens, file contents, or generated environment configuration.

Build-time checks fail for missing required packages, duplicate output names, unsupported native packages, or unexpected closure contents.

Fence or Claude runtime errors retain their original standard error and exit status. Cleanup uses best effort and does not hide the primary result.

## Platform behavior

| Platform | Fence behavior | Additional requirements |
|---|---|---|
| `x86_64-linux` | Whole-agent Linux sandbox with argv-aware command policy and required network namespace. | Package Claude, patched Fence 0.1.58, and RepoWolf clients; require the live Fence feature probe to report `Network namespace` as `ok`. |
| `aarch64-linux` | Whole-agent Linux sandbox with argv-aware command policy and required network namespace. | Package Claude, patched Fence 0.1.58, and RepoWolf clients; require the live Fence feature probe to report `Network namespace` as `ok`. |
| `x86_64-darwin` | Whole-agent macOS sandbox plus Claude Bash hook. | Package Claude, Fence, and RepoWolf clients. Document localhost widening. |
| `aarch64-darwin` | Whole-agent macOS sandbox plus Claude Bash hook. | Package Claude, Fence, and RepoWolf clients. Document localhost widening. |

A platform output must not silently omit Claude, Fence, RepoWolf, or the macOS `den-fence` hook.

## Documentation requirements

`README.md` must contain:

- the architecture and the Fence and RepoWolf boundaries.
- a warning that Fence is not a VM-grade boundary.
- all four supported platforms.
- RepoWolf server prerequisites.
- `REPOWOLF_ENDPOINT`, `REPOWOLF_TOKEN`, and `REPOWOLF_CA_FILE` requirements.
- package, Home Manager, devenv, and direct-library examples.
- `configDir` precedence, absolute-path requirement, default mode, custom mode, ownership, mode, ACL, ancestor, symlink, overlap, and revalidation rules.
- a project-specific example that supplies an absolute ignored state path through devenv or direnv.
- the macOS Keychain limitation for configuration-directory isolation.
- `extraPkgs` behavior and path precedence.
- Docker options, socket discovery, and the daemon warning.
- Podman options, rootless defaults, and the daemon warning.
- host-port behavior on Linux and macOS.
- normal `claude` usage, non-reserved argument forwarding, and reserved Den flags.
- inherited environment scrubbing and protected configuration precedence.
- signal, terminal, exit-status, cleanup, and `SIGKILL` behavior.
- the protected macOS `den-fence` hook and its coverage limits.
- `configDir` precedence, absolute-path validation, and writable Claude state.
- troubleshooting for RepoWolf variables, CA files, custom-directory privacy checks, sockets, and Fence errors.
- limitations, including blocked direct Git hosts and unsupported remote container endpoints.

Examples must use `claude`, not a Den-specific command.

## Test plan

### Pure launcher and policy checks

Automated tests must cover:

- missing and empty RepoWolf variables.
- explicit `configDir`, inherited `CLAUDE_CONFIG_DIR`, and fallback precedence.
- rejection of relative explicit and inherited configuration paths.
- missing custom-directory creation with exact mode `0700`.
- existing custom-directory owner, owner-access, group-permission, other-permission, ACL, non-directory, and unwritable cases.
- inherited ACL detection after creation on platforms that support ACLs.
- canonical parent components and rejection of a symbolic final component.
- safe and unsafe ancestor mode, ACL, ownership, and sticky-directory cases.
- device and inode replacement between initial validation and the final pre-Fence check.
- a canonical overlap matrix for `/`, `$HOME`, all three default paths, descendants, ancestors, denied credential paths, and disjoint paths.
- default-mode grants for the three normal Claude paths.
- custom-mode grants for only the selected Claude configuration directory.
- custom-mode default-path denials when the independent worktree grant contains those paths.
- accepted and rejected endpoint forms, including uppercase, trailing-dot, Unicode, IP-literal, and denied Git-host cases.
- token format validation without value disclosure.
- regular, unreadable, missing, directory, and symbolic CA paths.
- separate policy and scratch directories, policy mode `0400`, and write-deny precedence for the policy and its parent.
- inherited `TMPDIR` and `DEN_FENCE_TMPDIR` removal, followed by both variables being set to the validated scratch directory.
- Fence 0.1.58 patch behavior for absent, valid, relative, missing, non-directory, and symbolic `DEN_FENCE_TMPDIR` values.
- identical effective TMPDIR behavior for outer Fence and nested `fence -c`.
- exact Linux `Network namespace` parsing and rejection of missing, unavailable, duplicate, malformed, and unknown feature output before policy generation.
- generated JSON syntax.
- exact dynamic broker hostname insertion.
- absence of the token from policy and diagnostics.
- allowed Anthropic and registry hosts.
- denied GitHub, GitLab, Bitbucket, metadata, and telemetry hosts.
- deny precedence and no broad Git-host wildcard.
- GitHub HTTPS rewrite configuration.
- `pkgs.gitMinimal`, `GIT_SSH_COMMAND`, cleared credential helpers, and RepoWolf path precedence.
- fake clone, fetch, and push flows that invoke `repowolf-git-ssh` without configuration changes.
- removal of inherited GitHub tokens, SSH agents, askpass values, and Git environment configuration.
- seeded `GIT_CONFIG_PARAMETERS` cases that contain credential-helper injection and embedded credential data.
- continued availability of the three RepoWolf variables and normal Claude authentication variables.
- unchanged non-reserved user arguments, including spaces and empty arguments.
- acceptance of ordinary user `--plugin-dir`, `--mcp-config`, and `--strict-mcp-config` arguments.
- acceptance of ordinary user-managed skills, plugins, hooks, and MCP configuration from the selected Claude configuration directory.
- rejection of the three Den-owned security flags and attempts to disable or replace `den-fence`.
- standard input, standard output, standard error, PTY, and resize forwarding.
- normal, nonzero, and signaled child exits.
- `SIGINT`, `SIGTERM`, `SIGHUP`, `SIGQUIT`, `SIGWINCH`, `SIGTSTP`, and `SIGCONT` behavior.
- temporary directory cleanup after success, error, and catchable signal exit.
- stale temporary directory cleanup after simulated `SIGKILL` termination.
- no user or repository Git configuration changes.

Tests use fake RepoWolf, Fence, and Claude executables when they need deterministic process observations. Tests use valid fake tokens and never use live credentials.

### Native Fence and RepoWolf enforcement checks

Host-level integration jobs must use the packaged Fence executable. They must verify:

- an allowed local TLS broker fixture is reachable through the generated broker rule.
- hosts mapped to GitHub, GitLab, and Bitbucket deny rules remain unreachable without live external requests.
- a representative allowed registry hostname reaches a local fixture.
- a denied credential path remains unreadable through a worktree or Claude-state symbolic link.
- custom-mode default paths remain unwritable when the test worktree is also the temporary home directory.
- Linux POSIX ACL and macOS ACL grants to another principal cause custom-directory validation to fail.
- a replaceable custom-directory ancestor and a validation-time path swap both fail closed.
- the policy file cannot be truncated, replaced, renamed, or made writable from inside Fence.
- a later macOS Bash hook still denies a command after each policy mutation attempt.
- Linux argv command rules deny descendant commands with multiple tokens.
- the macOS Claude hook reroutes allowed Bash commands and denies blocked commands.
- a user-managed plugin or MCP server remains subject to Fence filesystem and network denials.
- RepoWolf `gh` and Git operations reach a local RepoWolf protocol fixture and never a provider endpoint.

These jobs run outside a nested Nix build sandbox when Fence requires host namespace or `sandbox-exec` features. Fixtures remain local and credential-free.

### Package and closure checks

Nix checks must verify:

- presence of the store-managed macOS `den-fence` hook and settings artifact.
- RepoWolf `gh` and `repowolf-git-ssh` links.
- absence of normal `gh`, RepoWolf server, credentials, and private keys from the client closure.
- inclusion of Fence 0.1.58 capabilities, `pkgs.gitMinimal`, Claude, and required base runtimes.

### Claude startup checks

Temporary-home startup checks must cover both state modes.

The default-mode check must:

1. create an empty writable home directory.
2. leave `configDir` and `CLAUDE_CONFIG_DIR` unset.
3. provide only the three required RepoWolf variables, with a fake valid token and regular CA file.
4. start the packaged `claude` through its normal wrapper.
5. confirm that Claude can write the normal state paths.

The custom-mode checks must:

1. create one ignored project-state directory inside the worktree.
2. create a second directory outside both the temporary home and worktree.
3. select each path through `configDir`, then repeat through inherited `CLAUDE_CONFIG_DIR`.
4. confirm that each created directory has mode `0700` and the correct owner.
5. confirm that Claude can write the selected directory.
6. confirm that Fence does not grant writes to `~/.claude/`, `~/.claude.json`, or `~/.config/claude/`.
7. repeat canonical overlap and final-symlink cases against both path layouts.

Both checks must avoid live Anthropic or GitHub traffic. A fixture can add user-managed skill, plugin, hook, and MCP configuration to the selected directory. The wrapper must not reject, alter, package, or validate those resources. A full prompt can stop only at the expected offline provider or authentication boundary.

A check fails on missing required base packages, state-path, policy, or store-write errors.

### Module and API checks

Evaluation checks must verify:

- `packages.<system>.default == packages.<system>.claude`.
- no `packages.<system>.den` output and no `bin/den` executable in any closure.
- `lib.<system>.mkClaude { }` matches the default package behavior.
- `configDir` defaults to `null` in the constructor and both modules.
- explicit constructor and module values require absolute path strings.
- runtime precedence between the explicit option, inherited variable, and fallback.
- Home Manager enable and disable behavior.
- devenv enable and disable behavior.
- both modules call the same constructor.
- `extraPkgs` appears only in the sandbox closure and after RepoWolf shims.
- custom Docker and Compose packages.
- custom Podman and Podman Compose packages.
- explicit and discovered socket wiring.
- rejection of invalid socket and host-port values.
- no container capability when its integration is disabled.

### Container behavior checks

Behavioral tests must create temporary Unix sockets and verify:

- Docker and Podman are disabled by default.
- Docker discovery order and `DOCKER_HOST` forwarding.
- Podman rootless discovery, ownership, `CONTAINER_HOST`, and `XDG_RUNTIME_DIR` forwarding.
- non-Unix endpoint rejection.
- missing and non-socket path rejection.
- symbolic socket resolution to one exact target.
- one read-write Fence socket permission and no parent-directory grant.
- macOS `allowUnixSockets` generation.
- Linux exact host-port generation.
- empty host-port behavior.
- the documented macOS localhost widening.

Tests do not need a live Docker or Podman daemon. Socket and policy behavior provides stable regression coverage without granting test builds daemon control.

### Four-system matrix

CI must evaluate every output for all four systems. It must provide four required native jobs: `x86_64-linux`, `aarch64-linux`, `x86_64-darwin`, and `aarch64-darwin`. A missing native runner fails the matrix instead of skipping that system.

Each native job must build at least:

- `packages.<system>.claude`.
- the RepoWolf client.
- module evaluation checks.
- pure behavioral checks.
- the platform's native Fence and RepoWolf enforcement checks.

Cross-evaluation does not replace any native job. Platform-specific Fence behavior requires native execution on every named system.

## Acceptance criteria

The design is complete when an implementation can demonstrate all of these results:

1. `nix run .#claude -- --version` invokes Claude through Fence.
2. The package is available as `claude` through every approved integration surface.
3. RepoWolf is the only route for GitHub API and Git traffic.
4. Direct Git-host traffic is denied.
5. On macOS, `den-fence` remains mandatory while unrelated user hooks remain available.
6. Filesystem-backed Claude state remains writable only in the selected directory and applicable default-mode legacy paths. Environment authentication is unchanged. macOS Keychain credentials remain outside this isolation.
7. `extraPkgs` is absent from host package sets and host `PATH`, enters only the sandbox launch environment, and cannot shadow RepoWolf clients.
8. Docker and Podman are disabled by default and expose only validated sockets when enabled.
9. Container host-port access follows the documented platform rules.
10. Non-reserved arguments, terminal state, supported signals, exit statuses, and documented cleanup behavior remain transparent.
11. The full check suite passes in the four-system matrix.
12. The README documents setup, operation, security warnings, troubleshooting, and limits.
