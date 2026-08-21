# Den Claude Sandbox MVP Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the resource-free Den MVP: a normal `claude` executable that always runs Claude Code through Fence, routes GitHub API and Git traffic only through RepoWolf, supports isolated Claude state and optional local Docker or Podman sockets, and exposes the same artifact through package, library, Home Manager, and devenv interfaces on four native systems.

**Architecture:** Follow the Roche Pi dendritic pattern: `flake.nix` delegates to `flake-parts` and `import-tree`, `modules/` declares outputs, and focused files under `nix/` build private implementation artifacts. A small, standard-library Go launcher performs strict runtime validation, creates a private dynamic Fence policy from a vendored static policy, starts Fence and Claude, and preserves process behavior. Nix supplies immutable Claude, Fence, RepoWolf client, security-hook, manifest, and closure paths; only `mkClaude` is public.

**Tech Stack:** Nix flakes, `flake-parts`, `import-tree`, pinned nixpkgs Fence 0.1.58 and Claude Code 2.1.158, pinned RepoWolf source, Go standard library, Fence JSON policy, Home Manager, devenv, GitHub Actions.

## Global Constraints

- Support exactly `x86_64-linux`, `aarch64-linux`, `x86_64-darwin`, and `aarch64-darwin`; all four require native checks.
- Expose `packages.<system>.claude`, `packages.<system>.default`, `lib.<system>.mkClaude`, `homeModules.den`, `devenvModules.den`, and `checks.<system>.*`.
- `packages.<system>.default` must be the same derivation as `packages.<system>.claude`.
- Do not expose `packages.<system>.den`, install `bin/den`, or add a runtime dispatcher.
- The public executable is `claude`; every invocation must follow `Den launcher -> Fence -> Claude Code`.
- Run Claude with Den-owned `--dangerously-skip-permissions`; reject user `--settings`, `--permission-mode`, and `--dangerously-skip-permissions` forms before Fence starts. On Darwin also reject `--bare` and remove inherited `CLAUDE_CODE_SIMPLE`, because Claude Code 2.1.158 uses either mechanism to skip the mandatory hook.
- RepoWolf is the only GitHub API and Git route. Do not expose normal `gh`, OpenSSH credentials, direct GitHub credentials, or provider endpoints.
- Preserve the MVP resource boundary: do not add supplied skills, plugins, Context Mode, CodeGraph, other MCP servers, non-security hooks, resource packages, `resourceBundles`, `claudeResources`, or `mkClaudeResourceBundle`.
- Permit user-managed skills, plugins, hooks, and MCP servers from the selected Claude configuration directory; do not package, rewrite, or validate those resources except to protect the mandatory macOS `den-fence` hook.
- Keep future marketplace resource bundles out of implementation tasks. They remain design notes for a later revision.
- `configDir` precedence is explicit option, inherited `CLAUDE_CONFIG_DIR`, then `~/.claude`; explicit and inherited custom paths must be absolute.
- In custom mode, protect default Claude paths and enforce canonical disjointness, final-component symlink rejection, owner-only access, ACL privacy, safe ancestors, and device/inode revalidation.
- Keep RepoWolf shims first in sandbox `PATH`; fixed tools precede adapter tools, `extraPkgs`, and optional container clients.
- Docker and Podman are disabled by default. Support local Unix sockets only; never start a daemon or Podman machine and never accept a remote endpoint.
- Treat daemon sockets and macOS host-port widening as documented sandbox escapes, not as safe isolation.
- Use automated tests for launcher, parsing, validation, policy, process, module, API, closure, and security behavior. Use direct syntax/evaluation checks, not bespoke tests, for lock data, workflow YAML, and documentation text.
- Each implementation task must use `superpowers:test-driven-development` where behavior changes and `superpowers:verification-before-completion` before its commit.
- Keep one writer in the worktree. Run review after each task-sized commit; do not combine unrelated tasks.
- Git-backed flakes omit untracked files. After the focused RED test and before each listed `nix build`, `nix eval`, or `nix flake check`, stage only that task's intended files with `git add` (or use `path:$PWD` for a deliberately pre-stage probe), inspect `git diff --cached`, then run the normal flake command. The task's commit step may repeat the same `git add` safely.

## Plan-Review Compatibility Note

The approved specification's reserved-flag list predates Claude Code 2.1.158's verified `--bare` behavior. That flag sets `CLAUDE_CODE_SIMPLE=1` and skips hooks, which conflicts with the higher-priority requirement that macOS `den-fence` remain mandatory. This plan therefore treats `--bare` and inherited `CLAUDE_CODE_SIMPLE` as Darwin hook-disable attempts. Before Task 8 implementation, record this compatibility erratum in the design specification in a separate approved documentation commit; the current task commits only this plan as requested.

Fence 0.1.58 also falls back to direct Linux networking when network namespaces are unavailable, supplies implicit filesystem defaults, and overwrites Darwin child `TMPDIR` with shared `/tmp/fence`. Tasks 6, 9, and 13 counter those pinned-version behaviors with a fail-closed feature preflight, strict read mode, explicit read/write sets, implicit-host-write denials, and effective packaged-Fence tests. Task 6 applies one reviewed local patch to the locked nixpkgs Fence 0.1.58 source so outer Fence and nested `fence -c` preserve Den's validated per-launch scratch directory through `DEN_FENCE_TMPDIR`; it does not change Fence version or source revision. This patch requires the explicit design-erratum approval gate in Task 6 before implementation.

## Implementation Map

| Area | Files | Responsibility |
|---|---|---|
| Flake entry | `flake.nix`, `flake.lock`, `modules/parts.nix` | Lock inputs, import the dendritic tree, and declare four systems. |
| Launcher entry and manifest | `go.mod`, `cmd/den-launcher/main.go`, `internal/manifest/manifest.go`, `internal/launch/launch.go` | Load one versioned immutable manifest and execute the ordered runtime flow. |
| Runtime validation | `internal/repowolf/config.go`, `internal/environment/environment.go`, `internal/configdir/*`, `internal/container/socket.go`, `internal/claude/settings.go` | Validate untrusted arguments, environment, paths, ACLs, and sockets before Fence starts. |
| Policy and lifecycle | `internal/policy/policy.go`, `internal/process/process.go`, `internal/tempdir/tempdir.go`, `policy/fence.json`, `policy/README.md` | Build the private policy, preserve terminal/signal/exit behavior, and clean temporary state. |
| Private Nix implementation | `nix/packages/den-launcher.nix`, `nix/packages/repowolf-client.nix`, `nix/lib/fence.nix`, `nix/lib/options.nix`, `nix/lib/mk-agent-sandbox.nix`, `nix/lib/mk-claude.nix` | Build private binaries, require Fence capabilities, normalize options, construct the adapter, and generate the wrapper and manifest. |
| Public outputs | `modules/lib/claude.nix`, `modules/packages/claude.nix`, `modules/home/den.nix`, `modules/devenv/den.nix`, `nix/lib/module-options.nix` | Export the constructor, package, and shared module option tree. |
| Checks | `modules/checks/*.nix`, `nix/check-support/*.nix`, `internal/**/*_test.go`, `tests/native/*` | Unit, pure behavioral, closure, startup, module, and native enforcement checks. |
| Operations | `README.md`, `scripts/check-native.sh`, `.github/workflows/checks.yml` | Explain use and limits and enforce the four native jobs. |

The launcher manifest is the internal boundary between Nix and Go. Use version `1` and keep this shape stable across tasks:

```go
type Manifest struct {
    Version          int             `json:"version"`
    Platform         string          `json:"platform"`
    FenceExecutable  string          `json:"fenceExecutable"`
    RepoWolfClientDir string          `json:"repoWolfClientDir"`
    BasePolicy       string          `json:"basePolicy"`
    ClosurePathsFile string          `json:"closurePathsFile"`
    ScratchRoot      string          `json:"scratchRoot"`
    ACLProbe         []string        `json:"aclProbe"`
    ProtectedPathPatterns []string   `json:"protectedPathPatterns"`
    PathEntries      []string        `json:"pathEntries"`
    ExplicitConfigDir *string        `json:"explicitConfigDir"`
    Agent            Agent           `json:"agent"`
    Docker           ContainerConfig `json:"docker"`
    Podman           ContainerConfig `json:"podman"`
}

type Agent struct {
    Name              string   `json:"name"`
    Executable        string   `json:"executable"`
    MandatoryArgs     []string `json:"mandatoryArgs"`
    ReservedFlags     []string `json:"reservedFlags"`
    ConfigEnvironment string   `json:"configEnvironment"`
    DefaultStatePaths []string `json:"defaultStatePaths"`
    DarwinSettings    string   `json:"darwinSettings,omitempty"`
}

type ContainerConfig struct {
    Enable         bool     `json:"enable"`
    SocketPath     *string  `json:"socketPath"`
    HostPorts      []uint16 `json:"hostPorts"`
    ClientPrograms []string `json:"clientPrograms"`
}
```

`nil` `ExplicitConfigDir` means runtime inheritance is allowed. Public `mkClaude` and module values reject a relative explicit path during Nix evaluation. Raw internal manifest tests may still pass a non-`nil` invalid value to prove the launcher's defense-in-depth runtime rejection and parity with inherited values.

---

### Task 1: Bootstrap and Lock the Four-System Dendritic Flake

**Files:**
- Create: `flake.nix`
- Create: `flake.lock`
- Create: `modules/parts.nix`

**Interfaces:**
- Produces: `inputs.nixpkgs`, `inputs.flake-parts`, `inputs.import-tree`, `inputs.repowolf`, `inputs.home-manager`, and `inputs.devenv` for later modules.
- Produces: the four-system `perSystem` matrix consumed by every package and check task.

- [ ] **Step 1: Write the minimal dendritic flake**

Follow `/home/roche/projects/pi/roche-pi/flake.nix`: declare the inputs and make the output expression only this delegation:

```nix
outputs = inputs:
  inputs.flake-parts.lib.mkFlake { inherit inputs; }
    (inputs.import-tree ./modules);
```

Set `inputs.nixpkgs.url = "github:NixOS/nixpkgs/c94da05fe469a845461ae503894fad568abeb2a6"`. This verified revision supplies Fence 0.1.58 and Claude Code 2.1.158 for all four target systems. Make Home Manager and devenv follow that `nixpkgs`, and set `inputs.repowolf.url = "github:rochecompaan/repowolf"`. The lock file, not a mutable runtime clone, pins each revision. Do not add `llm-agents.nix` or resource-related inputs.

- [ ] **Step 2: Declare the supported systems**

Write `modules/parts.nix` with:

```nix
{ inputs, ... }:
{
  systems = [
    "x86_64-linux"
    "aarch64-linux"
    "x86_64-darwin"
    "aarch64-darwin"
  ];

  perSystem = { system, ... }: {
    _module.args.pkgs = import inputs.nixpkgs {
      inherit system;
      config.allowUnfreePredicate = pkg:
        inputs.nixpkgs.lib.getName pkg == "claude-code";
    };
  };
}
```

Permit only the pinned unfree Claude package; do not set broad `allowUnfree = true`.

- [ ] **Step 3: Generate and inspect the lock**

Git-backed flake commands cannot see an untracked `flake.nix`. Stage the two authored inputs before locking, then stage the generated lock before metadata/evaluation:

```bash
git add flake.nix modules/parts.nix
nix flake lock
git add flake.lock
git diff --cached --check
nix flake metadata --json >/dev/null
```

Expected: all commands succeed; `flake.lock` pins all six inputs and their transitive inputs, and only the three Task 1 files are staged.

- [ ] **Step 4: Verify required upstream artifacts before building on them**

For each system, evaluate the selected nixpkgs Fence version, Claude Code version/path, and the pinned RepoWolf source. Use `builtins.getFlake (toString ./.)` so this check does not depend on outputs that later tasks have not created.

Run:

```bash
for system in x86_64-linux aarch64-linux x86_64-darwin aarch64-darwin; do
  nix eval --impure --raw --expr "
    let
      f = builtins.getFlake (toString ./.);
      pkgs = import f.inputs.nixpkgs {
        system = \"$system\";
        config.allowUnfreePredicate = pkg:
          f.inputs.nixpkgs.lib.getName pkg == \"claude-code\";
      };
    in
      if pkgs.fence.version != \"0.1.58\" then
        throw \"Den requires Fence 0.1.58\"
      else if pkgs.claude-code.version != \"2.1.158\" then
        throw \"Den requires Claude Code 2.1.158\"
      else
        pkgs.claude-code.outPath
  "
done
nix eval --impure --raw --expr '
  let f = builtins.getFlake (toString ./.); in f.inputs.repowolf.outPath
'
```

Expected: the four Claude store paths evaluate, and both version guards pass on every system. The fixed nixpkgs revision must remain unchanged unless a separate approved design update changes the Fence or Claude version.

- [ ] **Step 5: Record the pinned dependency security contract**

Inspect the exact locked sources/binaries before writing launcher code. Verify and record these known 0.1.58/2.1.158 contracts in the Task 1 review notes:

```text
Fence schema: defaultDenyRead, strictDenyRead, runtimeExecPolicy,
              allowUnixSockets, allowLocalOutboundPorts
Fence CLI:    --settings, --claude-pre-tool-use, -c, --linux-features,
              --expose-host-path
Fence Linux:  network namespace may report unavailable and otherwise falls back
Fence FS:     default read/write paths exist and must be countered/tested
Claude CLI:   --bare sets CLAUDE_CODE_SIMPLE=1 and skips hooks
RepoWolf:     client also reads REPOWOLF_SERVER_NAME unless Den removes it
```

Run the native pinned `fence --help`, `fence --linux-features`, and `claude --help` where executable, and inspect the locked source for every non-native system. Any drift from this contract blocks implementation and requires a reviewed plan/pin update.

- [ ] **Step 6: Directly verify the static flake setup**

Run:

```bash
nix flake check --no-build
```

Expected: evaluation succeeds with no missing module or unsupported-system error. No new automated test is needed for the lock or static system list; later output and native checks prove behavior.

- [ ] **Step 7: Commit the bootstrap**

```bash
git add flake.nix flake.lock modules/parts.nix
git commit -m "chore: bootstrap dendritic flake"
```

---

### Task 2: Add the Versioned Launcher Manifest and Unit-Test Check

**Files:**
- Create: `go.mod`
- Create: `cmd/den-launcher/main.go`
- Create: `internal/manifest/manifest.go`
- Create: `internal/manifest/manifest_test.go`
- Create: `internal/launch/launch.go`
- Create: `nix/packages/den-launcher.nix`
- Create: `modules/checks/launcher-unit.nix`

**Interfaces:**
- Produces: `manifest.Load(path string) (manifest.Manifest, error)`.
- Produces: `launch.Run(context.Context, manifest.Manifest, []string) int` as the single orchestration entry.
- Produces: a private `den-launcher` derivation; it is not added to public `packages`.

- [ ] **Step 1: Write failing manifest tests**

Cover one valid version-1 manifest and table cases for unknown version, empty Fence path, empty RepoWolf client path, empty policy path, empty closure file, unsupported platform, missing ACL probe, empty `PathEntries`, non-absolute immutable paths, an empty or unsafe agent program name, and malformed container entries. Validate only `ACLProbe[0]` as an absolute executable path; validate later argv elements as non-empty, NUL-free arguments such as `-lde`. Accept a safe non-Claude agent name at this internal boundary so `mkAgentSandbox` remains reusable; the MVP exports only the Claude adapter. Assert errors name the field but do not echo values.

```go
func TestLoadRejectsUnknownVersion(t *testing.T) {
    path := writeManifest(t, `{"version":2}`)
    _, err := Load(path)
    if err == nil || !strings.Contains(err.Error(), "manifest version") {
        t.Fatalf("expected version error, got %v", err)
    }
}
```

- [ ] **Step 2: Run the focused test and confirm RED**

Run:

```bash
nix shell "github:NixOS/nixpkgs/c94da05fe469a845461ae503894fad568abeb2a6#go" --command go test ./internal/manifest -run TestLoad -count=1
```

Expected: FAIL because `Load` and manifest types do not exist.

- [ ] **Step 3: Implement strict manifest loading**

Use `json.Decoder.DisallowUnknownFields`, require one JSON value and EOF, validate every immutable path with `filepath.IsAbs`, and keep `ExplicitConfigDir` unvalidated for the later runtime precedence task. `main` must accept exactly `--manifest ABSOLUTE_PATH -- USER_ARGS...`, load it, call `launch.Run`, and exit with that code.

- [ ] **Step 4: Package the standard-library launcher**

`nix/packages/den-launcher.nix` must use `pkgs.buildGoModule`, `vendorHash = null`, `CGO_ENABLED = 0`, and `subPackages = [ "cmd/den-launcher" ]`. Do not install a `den` symlink.

`modules/checks/launcher-unit.nix` must build the source and run:

```bash
go test ./internal/... ./cmd/... -count=1
```

- [ ] **Step 5: Verify GREEN and package shape**

Run:

```bash
nix build ".#checks.$(nix eval --impure --raw --expr builtins.currentSystem).launcher-unit" --no-link
```

Expected: PASS; the derivation runs the Go tests. Inspect the private package in the check and confirm it contains `bin/den-launcher` and no `bin/den`.

- [ ] **Step 6: Commit the launcher foundation**

```bash
git add go.mod cmd internal/manifest internal/launch nix/packages/den-launcher.nix modules/checks/launcher-unit.nix
git commit -m "feat: add launcher manifest foundation"
```

---

### Task 3: Package the Credential-Free RepoWolf Client Only

**Files:**
- Create: `nix/packages/repowolf-client.nix`
- Create: `nix/check-support/repowolf-client.nix`
- Create: `modules/checks/repowolf-client.nix`

**Interfaces:**
- Produces: a private package with `bin/repowolf-client`, `bin/gh`, and `bin/repowolf-git-ssh`.
- Produces: `checks.<system>.repowolf-client`; later constructors import the same package function directly.

- [ ] **Step 1: Write the failing package/closure check**

The check must assert:

```text
bin/repowolf-client is an executable regular file
bin/gh resolves to repowolf-client
bin/repowolf-git-ssh resolves to repowolf-client
no bin/repowolf server exists
no closure path basename matches openssh, github-cli, gh-, repowolf-server, private-key, or credentials
```

Make the check fail first because `nix/packages/repowolf-client.nix` is absent.

- [ ] **Step 2: Build the client from the pinned source**

Import `${inputs.repowolf}/nix/package-client.nix` with Den's `pkgs`, override only version metadata and `meta.platforms = lib.platforms.unix`, and retain the source's fixed Go vendor hash. Do not consume RepoWolf's server package or its limited `packages.<system>` output; this lets the pure Go client build from the same pinned source on Darwin without adding the service.

- [ ] **Step 3: Expose the build as a check, not a public package**

`modules/checks/repowolf-client.nix` must import the private package and set:

```nix
checks.repowolf-client = repowolfClient;
checks.repowolf-client-closure = import ../../nix/check-support/repowolf-client.nix {
  inherit pkgs repowolfClient;
};
```

Do not add `packages.repowolf-client`.

- [ ] **Step 4: Verify the native client and cross-evaluation**

Run:

```bash
system=$(nix eval --impure --raw --expr builtins.currentSystem)
nix build ".#checks.$system.repowolf-client" ".#checks.$system.repowolf-client-closure" --no-link
for target in x86_64-linux aarch64-linux x86_64-darwin aarch64-darwin; do
  nix eval --raw ".#checks.$target.repowolf-client.drvPath" >/dev/null
done
```

Expected: native builds pass; all four derivations evaluate; no service or credentials occur in the production client closure. Native builds for the other systems happen in Task 15 CI.

- [ ] **Step 5: Commit the client package**

```bash
git add nix/packages/repowolf-client.nix nix/check-support/repowolf-client.nix modules/checks/repowolf-client.nix
git commit -m "feat: package repowolf sandbox client"
```

---

### Task 4: Enforce RepoWolf Inputs, Git Routing, and Environment Scrubbing

**Files:**
- Create: `internal/repowolf/config.go`
- Create: `internal/repowolf/config_test.go`
- Create: `internal/environment/environment.go`
- Create: `internal/environment/environment_test.go`
- Create: `nix/check-support/git-transport.nix`
- Modify: `modules/checks/launcher-unit.nix`
- Modify: `internal/launch/launch.go`

**Interfaces:**
- Produces: `repowolf.LoadEnv(lookup func(string) (string, bool), lstat func(string) (fs.FileInfo, error)) (repowolf.Config, error)`.
- Produces: `environment.Build(host []string, controlled Controlled) []string` with no inherited Git transport or credential state.
- Consumes: manifest RepoWolf client path and controlled `PathEntries` from Tasks 2 and 3.

- [ ] **Step 1: Write failing endpoint, token, and CA tests**

Use table tests for:

- missing and empty `REPOWOLF_ENDPOINT`, `REPOWOLF_TOKEN`, and `REPOWOLF_CA_FILE`.
- HTTPS origin acceptance with an optional valid port and optional `/` path.
- rejection of HTTP, user information, non-root path, query, fragment, opaque URL, uppercase hostname, trailing dot, Unicode hostname, IPv4/IPv6 literals, and GitHub/GitLab/Bitbucket names including subdomains.
- strict RepoWolf token format: `rw1_` plus exactly 43 canonical raw-URL-base64 characters decoding to 32 bytes.
- regular readable CA acceptance; rejection of missing, unreadable, directory, and symbolic CA paths.
- seeded `REPOWOLF_SERVER_NAME` and arbitrary `REPOWOLF_*` overrides never reach the client.
- no token, endpoint, CA path, or environment value in errors.

- [ ] **Step 2: Run focused tests and confirm RED**

Run:

```bash
go test ./internal/repowolf ./internal/environment -count=1
```

Expected: FAIL because the packages do not exist.

- [ ] **Step 3: Implement canonical RepoWolf validation**

Use `net/url`, `net.ParseIP`, ASCII DNS-label validation, `os.Lstat`, and `filepath.Abs`. Preserve the three original RepoWolf variables for the client, but carry only the canonical broker hostname and absolute CA path into policy generation. Never put the token in the policy model.

- [ ] **Step 4: Implement a replacement environment**

Drop `GH_TOKEN`, `GITHUB_TOKEN`, `GH_ENTERPRISE_TOKEN`, `GITHUB_ENTERPRISE_TOKEN`, `SSH_AUTH_SOCK`, `GIT_ASKPASS`, `SSH_ASKPASS`, `GIT_SSH`, `GIT_SSH_COMMAND`, `GIT_CONFIG_GLOBAL`, `GIT_CONFIG_SYSTEM`, `GIT_CONFIG_PARAMETERS`, `GIT_CONFIG_COUNT`, every `GIT_CONFIG_KEY_*`/`GIT_CONFIG_VALUE_*`, and every inherited name beginning `REPOWOLF_`. Preserve ordinary variables, Claude authentication, locale, terminal, and editor variables. Restore only validated `REPOWOLF_ENDPOINT`, `REPOWOLF_TOKEN`, and canonical `REPOWOLF_CA_FILE`; never preserve `REPOWOLF_SERVER_NAME` or future unvalidated RepoWolf overrides. Replace `PATH` and set:

```text
GIT_TERMINAL_PROMPT=0
GIT_SSH_COMMAND=/nix/store/.../bin/repowolf-git-ssh
GIT_CONFIG_COUNT=3
GIT_CONFIG_KEY_0=url.git@github.com:.insteadOf
GIT_CONFIG_VALUE_0=https://github.com/
GIT_CONFIG_KEY_1=credential.helper
GIT_CONFIG_VALUE_1=
GIT_CONFIG_KEY_2=core.sshCommand
GIT_CONFIG_VALUE_2=/nix/store/.../bin/repowolf-git-ssh
```

Use Git's environment configuration interface only; never run `git config`.

- [ ] **Step 5: Add fake Git transport tests**

Implement these Nix-backed cases in `nix/check-support/git-transport.nix`, import them from `modules/checks/launcher-unit.nix`, and run fake clone, fetch, and push commands against `pkgs.gitMinimal`. Assert the rewritten transport invokes only `repowolf-git-ssh`, injected repository/global credential helpers cannot run, and repository/user Git config files remain byte-identical.

- [ ] **Step 6: Verify GREEN and secret-safe diagnostics**

Run:

```bash
go test ./internal/repowolf ./internal/environment -count=1
nix build ".#checks.$(nix eval --impure --raw --expr builtins.currentSystem).launcher-unit" --no-link
```

Expected: PASS; seeded token and credential strings do not appear in captured errors or generated environment diagnostics.

- [ ] **Step 7: Commit RepoWolf routing**

```bash
git add internal/repowolf internal/environment internal/launch/launch.go nix/check-support/git-transport.nix modules/checks/launcher-unit.nix
git commit -m "feat: enforce repowolf routing"
```

---

### Task 5: Implement Secure Claude Configuration-Directory Selection

**Files:**
- Create: `internal/configdir/configdir.go`
- Create: `internal/configdir/configdir_test.go`
- Create: `internal/configdir/identity_linux.go`
- Create: `internal/configdir/identity_darwin.go`
- Create: `internal/configdir/acl.go`
- Create: `internal/configdir/acl_linux.go`
- Create: `internal/configdir/acl_darwin.go`
- Create: `internal/configdir/acl_test.go`
- Create: `nix/lib/protected-paths.nix`
- Modify: `internal/launch/launch.go`

**Interfaces:**
- Produces: `configdir.Select(explicit *string, inherited *string, home string, denied []string, deps Dependencies) (Selection, error)`; pointers distinguish unset from explicitly empty custom values, and `Dependencies.ACLProbe []string` injects the platform probe.
- Produces: `Selection.Revalidate() error` immediately before Fence starts.
- Produces: `Selection.Rollback() error` for any pre-start failure and `Selection.Commit()` only after the Fence child starts successfully.
- Produces: `Selection{Mode, CanonicalPath, WritablePaths, DeniedDefaultPaths, Device, Inode, Created}` for policy generation and lifecycle cleanup.

- [ ] **Step 1: Write failing precedence and default-mode tests**

Cover explicit over inherited, inherited over fallback, empty inherited value as invalid custom input, and fallback only when both custom sources are absent. Default mode must grant exactly:

```text
$HOME/.claude/
$HOME/.claude.json
$HOME/.config/claude/
```

Do not apply custom privacy rules in fallback mode.

- [ ] **Step 2: Write failing custom-path matrix tests**

Use temporary directories and table cases for relative paths, missing path creation at `0700`, existing owner and owner `rwx`, every group/other bit, non-directory, unwritable path, final symlink, canonical parent symlink, owner/non-owner ACLs, inherited ACL after creation, safe/unsafe ancestors, sticky root/user-owned ancestors, and path replacement between selection and revalidation.

Create `nix/lib/protected-paths.nix` first with this exact list; the last two Den additions prevent global/user Git configuration after `allowGitConfig` is enabled for repository config:

```nix
[
  "~/.ssh/id_*"
  "~/.ssh/config"
  "~/.ssh/*.pem"
  "~/.gnupg/**"
  "~/.aws/**"
  "~/.config/gcloud/**"
  "~/.kube/**"
  "~/.docker/**"
  "~/.pypirc"
  "~/.netrc"
  "~/.git-credentials"
  "~/.cargo/credentials"
  "~/.cargo/credentials.toml"
  "~/.gitconfig"
  "~/.config/git/**"
]
```

The overlap table must cover equality, ancestor, and descendant relationships for `/`, `$HOME`, all three default Claude paths, every literal credential path or derived glob root in this shared list, and disjoint in-worktree/out-of-worktree paths. Task 6 must assert that `policy/fence.json` uses the same list.

- [ ] **Step 4: Run tests and confirm RED**

Run:

```bash
go test ./internal/configdir -count=1
```

Expected: FAIL because selection and platform probes do not exist.

- [ ] **Step 4: Implement canonical identity and privacy checks**

Use `Lstat` for the final component, resolve existing parents, and perform overlap plus existing-ancestor safety checks before creating anything. Create a missing custom directory with `0700`, then capture device/inode/uid/mode. On Linux, run the manifest's immutable `${pkgs.acl}/bin/getfacl`; on macOS, run the manifest's `/bin/ls -lde` probe. Reject any effective grant to another principal. Register rollback as soon as Den creates the final directory. Every later pre-launch error path—including settings, sockets, policy generation, final revalidation, and child-start failure—must call `Rollback`; call `Commit` only after Fence starts successfully.

For each canonical ancestor, reject group/other/ACL write unless it is sticky and owned by root or the invoking user. Never chmod an existing path.

- [ ] **Step 5: Implement canonical protected-path overlap**

Build the denied path list from `nix/lib/protected-paths.nix`, serialized into the manifest. For a glob, protect the last complete directory before the first path component containing `*`, `?`, or `[`; for example, every `~/.ssh/id_*` match maps to canonical `~/.ssh`, not `~/.ssh/id_`. Reject a glob with no complete protected root. Require the custom directory to be disjoint from every derived root and the three default paths. Add one concrete-match test per deny glob; Task 6's policy check proves the vendored policy uses the identical patterns.

- [ ] **Step 6: Re-run cross-platform unit and native ACL probes**

Run:

```bash
go test ./internal/configdir -count=1
nix build ".#checks.$(nix eval --impure --raw --expr builtins.currentSystem).launcher-unit" --no-link
```

Expected: PASS on the current host. Platform-specific build tags compile on all four systems during CI; Task 13 exercises real Linux and macOS ACL rejection.

- [ ] **Step 7: Commit configuration-directory isolation**

```bash
git add internal/configdir internal/launch/launch.go nix/lib/protected-paths.nix
git commit -m "feat: isolate claude configuration state"
```

---

### Task 6: Vendor and Generate the Private Fence Policy

**Files:**
- Create: `policy/fence.json`
- Create: `policy/README.md`
- Create: `patches/fence-0.1.58-den-tmpdir.patch`
- Modify: `docs/specs/2026-08-20-den-claude-sandbox-design.md`
- Create: `internal/policy/policy.go`
- Create: `internal/policy/policy_test.go`
- Create: `internal/fence/preflight.go`
- Create: `internal/fence/preflight_test.go`
- Modify: `internal/launch/launch.go`
- Create: `nix/lib/fence.nix`
- Create: `nix/check-support/fence-capabilities.nix`
- Create: `modules/checks/fence-policy.nix`

**Interfaces:**
- Produces: `policy.Generate(Base, Dynamic) ([]byte, error)` with strict JSON output.
- Consumes: canonical RepoWolf, config-directory, closure, socket, and host-port data from earlier tasks.
- Produces: `checks.<system>.fence-capabilities` and `checks.<system>.fence-policy`.

- [ ] **Step 1: Approve and record the pinned Fence TMPDIR erratum**

Stop for maintainer approval before patching the pinned dependency. Update the Fence policy/runtime, platform behavior, and test-plan sections of the specification to record this compatibility correction: Den still uses the locked nixpkgs Fence 0.1.58 source, but applies `patches/fence-0.1.58-den-tmpdir.patch` so `GenerateProxyEnvVars` honors Den's validated scratch path for both outer Fence and nested `fence -c`. Commit the approved erratum alone:

```bash
git add docs/specs/2026-08-20-den-claude-sandbox-design.md
git commit -m "docs: define private fence temporary state"
```

Do not proceed with Task 6 code if a local Fence patch is not approved; escalate the unresolved shared-TMPDIR conflict.

- [ ] **Step 2: Add the minimal pinned Fence patch**

Patch only `internal/sandbox/utils.go` in the locked 0.1.58 source. When `DEN_FENCE_TMPDIR` is absent, retain upstream behavior. When it is present, require an absolute, clean, existing, non-symlink directory and return it from `ensureSandboxTMPDIR`; return a fixed nonexistent fail-closed path for any invalid value rather than `/tmp`. The Den launcher removes inherited `DEN_FENCE_TMPDIR` and sets it only to its owner-validated `0700` scratch directory. Add upstream-level tests for absent, valid, relative, missing, non-directory, and symbolic values. `nix/lib/fence.nix` must apply this single patch with `overrideAttrs` while retaining version `0.1.58` and the locked source.

- [ ] **Step 3: Vendor the exact reference snapshot**

Extract commit `4be05d63af92cf79231313a20df22b3c144795d0`, file `ai-tooling/sandboxing/fence.json`, from the authenticated/local engineering-handbook checkout. Verify its original SHA-256 is:

```text
bc4ec1509ffa812b5d89bf258b9adade1262c4f1f50be2d52ffa230270475f29
```

Convert its JSONC comments/trailing commas to strict JSON, then make only the design changes. Do not import `jail.nix` permissions. Replace `network.allowedDomains` with exactly:

```text
api.anthropic.com
*.anthropic.com
claude.ai
*.claude.ai
registry.npmjs.org
*.npmjs.org
registry.yarnpkg.com
pypi.org
files.pythonhosted.org
crates.io
static.crates.io
index.crates.io
proxy.golang.org
sum.golang.org
formulae.brew.sh
```

Set the deny list to include exactly the protected Git-host families and retained metadata/telemetry entries:

```text
github.com
*.github.com
githubusercontent.com
*.githubusercontent.com
gitlab.com
*.gitlab.com
bitbucket.org
*.bitbucket.org
169.254.169.254
metadata.google.internal
instance-data.ec2.internal
statsig.anthropic.com
```

Normalize the static filesystem section instead of retaining the reference grants:

```text
allowPty = true
filesystem.defaultDenyRead = true
filesystem.strictDenyRead = true
filesystem.allowGitConfig = true
filesystem.allowRead = []
filesystem.allowExecute = []
filesystem.allowWrite = []
filesystem.denyRead = the exact nix/lib/protected-paths.nix patterns
filesystem.denyWrite = reference secret-file patterns plus Fence implicit host paths
```

The static file must contain no `/nix/store/**`, `/nix/var/nix/profiles/**`, `~/.nix-profile/**`, `.`, `/tmp`, cache, broad `~/.config/**`, broad `~/.local/**`, default Claude-state, other-agent-state, package-cache, or 1Password socket grant. Task 6's generator adds required closures and per-launch writable roots only after canonical validation.

Set `filesystem.denyRead` to exactly the shared patterns for SSH private keys/config, GnuPG, AWS, Google Cloud, Kubernetes, Docker credentials, `.pypirc`, `.netrc`, Git credential stores, Cargo credential files, `~/.gitconfig`, and `~/.config/git/**`. `allowGitConfig = true` is required only so Git can read the current worktree's `.git/config`; the explicit home denials and controlled Git environment prevent user/global transport or credential configuration. Add dynamic `denyWrite` entries for the canonical worktree `.git/config` and `.git/config.worktree` so repository configuration stays byte-identical. Extend static `denyWrite` with `$HOME/.npm/_logs`, `$HOME/.fence/debug`, `/tmp/fence`, and `/private/tmp/fence` so Fence 0.1.58's implicit host write grants cannot escape the selected roots. The launcher sets `TMPDIR` to the validated per-launch scratch directory, preventing Fence's Darwin TMPDIR-parent rule from widening access to `/var/folders/...`. Preserve the reference command denies and make filesystem/domain denies take precedence over grants. Record source commit, path, original digest, and each Den change in `policy/README.md`.

- [ ] **Step 4: Write failing policy tests**

Test strict parsing, `defaultDenyRead = true`, `strictDenyRead = true`, unmatched-outbound default denial, exact static domain sets, denied-domain precedence, no broad wildcard that matches a denied Git host, `command.runtimeExecPolicy = "argv"` on Linux, exact equality with `nix/lib/protected-paths.nix`, implicit-write denials, and absence of dynamic values or tokens in the base file. Add negative assertions for every removed reference grant: no whole Nix store/profile, `/tmp`, current-directory, cache, broad config/local, default Claude, other-agent, package-cache, or 1Password socket allow entry.

Add generation tables for one broker hostname, CA read-only path, closure read/execute paths, working tree, selected state paths, separate scratch/policy directories, custom-mode default write denials, socket grants, and Linux/macOS host-port differences.

- [ ] **Step 5: Run focused tests and confirm RED**

Run:

```bash
go test ./internal/policy -count=1
```

Expected: FAIL because the generator does not exist.

- [ ] **Step 6: Implement typed dynamic policy generation**

Parse the base into typed top-level network/filesystem/command structures while preserving supported fields. Resolve every dynamic path before insertion. Add only:

```text
exact RepoWolf hostname
absolute regular CA file as read-only
required Nix closure paths as read/execute
Linux operational reads: canonical resolv.conf target, /etc/hosts, /etc/nsswitch.conf, /etc/services, and /etc/protocols
Darwin operational reads: /System/Library, /usr/lib, /usr/share/icu, /private/etc, and /private/var/db/timezone
launch worktree and scratch as both readable and writable
selected Claude state paths as both readable and writable
custom-mode default state paths as higher-precedence write-denied
current worktree .git/config and .git/config.worktree as higher-precedence write-denied
policy file and parent as higher-precedence write-denied
validated Unix sockets as read-write only
combined validated host ports using platform semantics
```

Writable policy entries must also enter `allowRead`, because Darwin read and write operations are distinct. Marshal strict JSON. The API must not accept a token field. Remove inherited `TMPDIR` and `DEN_FENCE_TMPDIR`, then set both to the validated scratch directory; the patched outer Fence and nested `fence -c` preserve that exact path. After changing the generated policy to `0400`, export its absolute path as `DEN_FENCE_POLICY_FILE` only for the outer Fence process and mandatory Darwin hook.

Before policy generation on Linux, `internal/fence.Preflight` must execute pinned `${fence}/bin/fence --linux-features`, parse the exact `Network namespace` table row from 0.1.58, and require status `ok`. Missing, `unavailable`, duplicate, or malformed rows fail before Claude starts with a corrective message; never accept Fence's direct-network fallback. Unit tests use fixed feature outputs, and the native check runs the real probe on every Linux runner.

- [ ] **Step 7: Check Fence 0.1.58 capabilities**

`nix/lib/fence.nix` must start from `pkgs.fence`, assert upstream version `0.1.58`, apply only `patches/fence-0.1.58-den-tmpdir.patch`, and map that patched package to explicit booleans for `settings`, `claudePreToolUse`, `commandWrapper`, `linuxFeatures`, `exposeHostPath`, `denFenceTmpdir`, `strictDenyRead`, `argvRuntimePolicy`, `allowUnixSockets`, and `allowLocalOutboundPorts`. Assert all required platform capabilities during evaluation and reject every unknown upstream version or unexpected patch count/hash. `nix/check-support/fence-capabilities.nix` must run the patched binary/config parser to prove `--settings`, `--claude-pre-tool-use`, `fence -c`, `--linux-features`, `--expose-host-path`, strict read fields, argv runtime policy, Darwin `allowUnixSockets`, Linux `allowLocalOutboundPorts`, and valid/invalid `DEN_FENCE_TMPDIR` behavior. Add packaged-Fence behavioral tests that outer Claude and nested `fence -c` both receive and can write only the selected scratch TMPDIR, unrelated home/store files cannot be read, default implicit host write paths cannot be changed, selected worktree/state/scratch paths still work, and a Linux `REPOWOLF_CA_FILE` under `/tmp` is readable only through the explicit read-only exposure. This pairs fail-fast evaluation with binary evidence. A missing feature fails; do not fall back to a weaker policy.

- [ ] **Step 8: Verify policy checks**

Run:

```bash
go test ./internal/policy -count=1
system=$(nix eval --impure --raw --expr builtins.currentSystem)
nix build ".#checks.$system.fence-capabilities" ".#checks.$system.fence-policy" --no-link
```

Expected: PASS; generated JSON parses; no seeded `rw1_` token appears; policy and parent are explicitly write-denied; denied domains and paths win over grants.

- [ ] **Step 9: Commit the Fence policy**

```bash
git add policy patches/fence-0.1.58-den-tmpdir.patch internal/policy internal/fence internal/launch/launch.go nix/lib/fence.nix nix/check-support/fence-capabilities.nix modules/checks/fence-policy.nix
git commit -m "feat: generate private fence policy"
```

---

### Task 7: Add Optional Docker, Podman, and Host-Port Capabilities

**Files:**
- Create: `internal/container/socket.go`
- Create: `internal/container/socket_test.go`
- Modify: `internal/policy/policy.go`
- Modify: `internal/policy/policy_test.go`
- Modify: `internal/launch/launch.go`

**Interfaces:**
- Produces: `container.ResolveDocker(Config, Env, Home) (Socket, error)`.
- Produces: `container.ResolvePodman(Config, Env, Home, UID, Platform) (Socket, error)`.
- Produces: `container.CombinePorts(docker, podman []uint16) []uint16` sorted and deduplicated.

- [ ] **Step 1: Write failing Docker selection tests**

Cover precedence: explicit path, valid `DOCKER_HOST`, `$XDG_RUNTIME_DIR/docker.sock`, `$HOME/.docker/run/docker.sock`, `/run/docker.sock`, `/var/run/docker.sock`. Accept only `unix:///absolute/path` with no authority/query/fragment/encoded separator. Reject TCP, SSH, HTTP, npipe, relative, malformed, missing, and non-socket targets. Resolve a socket symlink to one final path.

- [ ] **Step 2: Write failing Podman and port tests**

Cover explicit path, valid `CONTAINER_HOST`, `$XDG_RUNTIME_DIR/podman/podman.sock`, and `/run/user/<uid>/podman/podman.sock`; Linux default `XDG_RUNTIME_DIR`; macOS requirement for explicit/environment socket when discovery fails; invoking-user ownership; the same endpoint rejections; disabled-by-default behavior; invalid ports; duplicates; empty lists; and rejection of non-empty ports when the parent integration is disabled.

- [ ] **Step 3: Run focused tests and confirm RED**

Run:

```bash
go test ./internal/container ./internal/policy -count=1
```

Expected: FAIL because socket resolution is missing.

- [ ] **Step 4: Implement exact socket capability wiring**

Use `Lstat`, `EvalSymlinks`, `Stat`, Unix mode-socket checks, and UID checks. Export only canonical endpoints:

```text
DOCKER_HOST=unix:///final/absolute/docker.sock
CONTAINER_HOST=unix:///final/absolute/podman.sock
XDG_RUNTIME_DIR=/run/user/<uid>  # Linux default only
```

Grant read-write to the socket itself, never its parent or Docker/Podman configuration directories. On Darwin add the exact socket to `network.allowUnixSockets`.

- [ ] **Step 5: Implement host-port policy semantics**

On Linux, set `network.allowLocalOutbound = true` and emit the exact combined ports in `network.allowLocalOutboundPorts` when at least one port is requested. With no ports, set `network.allowLocalOutbound = false` and emit no ports. On Darwin, any requested port sets `network.allowLocalOutbound = true`, which enables all localhost ports; an empty list sets it to false. Never record a false exact-port claim on Darwin. Test both fields for empty and non-empty inputs, and surface the widening in module/README documentation.

- [ ] **Step 6: Verify container behavior**

Run:

```bash
go test ./internal/container ./internal/policy -count=1
nix build ".#checks.$(nix eval --impure --raw --expr builtins.currentSystem).launcher-unit" --no-link
```

Expected: PASS using temporary Unix sockets; no live daemon is required or contacted.

- [ ] **Step 7: Commit container capability support**

```bash
git add internal/container internal/policy internal/launch/launch.go
git commit -m "feat: add optional container sockets"
```

---

### Task 8: Add the Claude Adapter and Mandatory macOS `den-fence` Hook

**Files:**
- Create: `internal/claude/arguments.go`
- Create: `internal/claude/arguments_test.go`
- Create: `internal/claude/settings.go`
- Create: `internal/claude/settings_test.go`
- Create: `nix/lib/mk-claude.nix`
- Create: `nix/check-support/claude-adapter.nix`
- Create: `nix/check-support/claude-settings-merge.nix`
- Create: `modules/checks/claude-adapter.nix`
- Modify: `internal/launch/launch.go`
- Modify: `docs/specs/2026-08-20-den-claude-sandbox-design.md`

**Interfaces:**
- Produces: `claude.ValidateArguments([]string) error` and unchanged accepted arguments.
- Produces: `claude.ValidateDarwinSettings(Scopes) error` that protects only `den-fence`.
- Produces: internal `mkClaude` adapter data consumed by `mkAgentSandbox` in Task 10 and directly evaluated by `checks.<system>.claude-adapter` in this task.

- [ ] **Step 1: Record the approved Claude 2.1.158 compatibility erratum**

Stop for maintainer approval before changing the finalized specification. Once approved, update the Claude adapter, runtime flow, error handling, documentation, and test-plan sections to state that Darwin rejects `--bare` and removes inherited `CLAUDE_CODE_SIMPLE` because both skip the mandatory hook. Commit this documentation change alone:

```bash
git add docs/specs/2026-08-20-den-claude-sandbox-design.md
git commit -m "docs: protect claude hook from bare mode"
```

Do not proceed with Task 8 code if the erratum is not approved; escalate the unresolved mandatory-hook conflict.

- [ ] **Step 2: Write failing reserved/ordinary argument tests**

Reject exact and `--flag=value` forms of `--settings`, `--permission-mode`, and `--dangerously-skip-permissions` on every platform. On Darwin also reject exact/assigned `--bare` before Fence and scrub inherited `CLAUDE_CODE_SIMPLE`; add black-box tests showing neither path can suppress the Den hook. On Linux preserve `--bare` and `CLAUDE_CODE_SIMPLE` as ordinary user choices because the security hook is Darwin-only. Accept and preserve order, spaces, and empty values for other ordinary arguments, including `--plugin-dir`, `--mcp-config`, and `--strict-mcp-config`.

- [ ] **Step 3: Write failing settings-scope protection tests**

On Darwin inspect the selected user file `$CLAUDE_CONFIG_DIR/settings.json`, project `$PWD/.claude/settings.json`, project `$PWD/.claude/settings.local.json`, and `/Library/Application Support/ClaudeCode/managed-settings.json`. These are the applicable settings scopes for Claude Code 2.1.158; add a startup assertion that fails if its documented scope set changes. Treat `den-fence` as Den's internal name for the exact tuple `PreToolUse` + `Bash` + `command` + the immutable Fence command; do not invent an unsupported Claude JSON `id` field. Reject `disableAllHooks = true` and any user hook command that contains `--claude-pre-tool-use` or `DEN_FENCE_POLICY_FILE`, because those are replacement attempts. Accept unrelated user hooks unchanged. Missing files are allowed; malformed applicable settings fail with the scope name and corrective action, not contents. Fingerprint every inspected file and re-read/revalidate the scopes immediately before Fence starts so a post-validation change fails closed.

- [ ] **Step 3: Run tests and confirm RED**

Run:

```bash
go test ./internal/claude -count=1
```

Expected: FAIL because argument and settings validation are absent.

- [ ] **Step 5: Implement the adapter's immutable settings artifact**

`nix/lib/mk-claude.nix` must select `pkgs.claude-code` from the pinned nixpkgs input, assert version `2.1.158`, define mandatory `--dangerously-skip-permissions`, and on Darwin create a store JSON settings file with one `PreToolUse` hook for `Bash`. Use only Claude's supported matcher/type/command fields; `den-fence` remains Den's internal diagnostic name. Its command is exactly:

```text
${fence}/bin/fence --claude-pre-tool-use --settings "$DEN_FENCE_POLICY_FILE"
```

It must reroute allowed Bash commands through `fence -c` according to Fence 0.1.58 behavior. Attach this file with Den's mandatory `--settings` argument; do not run `fence hooks install` or edit user settings.

`nix/check-support/claude-settings-merge.nix` must run the real pinned Claude binary against a local check-only Anthropic Messages API fixture. Set temporary test credentials and `ANTHROPIC_BASE_URL` only inside the check; the fixture returns one Bash `tool_use` request and then a final response. Use a recording fake Fence command in the immutable settings artifact and one unrelated user hook, invoke Claude with `-p "run the fixture command" --settings ...`, and assert both hook marker files are created before the Bash tool runs. The fixture must reject any non-loopback request and assert no external network attempt. Add black-box negatives for Darwin `--bare`, inherited `CLAUDE_CODE_SIMPLE`, `disableAllHooks`, and a replacement command; all must fail in the Den launcher before fake Fence starts. Repeat the settings fingerprint check immediately before launch. This executed tool-call test, not an authentication-stage debug log, is the compatibility gate for Claude's settings merge/precedence behavior.

- [ ] **Step 6: Keep user resources outside the adapter**

The adapter may describe writable state and the security hook only. `nix/check-support/claude-adapter.nix` must import it with a recording fake factory so this task verifies the generated adapter before public outputs exist. Assert in tests and Nix evaluation that it has no skill, plugin, MCP, Context Mode, CodeGraph, seed directory, resource package, or non-security hook field. Do not run `claude plugin install` or set `CLAUDE_CODE_PLUGIN_SEED_DIR`.

- [ ] **Step 7: Verify the adapter**

Run:

```bash
go test ./internal/claude -count=1
system=$(nix eval --impure --raw --expr builtins.currentSystem)
nix build ".#checks.$system.launcher-unit" ".#checks.$system.claude-adapter" --no-link
```

Expected: PASS; ordinary user resources are accepted; reserved flags and hook replacement fail before Fence; the immutable hook path is store-backed.

- [ ] **Step 8: Commit the Claude adapter**

```bash
git add internal/claude internal/launch/launch.go nix/lib/mk-claude.nix nix/check-support/claude-adapter.nix nix/check-support/claude-settings-merge.nix modules/checks/claude-adapter.nix
git commit -m "feat: add secured claude adapter"
```

---

### Task 9: Preserve Process, Terminal, Exit, and Cleanup Behavior

**Files:**
- Create: `internal/process/process.go`
- Create: `internal/process/process_test.go`
- Create: `internal/tempdir/tempdir.go`
- Create: `internal/tempdir/tempdir_test.go`
- Create: `nix/check-support/process-fixture.nix`
- Modify: `modules/checks/launcher-unit.nix`
- Modify: `internal/launch/launch.go`

**Interfaces:**
- Produces: `process.Run(Command, IO, Signals) int` with Fence/Claude exit semantics.
- Produces: `tempdir.NewPair(root string) (policyDir, scratchDir string, cleanup func() error, err error)`.
- Produces: `tempdir.RemoveStale(root string, uid int, olderThan time.Duration) error` with a fixed 24-hour threshold.

- [ ] **Step 1: Write failing lifecycle and permission tests**

Assert separate policy/scratch directories at `0700`, policy creation at `0600` followed by `0400`, cleanup after zero/nonzero/catchable-signal exits, cleanup failure not replacing child status, owner-and-age-gated stale cleanup, and preservation of a recent or foreign-owned directory.

- [ ] **Step 2: Write failing process behavior tests**

Use fake Fence and Claude children plus `nix/check-support/process-fixture.nix`, wired into `modules/checks/launcher-unit.nix`. Cover stdin/stdout/stderr, empty arguments, foreground process group, terminal resize, normal/nonzero exits, `128 + signal`, `SIGINT`, `SIGTERM`, `SIGHUP`, `SIGQUIT`, `SIGWINCH`, `SIGTSTP`, and `SIGCONT`.

- [ ] **Step 3: Run focused tests and confirm RED**

Run:

```bash
go test ./internal/process ./internal/tempdir -count=1
```

Expected: FAIL because lifecycle helpers do not exist.

- [ ] **Step 4: Implement process supervision**

Start `${fence}/bin/fence --settings "$DEN_FENCE_POLICY_FILE" --expose-host-path "$REPOWOLF_CA_FILE" --` followed by the immutable agent executable, mandatory arguments, and unchanged user arguments. The validated read-only host exposure is required because Fence overlays Linux `/tmp`; the policy still grants only read and explicitly denies write. Connect all three standard streams directly. Preserve the foreground process group and job-control signals; forward the four required terminating signals without swallowing `SIGWINCH`, `SIGTSTP`, or `SIGCONT`. Return the child's exact code or `128 + signal`.

- [ ] **Step 5: Implement private directory cleanup**

Use manifest `ScratchRoot = "/tmp"` on Linux and `ScratchRoot = "/private/tmp"` on Darwin. Require that root to resolve to the canonical sticky root-owned directory. Create or reuse only `${ScratchRoot}/den-${uid}` after `Lstat` proves it is a non-symlink directory owned by the invoking user with mode `0700`; fail rather than repair an unsafe existing parent. Create separate random policy and scratch children beneath it, and scan only that validated parent with the 24-hour stale threshold. Set `TMPDIR` and `DEN_FENCE_TMPDIR` to the random scratch child before outer Fence starts; tests must prove the patched outer Fence and hook-rerouted nested `fence -c` preserve it. Before starting Fence, make the policy read-only and run custom configuration-directory plus Darwin settings revalidation. Keep `Selection.Rollback` deferred across every pre-start return; call `Selection.Commit` only after `cmd.Start` succeeds. After exit, best-effort remove both temporary children while preserving the primary status; the selected Claude directory persists after a successful start.

- [ ] **Step 6: Verify unit and PTY behavior**

Run:

```bash
go test ./internal/process ./internal/tempdir -count=1
system=$(nix eval --impure --raw --expr builtins.currentSystem)
nix build ".#checks.$system.launcher-unit" --no-link
```

Expected: PASS; simulated SIGKILL leaves a stale directory that the next launch removes only after the test clock crosses 24 hours.

- [ ] **Step 7: Commit transparent lifecycle handling**

```bash
git add internal/process internal/tempdir internal/launch/launch.go nix/check-support/process-fixture.nix modules/checks/launcher-unit.nix
git commit -m "feat: preserve sandbox process behavior"
```

---

### Task 10: Build `mkAgentSandbox`, `mkClaude`, and Public Package Outputs

**Files:**
- Create: `nix/lib/options.nix`
- Create: `nix/lib/mk-agent-sandbox.nix`
- Modify: `nix/lib/mk-claude.nix`
- Create: `modules/lib/claude.nix`
- Create: `modules/packages/claude.nix`
- Create: `nix/check-support/package-api.nix`
- Create: `modules/checks/package-api.nix`

**Interfaces:**
- Produces: internal `mkAgentSandbox { adapter; configDir; extraPkgs; docker; podman; dependencies ? null; }`; `dependencies` is private test injection and is never exposed through `mkClaude`.
- Produces: public `lib.<system>.mkClaude { configDir ? null; extraPkgs ? []; docker ? {}; podman ? {}; }`.
- Produces: `packages.<system>.claude` and the identical `packages.<system>.default`.

- [ ] **Step 1: Write failing option and output evaluation checks**

Cover defaults, absolute explicit `configDir`, absolute explicit socket paths, unique 1-65535 host ports, ports requiring enabled parent integration, custom packages, and invalid values. Assert exact output names, package equality, `meta.mainProgram = "claude"`, no `packages.den`, no `bin/den`, no resource outputs/options/constructors, and `mkClaude { }` equality with the default package.

- [ ] **Step 2: Run the package API check and confirm RED**

Run:

```bash
nix build ".#checks.$(nix eval --impure --raw --expr builtins.currentSystem).package-api" --no-link
```

Expected: FAIL because the constructor and outputs are absent.

- [ ] **Step 3: Normalize constructor options**

`nix/lib/options.nix` must merge these defaults and assert the documented types and relationships:

```nix
{
  configDir = null;
  extraPkgs = [ ];
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

Do not convert strings to Nix paths. Keep `configDir = null` so runtime inheritance remains possible.

- [ ] **Step 4: Implement the internal factory**

Define a private dependency record with `fence`, `repoWolfClient`, `launcher`, `git`, `bash`, `coreutils`, and ACL tools. When `dependencies = null`, import the production packages and select Fence through `nix/lib/fence.nix` so unknown versions or missing declared capabilities fail during evaluation. Tests may pass fakes to this internal factory only. Compute one closure-info file over the selected dependencies, Claude, adapter tools, optional clients, and `extraPkgs`.

Build `PathEntries` in this exact order:

```text
RepoWolf client
Fence
pkgs.gitMinimal
Bash
Coreutils
launcher support
Claude adapter runtime packages
Docker/Compose when enabled
Podman/Podman Compose when enabled
extraPkgs
```

Generate the version-1 manifest as a store JSON file, including the RepoWolf client directory, protected path patterns from `nix/lib/protected-paths.nix`, `ScratchRoot = "/tmp"` on Linux or `"/private/tmp"` on Darwin, and the platform ACL probe (`${pkgs.acl}/bin/getfacl` on Linux, `/bin/ls -lde` on Darwin). Build one wrapper named `claude` that invokes only the immutable launcher with `--manifest ... -- "$@"`. The factory owns policy, environment, containers, signals, exit status, and cleanup; the adapter owns Claude executable, mandatory args, state description, and Darwin settings.

- [ ] **Step 5: Export the stable constructor and packages**

Follow Roche Pi's `modules/lib/jailed-pi.nix` and `modules/packages/pi.nix` pattern. `modules/lib/claude.nix` sets `perSystem.lib.mkClaude`; `modules/packages/claude.nix` sets both package attrs to one `self'.lib.mkClaude { }` value.

- [ ] **Step 6: Verify package API and PATH precedence**

Run:

```bash
system=$(nix eval --impure --raw --expr builtins.currentSystem)
nix build ".#checks.$system.package-api" ".#packages.$system.claude" --no-link
nix eval --raw ".#packages.$system.default.outPath"
nix eval --raw ".#packages.$system.claude.outPath"
```

Expected: both paths are identical; the package contains only `bin/claude`; a fake `extraPkgs` `gh` cannot shadow RepoWolf's `gh`; `extraPkgs` does not enter a host package set.

- [ ] **Step 7: Commit constructors and outputs**

```bash
git add nix/lib nix/check-support/package-api.nix modules/lib modules/packages modules/checks/package-api.nix
git commit -m "feat: expose claude sandbox package"
```

---

### Task 11: Add Shared Home Manager and devenv Modules

**Files:**
- Create: `nix/lib/module-options.nix`
- Create: `modules/home/den.nix`
- Create: `modules/devenv/den.nix`
- Create: `nix/check-support/module-api.nix`
- Create: `modules/checks/module-api.nix`

**Interfaces:**
- Produces: one shared `programs.den.claude` option declaration.
- Produces: `homeModules.den` and `devenvModules.den`, both calling `self.lib.${pkgs.system}.mkClaude`.

- [ ] **Step 1: Write failing module evaluation tests**

Using actual Home Manager and devenv evaluators, cover disabled modules adding no package, enabled modules adding exactly one Den Claude artifact, `configDir = null`, all container defaults, custom package propagation, absolute string checks, socket checks, unique port checks, and ports requiring enabled integration. Compare the resulting package outPath with direct `mkClaude` calls.

- [ ] **Step 2: Run the module check and confirm RED**

Run:

```bash
nix build ".#checks.$(nix eval --impure --raw --expr builtins.currentSystem).module-api" --no-link
```

Expected: FAIL because neither module exists.

- [ ] **Step 3: Define one shared option tree**

Use `types.nullOr (types.strMatching "^/.*")` for `configDir` and socket paths, `types.listOf types.package` for `extraPkgs`, `types.package` for clients, and `types.listOf (types.ints.between 1 65535)` plus assertions for uniqueness and parent enablement. Put the macOS localhost-widening warning in both `hostPorts` descriptions.

- [ ] **Step 4: Implement both thin integrations**

The Home Manager module adds only `mkClaude`'s result to `home.packages`. The devenv module adds only the same result to `packages`. Neither module may generate a wrapper, mutate Claude settings, create state, or add `extraPkgs` directly to the host list.

- [ ] **Step 5: Verify both integrations**

Run:

```bash
system=$(nix eval --impure --raw --expr builtins.currentSystem)
nix build ".#checks.$system.module-api" --no-link
nix eval --json '.#homeModules' --apply builtins.attrNames | grep -F '"den"'
nix eval --json '.#devenvModules' --apply builtins.attrNames | grep -F '"den"'
```

Expected: PASS; both module attrs contain `den`; enabled outputs equal direct construction and disabled outputs contain no Den package.

- [ ] **Step 6: Commit module integrations**

```bash
git add nix/lib/module-options.nix modules/home modules/devenv nix/check-support/module-api.nix modules/checks/module-api.nix
git commit -m "feat: add claude integration modules"
```

---

### Task 12: Add Pure End-to-End, Startup, and Closure Checks

**Files:**
- Create: `nix/check-support/fakes.nix`
- Create: `nix/check-support/pure-launcher.nix`
- Create: `nix/check-support/claude-startup.nix`
- Create: `nix/check-support/package-closure.nix`
- Create: `modules/checks/pure-launcher.nix`
- Create: `modules/checks/claude-startup.nix`
- Create: `modules/checks/package-closure.nix`

**Interfaces:**
- Produces: `checks.<system>.pure-launcher`, `.claude-startup`, and `.package-closure`.
- Consumes: the real Nix constructor with fake Fence/Claude/RepoWolf binaries for deterministic pure behavior.

- [ ] **Step 1: Create deterministic fake packages**

Provide fake Fence that records policy/argv/environment and then runs the child, fake Claude that records state writes and process observations, and fake RepoWolf shims that record API/Git calls. Import `nix/lib/mk-agent-sandbox.nix` with its private `dependencies` record; do not add test arguments to public `mkClaude`. The fakes must never use live credentials or networks.

- [ ] **Step 2: Write the comprehensive pure launcher check**

Translate every bullet under the specification's “Pure launcher and policy checks” into an assertion. Include RepoWolf validation, all config-directory and overlap cases, policy permissions/precedence, Git rewrites and scrubbed injections, unchanged ordinary arguments, accepted user resources, rejected security flags/hook replacement, PTY/signals/statuses, cleanup/stale cleanup, and unchanged Git configuration.

- [ ] **Step 3: Write default/custom startup checks**

Default mode uses an empty temporary home and only the three fake RepoWolf values; confirm all three normal Claude state paths are writable. Custom mode must test an ignored in-worktree directory and one outside home/worktree, each via explicit option and inherited environment; confirm `0700`, ownership, selected-path writes, and denied default writes. Repeat overlap and final-symlink cases for both layouts.

Seed a user skill, plugin, unrelated hook, and MCP configuration. Confirm the wrapper does not reject, alter, package, or pre-validate them. Stop at the fake/offline provider boundary; make no Anthropic or Git-host request.

- [ ] **Step 4: Write production closure checks**

Assert inclusion of Claude, Fence 0.1.58, RepoWolf client links, `pkgs.gitMinimal`, Bash, Coreutils, launcher, and required policy/settings artifacts. Assert absence of RepoWolf server, normal GitHub CLI, credentials, private keys, `bin/den`, supplied skills/plugins/MCP servers, Context Mode, CodeGraph, resource bundles, marketplace data, and `CLAUDE_CODE_PLUGIN_SEED_DIR`.

- [ ] **Step 5: Run the three checks**

Run:

```bash
system=$(nix eval --impure --raw --expr builtins.currentSystem)
nix build \
  ".#checks.$system.pure-launcher" \
  ".#checks.$system.claude-startup" \
  ".#checks.$system.package-closure" \
  --no-link --print-build-logs
```

Expected: all pass without a live RepoWolf, Claude provider, Docker daemon, Podman daemon, or Git host.

- [ ] **Step 6: Commit pure integration checks**

```bash
git add nix/check-support modules/checks
git commit -m "test: cover claude sandbox integration"
```

---

### Task 13: Add Native Fence and RepoWolf Enforcement Checks

**Files:**
- Create: `tests/native/native_test.go`
- Create: `tests/native/fixtures.go`
- Create: `tests/native/acl_linux_test.go`
- Create: `tests/native/acl_darwin_test.go`
- Create: `nix/check-support/native-enforcement.nix`
- Create: `modules/checks/native-enforcement.nix`
- Create: `scripts/check-native.sh`

**Interfaces:**
- Produces: `checks.<system>.native-enforcement`, intentionally run with host sandbox facilities available.
- Produces: `scripts/check-native.sh <system>` as the one CI/local entry for native checks.

- [ ] **Step 1: Write host-level tests before the driver**

Use the packaged Fence executable, local TLS/DNS fixtures, fake provider endpoints, and a check-only RepoWolf protocol fixture. Cover every bullet under “Native Fence and RepoWolf enforcement checks”: broker and registry allows, Git-host denies without external traffic, symlink credential denial, custom default-path denial inside a worktree-home, Linux/macOS ACL rejection, replaceable ancestor/path swap, immutable policy mutation attempts, Linux multi-token argv denial, Darwin Bash reroute/deny, user plugin/MCP Fence denials, and RepoWolf `gh`/Git operations reaching only the local fixture. Add Linux cases for a real `Network namespace: ok` preflight, fail-closed unavailable/malformed feature output, and a regular CA file under `/tmp` re-exposed read-only after Fence's tmpfs overlay. Add effective-policy probes showing unrelated host/store reads and Fence implicit host write paths remain denied on both platforms. On Darwin assert the actual outer Claude process and a Bash command rerouted through nested `fence -c` see the same unique `DEN_FENCE_TMPDIR` scratch path, can write there, cannot write `/tmp/fence` or `/private/tmp/fence`, and do not share temporary files across concurrent launches.

- [ ] **Step 2: Confirm the native test target fails before wiring**

Run:

```bash
go test -tags=native ./tests/native -count=1
```

Expected: FAIL with missing packaged Fence/fixture environment, proving the test is not silently skipping.

- [ ] **Step 3: Package the native driver**

The check may build a RepoWolf server or protocol fixture from the pinned source only as a test input. Keep it out of `mkAgentSandbox` and all production closures. Generate local certificates and provider data at test time; use no credentials.

The derivation must fail rather than skip when the platform lacks Fence, `sandbox-exec`, Linux namespace/argv enforcement, ACL support, or the RepoWolf fixture.

- [ ] **Step 4: Implement the native check script**

`scripts/check-native.sh` must reject a requested system different from `builtins.currentSystem`. First evaluate the full flake, then build the package and every check except the host-only enforcement check under normal Nix settings. Finally run only native enforcement with the Nix build sandbox disabled:

```bash
nix flake check --no-build
nix build ".#packages.$system.claude" --no-link --print-build-logs

while IFS= read -r check; do
  nix build ".#checks.$system.$check" --no-link --print-build-logs
done < <(
  nix eval --raw ".#checks.$system" --apply '
    checks:
      builtins.concatStringsSep "\n"
        (builtins.attrNames (builtins.removeAttrs checks [ "native-enforcement" ]))
      + "\n"
  '
)

nix build --option sandbox false ".#checks.$system.native-enforcement" \
  --no-link --print-build-logs
```

This split is mandatory: plain `nix flake check` must not try to execute host enforcement inside the normal Nix sandbox, and no non-native check may be omitted from a native job.

- [ ] **Step 5: Run on the current native host**

Run:

```bash
scripts/check-native.sh "$(nix eval --impure --raw --expr builtins.currentSystem)"
```

Expected: all local fixtures pass; no external provider is contacted. If host policy forbids unsandboxed checks, run on the matching CI native runner; do not replace it with cross-evaluation.

- [ ] **Step 6: Commit native enforcement checks**

```bash
git add tests/native nix/check-support/native-enforcement.nix modules/checks/native-enforcement.nix scripts/check-native.sh
git commit -m "test: add native sandbox enforcement"
```

---

### Task 14: Document Operation, Security, and Troubleshooting

**Files:**
- Create: `README.md`

**Interfaces:**
- Documents: package, direct constructor, Home Manager, and devenv usage with the normal `claude` command.
- Documents: all security boundaries and limits required by the design.

- [ ] **Step 1: Write the architecture and prerequisites**

Explain the `launcher -> Fence -> Claude -> RepoWolf clients` flow, all four systems, RepoWolf server prerequisite, and exact requirements for `REPOWOLF_ENDPOINT`, `REPOWOLF_TOKEN`, and `REPOWOLF_CA_FILE`. State that Fence is not a VM and RepoWolf is the only GitHub route.

- [ ] **Step 2: Add all four integration examples**

Show flake package installation, `inputs.den.lib.${system}.mkClaude`, `homeModules.den`, and `devenvModules.den`. Include the exact project-state example `export CLAUDE_CONFIG_DIR="$PWD/.devenv/state/claude"`. Every command example must run `claude`; never show `den`.

- [ ] **Step 3: Document configuration-directory modes**

Cover precedence, absolute paths, fallback/default state paths, custom creation/ownership/mode/ACL/ancestor/symlink/overlap/revalidation rules, environment authentication, and the macOS Keychain limitation. Include a devenv/direnv example that exports an absolute `$PWD/.devenv/state/claude` path and adds it to `.gitignore`.

- [ ] **Step 4: Document packages, containers, and process behavior**

Explain `extraPkgs` scope and PATH precedence; Docker/Podman options and discovery; rootless Podman defaults; local Unix-only endpoints; exact Linux host ports and Darwin all-localhost widening; daemon escape warning; reserved/forwarded arguments including Darwin `--bare`; Darwin `CLAUDE_CODE_SIMPLE` scrubbing; environment scrubbing; signals, terminal, status, cleanup, and the 24-hour SIGKILL stale cleanup.

- [ ] **Step 5: Document user resources and the resource-free boundary**

State that Den supplies no skills, plugins, Context Mode, CodeGraph, other MCP servers, or non-security hooks. Explain that user-managed resources in the selected config directory remain available inside Fence. Describe the mandatory Darwin `den-fence` hook and its Bash-only command-check coverage. Mention future resource bundles only as out of scope; do not document a future API as usable.

- [ ] **Step 6: Add troubleshooting and limits**

Cover invalid RepoWolf variables without echoing values, CA file issues, custom-directory privacy failures, socket discovery, Fence capability/application errors, blocked direct Git hosts, unsupported remote container endpoints, no daemon/machine startup, and no VM-grade guarantee.

- [ ] **Step 7: Directly verify documentation**

Run:

```bash
for file in README.md docs/specs/*.md docs/plans/*.md; do
  nix shell "github:NixOS/nixpkgs/c94da05fe469a845461ae503894fad568abeb2a6#pandoc" --command pandoc --from=gfm --to=native "$file" >/dev/null
done
```

Expected: every Markdown file parses as GitHub-Flavored Markdown. Also run the repository whitespace check. Do not add tests that assert README wording; direct parse/whitespace validation and the implementation coverage matrix are the appropriate checks.

- [ ] **Step 8: Commit documentation**

```bash
git add README.md
git commit -m "docs: document claude sandbox usage"
```

---

### Task 15: Enforce Four Native CI Jobs and Run Final Acceptance

**Files:**
- Create: `.github/workflows/checks.yml`
- Modify: `scripts/check-native.sh`

**Interfaces:**
- Produces: four required native jobs, one per supported system.
- Consumes: all package and check outputs from Tasks 1–14.

- [ ] **Step 1: Add an explicit native runner matrix**

Create four non-optional matrix entries:

```yaml
include:
  - system: x86_64-linux
    runner: ubuntu-24.04
  - system: aarch64-linux
    runner: ubuntu-24.04-arm
  - system: x86_64-darwin
    runner: macos-15-intel
  - system: aarch64-darwin
    runner: macos-15
```

Do not use `continue-on-error`, conditional skips, emulation, or cross builds. A missing runner must leave the required job failed/pending, not green.

- [ ] **Step 2: Run evaluation, pure checks, and native enforcement in every job**

Install Nix with sandbox support, verify `builtins.currentSystem` equals the matrix system, run `nix flake check --no-build`, then run `scripts/check-native.sh "$system"`. Upload logs on failure without environment secrets.

- [ ] **Step 3: Directly validate workflow syntax**

Run:

```bash
nix shell "github:NixOS/nixpkgs/c94da05fe469a845461ae503894fad568abeb2a6#actionlint" --command actionlint .github/workflows/checks.yml
```

Expected: PASS. Do not add a bespoke test for static workflow YAML.

- [ ] **Step 4: Run the complete current-host check suite**

Run:

```bash
nix flake check --no-build
scripts/check-native.sh "$(nix eval --impure --raw --expr builtins.currentSystem)"
```

Expected: the full flake evaluates; every current-system non-native check builds in the normal sandbox; native enforcement passes only in its explicit unsandboxed invocation. Do not run plain build-mode `nix flake check`, because it would execute the host-only check in the wrong environment.

- [ ] **Step 5: Verify all four systems evaluate and have mandatory checks**

Run:

```bash
for system in x86_64-linux aarch64-linux x86_64-darwin aarch64-darwin; do
  nix eval --raw ".#packages.$system.claude.drvPath" >/dev/null
  nix eval --raw ".#checks.$system.repowolf-client.drvPath" >/dev/null
  nix eval --raw ".#checks.$system.module-api.drvPath" >/dev/null
  nix eval --raw ".#checks.$system.pure-launcher.drvPath" >/dev/null
  nix eval --raw ".#checks.$system.native-enforcement.drvPath" >/dev/null
done
```

Expected: all twenty evaluations succeed. CI must then report four native green jobs before release.

- [ ] **Step 6: Run acceptance smoke checks**

With valid local/fake RepoWolf values, run:

```bash
nix run .#claude -- --version
```

Expected: the process trace proves Fence starts before Claude. Also verify direct GitHub/GitLab/Bitbucket traffic is denied, RepoWolf fixture API/Git succeeds, custom/default state modes pass, optional containers remain off by default, and no `den` or resource output exists.

- [ ] **Step 7: Commit CI only after local checks pass**

```bash
git add .github/workflows/checks.yml scripts/check-native.sh
git commit -m "ci: require four native sandbox checks"
```

## Plan Self-Review Coverage

### Design section coverage

| Design requirement | Planned work |
|---|---|
| Summary and Goals | Tasks 2–15 build and verify the single `claude -> Fence -> Claude Code` artifact and all integration surfaces. |
| Non-goals | Global Constraints; Tasks 3, 10, 12, and 14 exclude server, dispatcher, other agents, direct credentials/hosts, remote containers, supplied resources, and VM claims. |
| Flake architecture | Tasks 1, 10, 11, and 15 use the Roche Pi `flake-parts`/`import-tree` pattern and four systems. |
| `mkAgentSandbox` | Tasks 2, 4–10 define its manifest, common runtime responsibilities, closure, PATH, policy, containers, process handling, and cleanup. |
| Claude adapter | Tasks 5, 8, 10, and 12 cover package, mandatory args, state paths, user arguments/resources, and the Darwin hook. |
| RepoWolf client | Tasks 3, 4, 12, and 13 cover source pinning, client-only closure, endpoint/token/CA checks, `gh`, and Git SSH. |
| Future resource bundles | Global Constraints and Tasks 8, 10, 12, and 14 keep every future output, option, package, and marketplace mechanism out of MVP. |
| Direct construction | Task 10 implements exact defaults, types, and package behavior. |
| Home Manager module | Task 11 implements and evaluates `homeModules.den`. |
| devenv module | Task 11 implements and evaluates `devenvModules.den`. |
| Runtime flow | Tasks 4–9 implement the ordered validation, policy, environment, launch, signal, status, and cleanup flow; Task 12 proves it end to end. |
| RepoWolf Git routing | Tasks 4, 12, and 13 prove process-local HTTPS rewrite, client-only SSH, cleared helpers, and denied direct transports. |
| Fence policy source/generation | Task 6 vendors the exact snapshot, records provenance/digest, enforces 0.1.58 capabilities, and inserts only runtime values. |
| Fence network policy | Tasks 4, 6, 7, and 13 cover static allows, Git/metadata/telemetry denies, broker host, sockets, ports, and deny precedence. |
| Fence filesystem policy | Tasks 5, 6, 7, 12, and 13 cover closures, worktree/scratch/state, default denials, credentials, symlinks, sockets, and policy immutability. |
| Fence command/macOS hook | Tasks 6, 8, 12, and 13 cover Linux argv enforcement and the mandatory Darwin Bash hook plus its limits. |
| Docker | Tasks 7, 10–12, and 14 cover options, packages, discovery, validation, policy, docs, and disabled default. |
| Podman | Tasks 7, 10–12, and 14 cover options, packages, rootless discovery/ownership, policy, docs, and disabled default. |
| Host ports/security warning | Tasks 7, 11, 12, and 14 cover validation, Linux exact ports, Darwin widening, and daemon warnings. |
| Error handling | Tasks 2, 4–9, and 12 require field-specific corrective errors, secret redaction, fail-closed behavior, and primary-status preservation. |
| Platform behavior | Tasks 1, 6, 8, 13, and 15 require every platform artifact and native behavior without silent omission. |
| Documentation | Task 14 covers every README bullet and uses only the `claude` command. |
| Pure launcher/policy tests | Tasks 4–9 and 12 cover every listed pure behavior with fakes and no live credentials. |
| Native Fence/RepoWolf tests | Task 13 covers every listed host enforcement behavior with local fixtures. |
| Package/closure checks | Tasks 3, 6, 10, and 12 cover required inclusions and forbidden closures/resources. |
| Claude startup checks | Task 12 covers default/custom state, explicit/inherited selection, resource passthrough, and offline startup. |
| Module/API checks | Tasks 10 and 11 cover output identity, missing `den`, defaults, invalid values, integrations, packages, sockets, and ports. |
| Container behavior checks | Tasks 7 and 12 cover discovery, forwarding, ownership, symlinks, exact socket grants, ports, and Darwin widening without a daemon. |
| Four-system matrix | Tasks 13 and 15 require native package, client, module, pure, and enforcement work on all four systems. |

### Acceptance criteria coverage

| # | Acceptance result | Evidence task(s) |
|---|---|---|
| 1 | `nix run .#claude -- --version` uses Fence | Tasks 10, 12, 15 |
| 2 | `claude` exists through every approved integration | Tasks 10, 11, 15 |
| 3 | RepoWolf is the only GitHub API/Git route | Tasks 3, 4, 12, 13 |
| 4 | Direct Git-host traffic is denied | Tasks 6, 13, 15 |
| 5 | Darwin `den-fence` is mandatory and unrelated hooks remain | Tasks 8, 12, 13 |
| 6 | Selected/default state writes and auth/Keychain limits are correct | Tasks 5, 12, 14 |
| 7 | `extraPkgs` stays sandbox-only and cannot shadow RepoWolf | Tasks 10, 11, 12 |
| 8 | Containers are off by default and expose validated sockets only | Tasks 7, 10–12 |
| 9 | Host ports follow Linux/Darwin rules | Tasks 7, 12–14 |
| 10 | Args, terminal, signals, statuses, and cleanup are transparent | Tasks 8, 9, 12 |
| 11 | Full checks pass in the four-system native matrix | Tasks 13, 15 |
| 12 | README covers setup, operation, warnings, troubleshooting, limits | Task 14 |

### Explicit out-of-scope confirmation

No task creates or exposes `resourceBundles`, `claudeResources`, `mkClaudeResourceBundle`, marketplace references, seed-managed Claude resources, supplied skills/plugins, Context Mode, CodeGraph, other MCP servers, or a `den` executable. The only Den-supplied Claude settings are the mandatory security settings required for the macOS `den-fence` hook.
