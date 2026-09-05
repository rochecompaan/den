# Sandboxed Pi Support Implementation Plan

> **Status:** Draft. User approval pending.
>
> **For the implementer:** Use `superpowers:executing-plans` to execute this plan one task at a time. Use `superpowers:test-driven-development` for every behavior change. Use `superpowers:verification-before-completion` before each commit. Read the `nix-config`, `module-size`, and `commit` skills before their matching work. If a test fails unexpectedly, stop and use `superpowers:systematic-debugging`.

**Goal:** Add Pi 0.84.4 as a second sandboxed agent without changing Claude behavior, existing Claude interfaces, native fixture strength, or `packages.default`.

**Architecture:** Keep one generic launcher and one generic sandbox constructor. Put Pi grammar, package hardening, resource order, and Darwin shell enforcement behind explicit adapter seams. Advance the manifest to version 2 so it can describe multiple state bindings and agent-controlled inputs without recognizing Pi by name.

**Toolchain:** Go 1.24, Nix flakes and flake-parts, `buildNpmPackage`, Node.js 22, Fence 0.1.58, Home Manager, nix-darwin, devenv, GitHub Actions.

**Approved design:** `docs/superpowers/specs/2026-09-05-pi-support-design.md`

**Required base:** `810c6eba1685d58497b432747c7f03f6f78eb756`

---

## Delivery rules

- Work only in `/home/roche/projects/den/.worktrees/den-pi-sandbox` on `feat/den-pi-sandbox`.
- Do not read or alter protected Claude worktrees, PR #1, EC2 Mac state, or credential values.
- Keep every task green before its commit.
- Commit only files named by the active task. Tasks 6 and 7 are one documented security-atomic checkpoint, so Task 7 also commits Task 6's listed files.
- Use Conventional Commit subjects shown below.
- Do not push.
- Do not start implementation until this committed plan has explicit user approval.
- Preserve `packages.default = packages.claude` on all four systems.
- Run native checks only through `scripts/check-native.sh "$system"` as the invoking host user.
- Do not run native fixtures from a Nix build sandbox or as root.

### Git-flake staging rule

This repository is a Git flake. Before every `nix build`, `nix flake`, or native-script command in a task:

1. stage every active-task file that exists;
2. run `git diff --cached --name-only`;
3. confirm the index contains only active-task files;
4. run the command;
5. re-stage changed active-task files before the GREEN command.

A RED check file must be staged before its first flake evaluation. Staging is not a commit. Keep the task's final commit scoped to its listed files.

## Fixed implementation contracts

### Manifest version 2

Use these Go types in `internal/manifest`:

```go
const CurrentVersion = 2

type Manifest struct {
    Version               int             `json:"version"`
    Platform              string          `json:"platform"`
    FenceExecutable       string          `json:"fenceExecutable"`
    RepoWolfClientDir     string          `json:"repoWolfClientDir"`
    BasePolicy            string          `json:"basePolicy"`
    ClosurePathsFile      string          `json:"closurePathsFile"`
    ScratchRoot           string          `json:"scratchRoot"`
    ACLProbe              []string        `json:"aclProbe"`
    ProtectedPathPatterns []string        `json:"protectedPathPatterns"`
    PathEntries           []string        `json:"pathEntries"`
    Agent                 Agent           `json:"agent"`
    StateBindings         []StateBinding  `json:"stateBindings"`
    Docker                ContainerConfig `json:"docker"`
    Podman                ContainerConfig `json:"podman"`
}

type Agent struct {
    Name             string            `json:"name"`
    Executable       string            `json:"executable"`
    CommandName      string            `json:"commandName"`
    ArgumentPolicy   string            `json:"argumentPolicy"`
    MandatoryArgs    []string          `json:"mandatoryArgs"`
    ResourceArgs     []string          `json:"resourceArgs"`
    ReservedFlags    []string          `json:"reservedFlags"`
    ReservedCommands []string          `json:"reservedCommands"`
    Environment      AgentEnvironment  `json:"environment"`
    PackageDirectory *EnvironmentValue `json:"packageDirectory,omitempty"`
    SecurityAdapter  *SecurityAdapter  `json:"securityAdapter,omitempty"`
}

type AgentEnvironment struct {
    Scrub []string          `json:"scrub"`
    Set   map[string]string `json:"set"`
}

type EnvironmentValue struct {
    Name  string `json:"name"`
    Value string `json:"value"`
}

type SecurityAdapter struct {
    Kind      string   `json:"kind"`
    Path      string   `json:"path"`
    Arguments []string `json:"arguments"`
}

type StateBinding struct {
    Name                 string        `json:"name"`
    ExplicitPath         *string       `json:"explicitPath"`
    InheritedEnvironment string        `json:"inheritedEnvironment"`
    DefaultPath          string        `json:"defaultPath"`
    DefaultWritablePaths []string      `json:"defaultWritablePaths"`
    Exports              []StateExport `json:"exports"`
}

type StateExport struct {
    Kind          string `json:"kind"`
    Name          string `json:"name"`
    ExportDefault bool   `json:"exportDefault"`
}
```

Allowed `StateExport.Kind` values are `environment` and `argument`. An argument export emits `Name`, then the selected path. An environment export sets `Name` to the selected path.

`ExportDefault` controls delivery for a Den-owned default. Claude sets it to `false`. Pi sets it to `true` for both state bindings.

Claude has one `config` binding. It exports `CLAUDE_CONFIG_DIR` only when the selected source is explicit or inherited. Its empty default path retains the existing default writable paths.

Pi has these bindings:

| Binding | Inherited source | Default | Exports |
| --- | --- | --- | --- |
| `agent` | `PI_CODING_AGENT_DIR` | `.local/state/den/pi/agent` | environment `PI_CODING_AGENT_DIR` |
| `sessions` | `PI_CODING_AGENT_SESSION_DIR` | `.local/state/den/pi/sessions` | argument `--session-dir`; environment `PI_CODING_AGENT_SESSION_DIR` |

Pi does not consume `PI_CODING_AGENT_SESSION_DIR` itself. Den also installs `--session-dir`. The patch captures the environment value once before extensions load.

### Nix adapter boundary

`nix/lib/mk-agent-sandbox.nix` receives this adapter shape:

```nix
adapter = {
  runtimePackages = [ ];
  closureOnlyPackages = [ ];
  output = {
    packageName = "pi";
    commandName = "pi";
    manifestName = "pi-manifest.json";
    mainProgram = "pi";
  };
  agent = {
    name = "pi";
    executable = "/nix/store/.../bin/den-pi-agent";
    argumentPolicy = "pi-0.84.4";
    mandatoryArgs = [ ];
    resourceArgs = [ ];
    reservedFlags = [ ];
    reservedCommands = [ ];
    environment = { scrub = [ ]; set = { }; };
    packageDirectory = null;
    securityAdapter = null;
  };
  stateBindings = [ ];
};
```

The shared constructor may inspect this schema. It must not branch on `agent.name`, `commandName`, or the string `pi`.

`mkClaude` supplies the same schema with Claude values. Its public arguments and resulting behavior remain unchanged.

### Pi package pin

Use these fixed values:

```text
package:    @earendil-works/pi-coding-agent
version:    0.84.4
Node:       >=22.19.0
tarball:    https://registry.npmjs.org/@earendil-works/pi-coding-agent/-/pi-coding-agent-0.84.4.tgz
tar hash:   sha256-W852bRnDzroY8/uq2RxEnJ+dc5gfnjQA7O+TIAbwaWg=
lock hash:  sha256-/xfQaHHRD9Riiv+hqSHfFzvx+GBeByzCqgpO5Oi0cc4=
npm hash:   sha256-rSUYLw/RoIZ2f6gMwpSUdDqGECFUnm0KnNu/uCLYbpE=
```

The published `npm-shrinkwrap.json` omits integrity values for Pi sibling packages. Commit a normalized `package-lock.json` and assert its fixed hash. Stage the tarball with that lock before `buildNpmPackage` computes the fixed dependency closure.

Run the packaged executable through the unbundled `dist/cli.js`. Do not use `dist/bundle/cli.js`. This keeps the fixed patch on the readable unbundled modules authoritative.

### Module size limits

Keep new production files near 200 lines. Split any file before it reaches 400 lines. Do not add large Pi suites to `tests/native/native_test.go` or large Pi branches to `internal/launch/lifecycle.go`.

---

## Task 1: Make sandbox output naming adapter-driven

**Files:**
- Create: `nix/check-support/agent-adapter.nix`
- Create: `modules/checks/agent-adapter.nix`
- Modify: `nix/lib/mk-agent-sandbox.nix`
- Modify: `nix/lib/mk-claude.nix`
- Modify: `nix/check-support/package-api.nix`
- Modify: `nix/check-support/claude-adapter.nix`

### Step 1: Write the failing adapter contract check

Create a fake adapter named `test-agent`. Assert that the constructor derives these outputs only from `adapter.output`:

- package name `test-agent`;
- wrapper `bin/test-agent`;
- manifest file `test-agent-manifest.json`;
- `meta.mainProgram = "test-agent"`.

Also assert that the generated file set contains no unrequested `claude` wrapper or manifest.

Run:

```sh
system=$(nix eval --impure --raw --expr builtins.currentSystem)
nix build ".#checks.$system.agent-adapter" --no-link --print-build-logs
```

Expected: the staged check is RED because the constructor still hard-codes Claude output names.

### Step 2: Add the generic output fields

Update `mk-agent-sandbox.nix` to use only `adapter.output.packageName`, `commandName`, `manifestName`, and `mainProgram` for output naming.

Validate every field as a non-empty safe basename. Reject `/`, newlines, `.` and `..`. Reject an output manifest name that does not end in `.json`.

Do not change policy generation, RepoWolf setup, Fence invocation, terminal handling, or cleanup.

### Step 3: Adapt Claude without changing its API

Populate the new output block in `mk-claude.nix`. Keep the command `claude`, manifest name `claude-manifest.json`, main program `claude`, and package name `claude`.

### Step 4: Run focused and regression checks

```sh
system=$(nix eval --impure --raw --expr builtins.currentSystem)
nix build ".#checks.$system.agent-adapter" --no-link --print-build-logs
nix build ".#checks.$system.package-api" --no-link --print-build-logs
nix build ".#checks.$system.claude-adapter" --no-link --print-build-logs
nix build ".#packages.$system.claude" --no-link --print-build-logs
```

Expected: all pass. The Claude package output and wrapper contract remain unchanged.

### Step 5: Commit

```sh
git add nix/check-support/agent-adapter.nix modules/checks/agent-adapter.nix \
  nix/lib/mk-agent-sandbox.nix nix/lib/mk-claude.nix \
  nix/check-support/package-api.nix nix/check-support/claude-adapter.nix
git commit -m "refactor: make sandbox outputs adapter-driven"
```

---

## Task 2: Add manifest v2 and multi-directory state lifecycle

**Files:**
- Create: `internal/configdir/bindings.go`
- Create: `internal/configdir/bindings_test.go`
- Create: `internal/launch/state.go`
- Create: `internal/launch/state_test.go`
- Modify: `internal/manifest/manifest.go`
- Modify: `internal/manifest/manifest_test.go`
- Modify: `internal/configdir/configdir.go`
- Modify: `internal/configdir/configdir_test.go`
- Modify: `internal/launch/lifecycle.go`
- Modify: `internal/launch/lifecycle_test.go`
- Modify: `internal/policy/policy.go`
- Modify: `internal/policy/policy_test.go`
- Modify: `nix/lib/mk-agent-sandbox.nix`
- Modify: `nix/lib/mk-claude.nix`
- Modify: `tests/claude-startup-runtime-manifest.sh`
- Modify: `nix/check-support/pure-launcher-linux.nix`
- Modify: `nix/check-support/pure-launcher-darwin.nix`

### Step 1: Write failing manifest v2 tests

Cover:

- exact version 2 acceptance;
- version 1 and unknown version rejection;
- complete validation before state or RepoWolf calls;
- duplicate state names;
- empty or unsafe command, export, environment, and security-adapter names;
- unknown export kinds and malformed optional security adapters;
- invalid reserved, mandatory, resource, and security argument values;
- bindings without exports;
- both state directories resolving to the same canonical path;
- parent-child overlap in either order;
- default paths writable only when selected;
- default paths denied after an explicit or inherited selection;
- mixed default-agent and custom-session selection;
- mixed custom-agent and default-session selection;
- both mixed cases with reversed binding order;
- exact Claude read/write parity for default and custom selection;
- unselected defaults appear in `denyWrite` but not `denyRead`;
- protected paths remain in both `denyRead` and `denyWrite`;
- final-component symbolic links;
- sibling-prefix paths that do not overlap;
- manifest paths containing newlines;
- rollback of all unchanged new directories when a later binding fails;
- commit of every selected directory only after the child starts;

Run:

```sh
go test ./internal/manifest ./internal/configdir ./internal/launch ./internal/policy \
  -run 'Test(LoadVersion2|ValidateStateBinding|PlanBindings|LaunchStateBindings|MixedStatePolicy|ClaudeStatePolicyParity)' -count=1
```

Expected: RED because version 2 and binding plans do not exist.

### Step 2: Parse and validate the complete manifest

Implement the fixed version 2 types. Reject unknown JSON fields. Validate all manifest fields before any filesystem mutation.

Keep structural validation in `internal/manifest`. Do not put agent-specific policy tables there. Require a non-empty safe policy identifier and let `internal/arguments` resolve it.

### Step 3: Plan all state paths before opening any

Add:

```go
func PlanBindings(specs []manifest.StateBinding, inherited map[string]string, runtimeHome string) (BindingPlan, error)
func (p BindingPlan) Open(platform string, acl configdir.ACLValidator) ([]*configdir.Handle, error)
```

`PlanBindings` selects explicit, inherited, or default paths from the original inherited environment. It performs lexical and canonical overlap checks without creating state directories.

When a non-default source wins, add its Den-owned defaults to the deny set. When the default wins, grant only that binding's declared default paths.

`Open` uses the existing secure directory logic. If any later binding fails, close all earlier handles and return the first error.

Keep `configdir.Select` as a compatibility wrapper for existing focused tests until all callers use `PlanBindings`. Remove the wrapper before Task 2 ends if nothing uses it.

### Step 4: Apply exports without recognizing an agent

`internal/launch/state.go` converts opened bindings into:

```go
type StateInputs struct {
    Environment      map[string]string
    Arguments        []string
    WritablePaths    []string
    DeniedWritePaths []string
}
```

Preserve binding order and export order. Append state arguments before user arguments.

Replace `policy.Dynamic.CustomMode` and `DefaultStatePaths` with explicit aggregate inputs. Pass `WritablePaths` as state grants. Pass `DeniedWritePaths` through a separate `policy.Dynamic.DeniedWritePaths` field that contributes only to Fence `denyWrite`.

Keep host credentials, Pi state, and other read-and-write denials in `policy.Dynamic.ProtectedPaths`. Do not merge unselected Den defaults into that field.

This policy interface must represent each binding independently. A default binding cannot grant a different binding's denied default.

For Claude, preserve the existing default behavior exactly. A custom Claude binding still denies writes to its unselected default, but does not newly deny reads. Test the generated `allowRead`, `allowWrite`, `denyRead`, and `denyWrite` sets for both default and custom Claude launches. Do not export `CLAUDE_CONFIG_DIR` for Claude's default binding.

### Step 5: Emit v2 from Nix

Update `mk-agent-sandbox.nix` to serialize the fixed v2 schema. Derive the manifest command name from `adapter.output.commandName`.

Update `mk-claude.nix` to declare its one `config` binding and existing protected paths. Represent Darwin's settings file as the optional security adapter without changing its effective argv order.

### Step 6: Run focused and Claude regression checks

```sh
go test ./internal/manifest ./internal/configdir ./internal/launch ./internal/policy -count=1
go test ./... -count=1
system=$(nix eval --impure --raw --expr builtins.currentSystem)
nix build ".#checks.$system.launcher-unit" --no-link --print-build-logs
nix build ".#checks.$system.pure-launcher" --no-link --print-build-logs
nix build ".#packages.$system.claude" --no-link --print-build-logs
```

Expected: all pass. Existing Claude precedence, path security, cleanup, exit, signal, and terminal tests remain green.

### Step 7: Commit

```sh
git add internal/manifest internal/configdir internal/launch internal/policy \
  nix/lib/mk-agent-sandbox.nix nix/lib/mk-claude.nix \
  tests/claude-startup-runtime-manifest.sh \
  nix/check-support/pure-launcher-linux.nix \
  nix/check-support/pure-launcher-darwin.nix
git commit -m "feat: support manifest v2 state bindings"
```

---

## Task 3: Add generic child inputs and Pi argument validation

**Files:**
- Create: `internal/arguments/arguments.go`
- Create: `internal/arguments/arguments_test.go`
- Create: `internal/piargs/piargs.go`
- Create: `internal/piargs/piargs_test.go`
- Create: `internal/launch/inputs.go`
- Create: `internal/launch/inputs_test.go`
- Modify: `internal/environment/environment.go`
- Modify: `internal/environment/environment_test.go`
- Modify: `internal/launch/lifecycle.go`
- Modify: `internal/launch/lifecycle_test.go`
- Modify: `internal/manifest/manifest.go`
- Modify: `internal/manifest/manifest_test.go`
- Modify: `nix/lib/mk-claude.nix`

### Step 1: Write the failing policy tests

Add table tests for every row in the approved argument policy. Include:

- split, equals, attached-value, and combined-short reserved forms;
- missing values and repeated options;
- `--session-dir`, `--export`, and every resource source flag;
- UUID and hexadecimal partial IDs for `--session` and `--fork`;
- path-like, sibling-prefix, empty, and malformed session values;
- valid and invalid UUIDs for `--session-id`;
- permitted `--continue`, `--resume`, `--no-session`, trust, and discovery flags;
- `install`, `remove`, `uninstall`, `update`, `list`, and `config` as the first user token;
- the same commands after the option terminator;
- an unknown extension flag;
- nested wrapper arguments;
- parity fixtures taken from Pi 0.84.4's `dist/cli/args.js`.

Run:

```sh
go test ./internal/arguments ./internal/piargs ./internal/launch \
  -run 'Test(Pi|ValidateArguments|BuildChildInputs)' -count=1
```

Expected: RED because the packages do not exist.

### Step 2: Implement policy dispatch

Add:

```go
func Validate(policy string, reservedFlags, reservedCommands, userArgs []string) error
```

`internal/arguments` dispatches by the manifest policy string. `internal/piargs` owns Pi 0.84.4 grammar and its security table.

Each policy validates the manifest's reserved flags and commands against its exact compiled table. A mismatch is an invalid manifest.

Validate only user arguments. Mandatory adapter and state arguments are trusted manifest inputs and are already schema-validated.

Reject a blocked package token before mandatory arguments are prepended. Preserve Pi's `--` behavior. Treat unknown options as Pi would so configured extensions retain their flags.

Keep Claude's current reserved-argument results and error text unless a test proves a security defect.

### Step 3: Build controlled inputs generically

`internal/launch/inputs.go` combines:

1. the current safe base environment;
2. the existing GitHub credential and unsafe Git or SSH scrubbing;
3. `Agent.Environment.Scrub`;
4. RepoWolf variables;
5. state environment exports;
6. `Agent.Environment.Set`;
7. `Agent.PackageDirectory`.

Use a single exact-key overwrite rule. Do not read an agent-specific environment variable in `internal/launch`.

Preserve provider credentials and ordinary proxy variables as runtime values. Do not add a provider allowlist or change Claude's existing scrub table.

Build argv in this order:

1. manifest mandatory arguments;
2. optional security-adapter arguments;
3. immutable resource arguments;
4. state argument exports;
5. user arguments.

### Step 4: Move argument validation before mutable launch work

The launch sequence becomes:

1. parse and validate the complete manifest;
2. validate user arguments and package commands;
3. load RepoWolf configuration;
4. resolve the invoking-account and inherited runtime homes;
5. select and validate all state bindings;
6. resolve optional container sockets;
7. run the Linux Fence feature preflight when required;
8. build the controlled child environment;
9. create private policy and scratch directories;
10. prepare the RepoWolf CA file;
11. generate the strict per-launch Fence policy;
12. revalidate every state binding;
13. revalidate the optional security adapter;
14. start Fence around the immutable agent executable;
15. commit new state directories after process start;
16. return the agent or Fence status.

Verify invalid Pi arguments create no state, policy, scratch, RepoWolf, or agent process artifacts.

### Step 5: Run focused and full Go tests

```sh
go test ./internal/arguments ./internal/piargs ./internal/environment ./internal/launch -count=1
go test ./... -count=1
```

Expected: all pass, including unchanged Claude tests.

### Step 6: Commit

```sh
git add internal/arguments internal/piargs internal/launch \
  internal/environment internal/manifest nix/lib/mk-claude.nix
git commit -m "feat: validate adapter-controlled launch inputs"
```

---

## Task 4: Package and harden Pi 0.84.4

**Files:**
- Create: `nix/packages/pi-coding-agent.nix`
- Create: `nix/packages/pi-0.84.4-package-lock.json`
- Create: `patches/pi-0.84.4-den-hardening.patch`
- Create: `nix/check-support/pi-package.nix`
- Create: `nix/check-support/fixtures/pi/hostile-package-extension.ts`
- Create: `modules/checks/pi-package.nix`

### Step 1: Materialize the reviewed fixed lock

Extract the content-addressed lock payload from Appendix A of this committed plan:

```sh
python3 - <<'PY'
from pathlib import Path
import base64
import gzip

plan = Path("docs/plans/2026-09-05-pi-support.md").read_text()
block = plan.rsplit("<!-- PI_LOCK_GZIP_BASE64_BEGIN -->", 1)[1]
block = block.split("<!-- PI_LOCK_GZIP_BASE64_END -->", 1)[0]
payload = "".join(
    line for line in block.splitlines()
    if line and not line.startswith("```")
)
lock = gzip.decompress(base64.b64decode(payload))
Path("nix/packages/pi-0.84.4-package-lock.json").write_bytes(lock)
PY
```

Do not regenerate this lock from current registry ranges. Registry changes can select a different transitive closure.

Verify:

```sh
nix hash file --type sha256 --sri nix/packages/pi-0.84.4-package-lock.json
```

Expected exactly:

```text
sha256-/xfQaHHRD9Riiv+hqSHfFzvx+GBeByzCqgpO5Oi0cc4=
```

Also parse the lock and fail if any non-root, non-link package lacks `resolved` or `integrity`, or uses a Git dependency. Stop if decoding or the hash check fails.

### Step 2: Write the failing package check

The check must prove:

- exact package name and version;
- selected Node version is exactly checked as `>=22.19.0`;
- tar, lock, npm dependency, and patch hash assertions;
- `bin/pi` executes patched unbundled `dist/cli.js`;
- the runtime closure contains Pi metadata, assets, and every required dependency;
- the executable starts with a controlled empty host `PATH`;
- no runtime network fetcher is in the wrapper path;
- no credential file or value enters the derivation;
- all six package commands fail before a hostile project extension writes a marker;
- `DefaultPackageManager` mutation entry points fail after `PI_OFFLINE` is changed or deleted;
- settings and managed package directories remain byte-identical;
- a resource reload cannot trigger package installation;
- absent npm and Git sources are skipped without requests;
- normal `--version` and `--mode rpc` startup still work in isolated temporary state.

Run:

```sh
system=$(nix eval --impure --raw --expr builtins.currentSystem)
nix build ".#checks.$system.pi-package" --no-link --print-build-logs
```

Expected: the staged check is RED because the Pi derivation and hardening behavior do not exist.

### Step 3: Write the fixed hardening patch

Patch these unbundled files:

- `dist/main.js`;
- `dist/core/package-manager.js`;
- `dist/core/agent-session-runtime.js`.

Add an immutable build constant. It must not consult `PI_OFFLINE` or another mutable environment value.

At the start of `main(args)`, reject package dispatch when the first process argument is `install`, `remove`, `uninstall`, `update`, `list`, or `config`. Perform this check before trust and resource bootstrap.

In `DefaultPackageManager`:

- reject all public mutation methods;
- reject runtime-visible internal npm and Git mutation helpers;
- skip update checks;
- skip absent npm and Git sources without installation or network access;
- preserve read-only resolution of existing local sources;
- preserve resource order and collision diagnostics.

At the first line of `AgentSessionRuntime.switchSession`:

- use the startup-captured canonical `PI_CODING_AGENT_SESSION_DIR`;
- recheck root device and inode identity;
- reject an absent or non-regular target;
- reject target symlinks and canonical escapes;
- open the canonical target path, not the untrusted alias;
- use path-component containment, not string prefixes;
- perform checks before `emitBeforeSwitch`, file reads, or runtime teardown.

This one boundary must cover interactive, RPC, and extension callers.

Compute the final patch hash:

```sh
nix hash file --type sha256 --sri patches/pi-0.84.4-den-hardening.patch
```

Pin that exact value in `pi-coding-agent.nix`. Add a check that fails if the patch changes without updating the pin.

### Step 4: Build Pi reproducibly

`pi-coding-agent.nix` must:

- fetch the exact npm tarball with the fixed tar hash;
- stage the extracted source with the committed normalized lock;
- assert the lock hash;
- apply the fixed patch;
- use `pkgs.nodejs_22` and `buildNpmPackage`;
- assert `lib.versionAtLeast pkgs.nodejs_22.version "22.19.0"` during evaluation;
- compare the packaged `node --version` with `22.19.0` in the executable check;
- use the fixed npm dependency hash;
- install a wrapper that executes `node dist/cli.js`;
- keep Pi's package root and assets in the runtime closure;
- expose passthru paths needed by native fixtures.

### Step 5: Run the package check twice

```sh
system=$(nix eval --impure --raw --expr builtins.currentSystem)
nix build ".#checks.$system.pi-package" --no-link --print-build-logs
nix build ".#checks.$system.pi-package" --rebuild --no-link --print-build-logs
```

Expected: both pass. The second build performs no network access outside fixed-output fetches.

### Step 6: Commit

```sh
git add nix/packages/pi-coding-agent.nix \
  nix/packages/pi-0.84.4-package-lock.json \
  patches/pi-0.84.4-den-hardening.patch \
  nix/check-support/pi-package.nix \
  nix/check-support/fixtures/pi/hostile-package-extension.ts \
  modules/checks/pi-package.nix
git commit -m "feat: package hardened Pi 0.84.4"
```

---

## Task 5: Build the Pi adapter and immutable resource interface

**Files:**
- Create: `nix/lib/pi-options.nix`
- Create: `nix/lib/pi-resources.nix`
- Create: `nix/lib/mk-pi.nix`
- Create: `nix/pi/den-pi-agent.sh`
- Create: `nix/check-support/pi-resources.nix`
- Create: `nix/check-support/pi-adapter.nix`
- Create: `nix/check-support/fixtures/pi/resources/extensions/first.ts`
- Create: `nix/check-support/fixtures/pi/resources/extensions/second.ts`
- Create: `nix/check-support/fixtures/pi/resources/skills/first/SKILL.md`
- Create: `nix/check-support/fixtures/pi/resources/skills/second/SKILL.md`
- Create: `nix/check-support/fixtures/pi/resources/prompts/first.md`
- Create: `nix/check-support/fixtures/pi/resources/prompts/second.md`
- Create: `nix/check-support/fixtures/pi/resources/themes/first.json`
- Create: `nix/check-support/fixtures/pi/resources/themes/second.json`
- Create: `modules/checks/pi-resources.nix`
- Create: `modules/checks/pi-adapter.nix`
- Modify: `nix/lib/mk-agent-sandbox.nix`

### Step 1: Write failing option and order checks

The direct constructor is:

```nix
mkPi {
  agentDir = null;
  sessionDir = null;
  extraPkgs = [ ];
  resources = {
    extensions = [ ];
    packages = [ ];
    skills = [ ];
    promptTemplates = [ ];
    themes = [ ];
  };
  docker = { };
  podman = { };
}
```

Reject unknown keys, relative state paths, mutable string resources, duplicate canonical paths within one class, and `extraPkgs` that expose `bin/pi`.

Validate realized resource shapes:

- extensions are `.ts`, `.js`, or Pi-compatible extension directories;
- skills are `SKILL.md` files or Pi-compatible skill directories;
- prompt templates are Markdown files or directories;
- themes are JSON files or directories;
- packages contain valid Pi package metadata or convention directories;
- configured package runtime dependencies already exist in the store closure.

Freeze these independent effective sequences:

1. Extensions: direct extensions, configured package extensions, then enabled ambient sources.
2. Skills: configured package skills, enabled ambient sources, then direct skills.
3. Prompt templates: configured package templates, enabled ambient sources, then direct templates.
4. Themes: configured package themes, enabled ambient sources, then direct themes.

For each type, assert first-winner behavior and Pi's loser diagnostic. Do not assert a cross-type order.

Run:

```sh
system=$(nix eval --impure --raw --expr builtins.currentSystem)
nix build ".#checks.$system.pi-resources" --no-link --print-build-logs
nix build ".#checks.$system.pi-adapter" --no-link --print-build-logs
```

Expected: the staged checks are RED because `mkPi` and resource validation do not exist.

### Step 2: Normalize resources in one focused module

`pi-resources.nix` validates entries and returns:

```nix
{
  resourceArgs = [ ];
  closureInputs = [ ];
  diagnosticsCheck = derivation;
}
```

Map direct inputs to `--extension`, `--skill`, `--prompt-template`, and `--theme`. Map each configured Pi package to a local store-path `--extension` source.

Preserve each option list exactly. Let Pi retain its documented per-type subgroup order.

The compatibility check must capture Pi's type, winner, and loser diagnostic for a same-name collision. That collision is not fatal.

Duplicate canonical paths in one configured class are fatal. A missing or malformed realized resource must fail before the wrapper is available.

### Step 3: Construct the Pi adapter

`mk-pi.nix` must:

- keep the hardened Pi package in `closureOnlyPackages`, not the child `PATH`;
- assert exact Pi version 0.84.4;
- select the fixed agent and session bindings;
- set `PI_OFFLINE=1`;
- set `PI_PACKAGE_DIR` to Pi's immutable runtime root, including its package metadata and assets;
- export the selected session path through `--session-dir` and `PI_CODING_AGENT_SESSION_DIR`;
- scrub inherited `PI_CODING_AGENT_DIR`, `PI_CODING_AGENT_SESSION_DIR`, `PI_PACKAGE_DIR`, and `PI_OFFLINE` before reinstalling controlled values;
- emit exact reserved flag and package-command tables;
- prepend configured resource arguments;
- use `argumentPolicy = "pi-0.84.4"`;
- use the shared Fence package and policy path;
- avoid Pi-specific branches in the shared constructor.

The agent shim sets only Pi runtime inputs that depend on Fence-created paths or RepoWolf. It then executes the hardened unbundled Pi entry.

### Step 4: Protect both host homes

Extend the manifest path data so policy generation denies these paths under both the invoking-account and inherited runtime homes:

- `.pi/agent`;
- `.agents` and `.agents/skills`;
- canonical and symbolic-link aliases of both homes.

Reject launch when the invoking-account home cannot be resolved. Treat the inherited runtime home as a separate input even when both homes currently match.

Do not deny trusted repository `.pi` or `.agents/skills` paths inside the validated workspace.

Keep filesystem deny rules authoritative over writable grants.

### Step 5: Verify the adapter

```sh
system=$(nix eval --impure --raw --expr builtins.currentSystem)
nix build ".#checks.$system.pi-resources" --no-link --print-build-logs
nix build ".#checks.$system.pi-adapter" --no-link --print-build-logs
nix build ".#checks.$system.pi-package" --no-link --print-build-logs
nix build ".#packages.$system.claude" --no-link --print-build-logs
```

Expected: all pass. A permitted `--no-*` flag omits only ambient sources and leaves mandatory resources loaded.

### Step 6: Commit

```sh
git add nix/lib/pi-options.nix nix/lib/pi-resources.nix nix/lib/mk-pi.nix \
  nix/pi/den-pi-agent.sh nix/check-support/pi-resources.nix \
  nix/check-support/pi-adapter.nix nix/check-support/fixtures/pi/resources \
  modules/checks/pi-resources.nix modules/checks/pi-adapter.nix \
  nix/lib/mk-agent-sandbox.nix
git commit -m "feat: add immutable Pi sandbox adapter"
```

---

## Task 6: Expose Pi through package, library, and modules

**Files:**
- Create: `modules/lib/pi.nix`
- Create: `modules/packages/pi.nix`
- Create: `nix/check-support/pi-package-api.nix`
- Create: `nix/check-support/pi-module-api.nix`
- Create: `modules/checks/pi-package-api.nix`
- Create: `modules/checks/pi-module-api.nix`
- Modify: `nix/lib/module-options.nix`
- Modify: `modules/home/den.nix`
- Modify: `modules/devenv/den.nix`
- Modify: `nix/check-support/package-api.nix`

### Step 1: Write failing public API checks

Assert:

- `packages.${system}.pi` exists and its main program is `pi`;
- `lib.${system}.mkPi` constructs a package directly;
- `packages.default` remains the same derivation as `packages.claude`;
- Home Manager and devenv expose `programs.den.pi`;
- `enable`, `agentDir`, `sessionDir`, `extraPkgs`, resources, Docker, and Podman map exactly to `mkPi`;
- enabling Claude and Pi installs both commands without collisions;
- disabling Pi changes no Claude module output;
- Pi remains disabled by default;
- every resource list keeps its configured order.

Run:

```sh
system=$(nix eval --impure --raw --expr builtins.currentSystem)
nix build ".#checks.$system.pi-package-api" --no-link --print-build-logs
nix build ".#checks.$system.pi-module-api" --no-link --print-build-logs
```

Expected: the staged checks are RED because the Pi outputs and module options do not exist.

### Step 2: Add focused module option helpers

Refactor common container options into a private helper inside `module-options.nix`. Keep Claude option names, defaults, descriptions, and assertions unchanged.

Add `programs.den.pi` with the direct constructor fields from Task 5. Resource list order must survive module evaluation.

### Step 3: Add package and library outputs

`modules/packages/pi.nix` publishes `packages.pi`. `modules/lib/pi.nix` publishes `lib.mkPi` with the same system-scoped pattern as Claude.

Do not modify the default package assignment.

### Step 4: Extend the existing modules

Update `modules/home/den.nix` and `modules/devenv/den.nix`. Each enabled Pi block calls the same `mkPi` interface.

Keep the wrappers thin. Do not duplicate validation from `pi-options.nix` or `module-options.nix`.

### Step 5: Run all API checks

```sh
system=$(nix eval --impure --raw --expr builtins.currentSystem)
nix build ".#checks.$system.package-api" --no-link --print-build-logs
nix build ".#checks.$system.module-api" --no-link --print-build-logs
nix build ".#checks.$system.pi-package-api" --no-link --print-build-logs
nix build ".#checks.$system.pi-module-api" --no-link --print-build-logs
nix build ".#packages.$system.default" --no-link --print-build-logs
```

Expected: all pass and `packages.default` is still Claude.

### Step 6: Keep the public surface private until Darwin enforcement exists

Do not commit Task 6 alone. Leave its verified changes in the worktree and continue directly to Task 7.

This checkpoint must remain uncommitted because its public Pi outputs do not yet contain the mandatory Darwin security extension. Task 7 commits both tasks together after that enforcement passes.

---

## Task 7: Add the mandatory Darwin shell security extension

**Files:**
- Create: `nix/pi/den-pi-security.ts`
- Create: `nix/check-support/pi-security-extension.nix`
- Create: `nix/check-support/fixtures/pi/replace-shell-tools.ts`
- Create: `modules/checks/pi-security-extension.nix`
- Modify: `nix/lib/mk-pi.nix`
- Modify: `nix/check-support/fence-capabilities.nix`
- Modify: `nix/check-support/pi-adapter.nix`

### Step 1: Write failing helper and extension checks

Cover:

- a Claude-compatible `PreToolUse` request containing command and current directory;
- Fence's exact successful empty no-change output;
- explicit deny response;
- rewritten-command response;
- malformed non-empty output;
- spawn failure and non-zero status;
- missing `FENCE_SANDBOX=1`;
- no HTTP or SOCKS proxy listener during helper evaluation;
- Fence capabilities `claudePreToolUse`, `denFenceTmpdir`, `strictDenyRead`, and platform enforcement;
- unchanged Fence 0.1.58 source and Den patch hashes;
- replacement attempts for `bash` and `user_bash`;
- security-extension ownership after all ordinary resources load.

Run on Darwin:

```sh
system=$(nix eval --impure --raw --expr builtins.currentSystem)
nix build ".#checks.$system.pi-security-extension" --no-link --print-build-logs
scripts/check-native.sh "$system"
```

Expected: the staged check is RED because the Pi extension and helper behavior do not exist.

### Step 2: Implement one fail-closed command evaluator

In `den-pi-security.ts`, add one pure adapter function that:

1. verifies `FENCE_SANDBOX=1`;
2. sends `{ hook_event_name: "PreToolUse", tool_name: "Bash", tool_input: { command, cwd }, cwd }`;
3. invokes `${fence}/bin/fence --claude-pre-tool-use --settings "$DEN_FENCE_POLICY_FILE"` synchronously;
4. accepts only exit zero with empty stdout, which is Fence's no-change result;
5. rejects denial, rewriting, malformed non-empty output, and process errors.

Do not start a nested Fence manager. Do not accept a rewritten command.

### Step 3: Own both shell entry points through Pi's first-winner rules

For built-in `bash`, use `createBashTool` with a `spawnHook`. Recreate the tool at execution time with `ctx.cwd` so the helper sees the current working directory.

For `user_bash`, wrap `createLocalBashOperations`. Run the same evaluator before delegating to local operations.

Prepend the security extension before every ordinary extension source. Pi 0.84.4 keeps the first tool name and the first non-empty `user_bash` result.

Keep the security extension outside ordinary configured extension order. Compatibility tests must prove that later user and project handlers cannot take either entry point.

Do not add this extension on Linux.

### Step 4: Revalidate the extension inputs

Put the extension, fixed Fence binary, and policy path in protected manifest inputs. Revalidate their type, canonical path, and identity immediately before Fence starts Pi.

### Step 5: Verify Darwin and Linux adapter shapes

```sh
system=$(nix eval --impure --raw --expr builtins.currentSystem)
nix build ".#checks.$system.pi-adapter" --no-link --print-build-logs
nix build ".#checks.$system.pi-security-extension" --no-link --print-build-logs
scripts/check-native.sh "$system"
```

On Linux, evaluate both package shapes and assert the security extension is absent from mandatory arguments.

Expected: all checks pass. Fence remains the shared patched 0.1.58 package with the approved source and patch hashes.

### Step 6: Commit the public API with mandatory enforcement

```sh
git add modules/lib/pi.nix modules/packages/pi.nix \
  nix/check-support/pi-package-api.nix nix/check-support/pi-module-api.nix \
  modules/checks/pi-package-api.nix modules/checks/pi-module-api.nix \
  nix/lib/module-options.nix modules/home/den.nix modules/devenv/den.nix \
  nix/check-support/package-api.nix \
  nix/pi/den-pi-security.ts nix/check-support/pi-security-extension.nix \
  nix/check-support/fixtures/pi/replace-shell-tools.ts \
  modules/checks/pi-security-extension.nix nix/lib/mk-pi.nix \
  nix/check-support/fence-capabilities.nix nix/check-support/pi-adapter.nix
git commit -m "feat: expose sandboxed Pi support"
```

---

## Task 8: Add the native Pi suite and state containment fixtures

**Files:**
- Create: `tests/native/pi/main_test.go`
- Create: `tests/native/pi/fixture_test.go`
- Create: `tests/native/pi/state_test.go`
- Create: `tests/native/pi/package_test.go`
- Create: `tests/native/pi/resources_test.go`
- Create: `nix/check-support/pi-native-fixture.nix`
- Create: `nix/check-support/fixtures/pi/native/report-extension.ts`
- Create: `nix/check-support/fixtures/pi/native/provider-extension.ts`
- Create: `nix/check-support/fixtures/pi/native/switch-extension.ts`
- Create: `nix/check-support/fixtures/pi/native/skill/SKILL.md`
- Create: `nix/check-support/fixtures/pi/native/prompt.md`
- Create: `nix/check-support/fixtures/pi/native/theme.json`
- Modify: `nix/check-support/native-enforcement.nix`
- Modify: `nix/check-support/native-runner.sh`
- Modify: `modules/checks/native-enforcement.nix`
- Modify: `tests/native/native_test.go`

### Step 1: Wire a separate Pi binary into the host runner

Build `tests/native/pi` with the existing `native` tag. Do not add Pi cases to the large Claude native file.

Before writing the behavioral assertions, modify the native derivation and runner so the new binary is mandatory. Export these packaged inputs from `native-enforcement.nix`:

```text
DEN_NATIVE_PI_TEST_BINARY
DEN_NATIVE_PI
DEN_NATIVE_PI_SANDBOX
DEN_NATIVE_PI_MANIFEST
DEN_NATIVE_PI_PACKAGE_ROOT
DEN_NATIVE_PI_RESOURCE_FIXTURE
```

Create a baseline `pi-native-fixture.nix` with the real package, sandbox, manifest, package root, and an empty configured-resource fixture. Import it through `modules/checks/native-enforcement.nix`.

Run the Pi test binary after the Claude suite on Linux and Darwin. Keep cleanup and deferred-error behavior unchanged.

Add `TestMain` before the first run. It writes `pi-suite.complete` under `DEN_NATIVE_HOST_ROOT` only when `m.Run()` succeeds. The runner must require exact `complete\n` content.

Add the same exact marker for the existing Claude Go suite as `claude-suite.complete`. Keep the separate Darwin `claude-startup.complete` marker.

### Step 2: Write failing native behavior tests

Require only fixture paths and sanitized endpoint names. Reject relative executable paths. Never print credential values.

Use Pi's real `--mode rpc` with newline-delimited input and EOF. The configured provider extension emits fixed responses in process and uses no real provider credential or endpoint. Plain startup tests make no model API call.

Cover:

- direct `mkPi` startup;
- agent and session precedence: explicit, inherited, then Den default;
- isolated credential, trust, and session writes;
- final-component links, parent aliases, overlap, and path swaps;
- invoking and runtime home differences;
- denial of `.pi/agent` and host-global `.agents` in both homes;
- untrusted project resources do not load;
- trusted project resources load only after Pi's existing trust gate;
- interactive, RPC, and extension session switches within the root;
- out-of-root, sibling-prefix, symlink, and root-identity switch rejection;
- rejection before file reads and extension events;
- unchanged containment after the extension changes `PI_CODING_AGENT_SESSION_DIR`;
- all wrapper and direct-binary package commands;
- hostile `DefaultPackageManager` calls after environment mutation;
- unchanged settings and package directories;
- no package-resolution request;
- configured package, extension, skill, prompt, and theme reporting.

Stage every new import-tree input, then run:

```sh
git add tests/native/pi nix/check-support/pi-native-fixture.nix \
  nix/check-support/native-enforcement.nix nix/check-support/native-runner.sh \
  modules/checks/native-enforcement.nix tests/native/native_test.go
system=$(nix eval --impure --raw --expr builtins.currentSystem)
scripts/check-native.sh "$system"
```

Expected: the runner executes the Pi test binary. The log names the failing configured-resource assertion because the baseline fixture is empty. This is the RED; an absent binary or skipped suite is not an acceptable failure.

### Step 3: Add real immutable resource fixtures

The fixture package supplies one configured package plus direct extension, skill, prompt, and theme inputs. The report extension writes only sanitized resource names and order into `DEN_NATIVE_HOST_ROOT`.

Assert per-type subgroup order, first winner, loser, and diagnostic. Assert every permitted `--no-*` form removes only the ambient subgroup. Mandatory direct and package resources must still load.

Update `pi-native-fixture.nix` to replace the empty resource fixture with these inputs. Do not weaken the failing assertion.

### Step 4: Run focused and full native checks

Stage the new fixture inputs before any Nix evaluation:

```sh
git add nix/check-support/pi-native-fixture.nix \
  nix/check-support/fixtures/pi/native

go test -c -tags native -o /tmp/den-pi-native-tests ./tests/native/pi
system=$(nix eval --impure --raw --expr builtins.currentSystem)
scripts/check-native.sh "$system"
rm -f /tmp/den-pi-native-tests
```

Expected: the native package compiles and the host-run script passes both executed agent suites. Both exact suite markers are present.

### Step 5: Commit

```sh
git add tests/native/pi nix/check-support/pi-native-fixture.nix \
  nix/check-support/fixtures/pi/native nix/check-support/native-enforcement.nix \
  nix/check-support/native-runner.sh modules/checks/native-enforcement.nix \
  tests/native/native_test.go
git commit -m "test: add native Pi state enforcement suite"
```

---

## Task 9: Prove RepoWolf, network, process, and filesystem enforcement

**Files:**
- Create: `tests/native/pi/network_test.go`
- Create: `tests/native/pi/repowolf_test.go`
- Create: `tests/native/pi/filesystem_test.go`
- Create: `tests/native/pi/process_test.go`
- Modify: `tests/native/pi/fixture_test.go`
- Modify: `nix/check-support/pi-native-fixture.nix`
- Modify: `nix/check-support/native-runner.sh`
- Modify: `nix/check-support/native-enforcement.nix`

### Step 1: Write failing network and routing tests

Use the existing packaged RepoWolf fixture and local DNS controls. Cover:

- provider traffic allowed only through Fence's provider route;
- GitHub API only through RepoWolf;
- Git and SSH only through RepoWolf helpers;
- direct GitHub API, Git, SSH, arbitrary TCP, and package registry requests denied;
- exact `NODE_EXTRA_CA_CERTS`, `GIT_SSH_COMMAND`, and RepoWolf client inputs;
- provider credentials and ordinary proxy variables remain runtime values;
- GitHub credentials and unsafe Git or SSH transport values are scrubbed;
- inherited RepoWolf and the four Den-controlled Pi variables are replaced exactly once;
- credential directories outside the two selected state bindings remain unreadable;
- unrelated host and store paths remain unreadable;
- errors and telemetry do not contain synthetic secret fixture values;
- extra packages cannot replace the wrapper `pi` command;
- native Pi file tools remain inside outer Fence controls;
- arbitrary extension processes receive outer Fence controls but no false argv-aware guarantee.

Run:

```sh
system=$(nix eval --impure --raw --expr builtins.currentSystem)
scripts/check-native.sh "$system"
```

Expected: RED in the new Pi tests before fixture routing is complete.

### Step 2: Reuse the existing policy and RepoWolf authority

Do not add Pi provider-domain options. Pass Pi through the same policy generator, RepoWolf configuration, CA, Git, and SSH helpers as Claude.

Keep telemetry optional, sanitized, non-fatal, and outside RepoWolf synchronization.

### Step 3: Add host-isolation aliases

Create separate invoking and inherited runtime homes in the fixture. Add canonical aliases and symbolic-link aliases. Prove deny rules win over writable grants for both homes.

Do not inspect real host credentials. Populate only synthetic fixture markers.

### Step 4: Verify Linux and current-host behavior

```sh
go test ./internal/policy ./internal/environment ./internal/launch -count=1
system=$(nix eval --impure --raw --expr builtins.currentSystem)
scripts/check-native.sh "$system"
```

Expected: all pass. Linux still performs the existing namespace and runtime feature preflight before Pi starts.

### Step 5: Commit

```sh
git add tests/native/pi nix/check-support/pi-native-fixture.nix \
  nix/check-support/native-runner.sh nix/check-support/native-enforcement.nix
git commit -m "test: verify Pi Fence and RepoWolf enforcement"
```

---

## Task 10: Prove Darwin command enforcement and four-system CI coverage

**Files:**
- Create: `tests/native/pi/darwin_test.go`
- Create: `nix/check-support/pi-darwin-startup.nix`
- Create: `nix/check-support/pi-darwin-startup.sh`
- Modify: `nix/check-support/pi-native-fixture.nix`
- Modify: `nix/check-support/native-enforcement.nix`
- Modify: `nix/check-support/native-runner.sh`
- Modify: `modules/checks/native-enforcement.nix`
- Modify: `scripts/check-native.sh`

### Step 1: Write failing Darwin native tests

On Darwin, cover:

- allowed built-in `bash` reaches execution after the helper's no-change result;
- blocked command forms stop before execution;
- native `!` user shell follows the same policy;
- malformed, rewritten, denied, and failed helper results fail closed;
- hostile user and project extensions cannot replace either shell entry point;
- helper execution creates no HTTP or SOCKS listener;
- outer Fence still confines direct extension process creation;
- exact extension and policy identity are checked immediately before Pi starts.

Package this fixture as `DEN_NATIVE_PI_STARTUP`. Require an absolute executable path on Darwin.

The fixture writes `pi-darwin-startup.complete` only after all startup assertions pass. Use exact `complete\n` content.

Run on a Darwin host:

```sh
system=$(nix eval --impure --raw --expr builtins.currentSystem)
scripts/check-native.sh "$system"
```

Expected: RED until the runner includes the Pi startup fixture and validates its marker.

### Step 2: Require both agent suites in the native driver

Update `scripts/check-native.sh` so it:

- requires `claude-startup` as before;
- requires the Pi package and Pi non-native checks;
- builds one native runner containing both real agent suites;
- imports the Pi startup fixture in `modules/checks/native-enforcement.nix`;
- exports it from `native-enforcement.nix` as `DEN_NATIVE_PI_STARTUP`;
- scans the Darwin derivation graph for the new Pi startup and native inputs;
- runs the host runner once as the invoking user;
- executes `DEN_NATIVE_PI_STARTUP` before either Go suite on Darwin;
- requires exact `pi-darwin-startup.complete` content after that execution;
- fails if either `claude-suite.complete` or `pi-suite.complete` is absent or malformed;
- preserves Darwin disk telemetry and cleanup.

Do not weaken, skip, or replace an existing Claude fixture.

### Step 3: Verify the existing four-system matrix remains unchanged

Confirm the current workflow retains these systems:

- `x86_64-linux`;
- `aarch64-linux`;
- `x86_64-darwin`;
- `aarch64-darwin`.

Each job already runs `scripts/check-native.sh "$system"`. The expanded script makes Pi mandatory without a workflow edit.

Use direct static verification instead of a new workflow-content test:

```sh
python3 - <<'PY'
from pathlib import Path

text = Path(".github/workflows/checks.yml").read_text()
for system in (
    "x86_64-linux",
    "aarch64-linux",
    "x86_64-darwin",
    "aarch64-darwin",
):
    assert f"system: {system}" in text
assert 'scripts/check-native.sh "$system"' in text
print("existing four-system native workflow preserved")
PY
git diff --exit-code bcc23c0 -- .github/workflows/checks.yml
```

### Step 4: Run local verification

```sh
system=$(nix eval --impure --raw --expr builtins.currentSystem)
scripts/check-native.sh "$system"
nix flake check --accept-flake-config --print-build-logs
```

Expected: both pass on the current system. Cross-system evaluation succeeds for all four systems.

### Step 5: Commit

```sh
git add tests/native/pi/darwin_test.go nix/check-support/pi-darwin-startup.nix \
  nix/check-support/pi-darwin-startup.sh nix/check-support/pi-native-fixture.nix \
  nix/check-support/native-enforcement.nix nix/check-support/native-runner.sh \
  modules/checks/native-enforcement.nix scripts/check-native.sh
git commit -m "ci: require native Pi enforcement on four systems"
```

---

## Task 11: Document Pi operation and run final acceptance

**Files:**
- Modify: `README.md`
- Modify: `docs/superpowers/specs/2026-09-05-pi-support-design.md`

No new automated test is justified for documentation text. Use direct link, command, and build verification instead.

### Step 1: Update user documentation

Document:

- `packages.${system}.pi`;
- `lib.${system}.mkPi`;
- `programs.den.pi` for Home Manager and devenv;
- devenv consumption through package or library outputs;
- agent and session precedence;
- configured resources and per-type ordering;
- trusted project resources;
- package-command restrictions;
- RepoWolf routing;
- Linux and Darwin enforcement differences;
- the limited outer-Fence guarantee for arbitrary extension processes on Darwin;
- upgrade obligations for Pi grammar, fixed hashes, patch, and compatibility fixtures.

State clearly that `packages.default` remains Claude.

### Step 2: Mark implementation status accurately

After all implementation checks pass, set the design status to `Implemented on branch. User integration pending.` Do not claim merge, push, or deployment.

### Step 3: Run documentation checks

```sh
python3 - <<'PY'
from pathlib import Path
for name in [
    'README.md',
    'docs/superpowers/specs/2026-09-05-pi-support-design.md',
    'docs/plans/2026-09-05-pi-support.md',
]:
    text = Path(name).read_text()
    assert 'packages.${system}.pi' in text or name.endswith('pi-support.md')
print('documentation references complete')
PY
git diff --check
```

Expected: both pass.

### Step 4: Run the complete acceptance suite

```sh
go test ./... -count=1
system=$(nix eval --impure --raw --expr builtins.currentSystem)
nix build ".#packages.$system.claude" --no-link --print-build-logs
nix build ".#packages.$system.pi" --no-link --print-build-logs
nix build ".#packages.$system.default" --no-link --print-build-logs
scripts/check-native.sh "$system"
nix flake check --accept-flake-config --print-build-logs
git diff --check
git status --short
```

Expected:

- all Go tests pass;
- Claude, Pi, and default packages build;
- default still resolves to Claude;
- native output confirms both exact suite completion markers;
- all flake checks pass;
- no whitespace errors;
- only expected documentation files remain uncommitted.

### Step 5: Request independent review

Dispatch the canonical `reviewer` with fresh context. Give it:

- approved specification path;
- committed implementation plan path;
- base `bcc23c0`;
- current head SHA;
- full diff;
- verification evidence;
- the requirement to preserve Claude and `packages.default`;
- the package, session, Darwin, and four-system acceptance criteria.

Resolve every blocker with a new RED test before changing production code. Rerun the complete acceptance suite after fixes.

### Step 6: Commit documentation only

Read the `commit` skill, then run:

```sh
git add README.md docs/superpowers/specs/2026-09-05-pi-support-design.md
git commit -m "docs: document sandboxed Pi support"
```

Do not push.

---

## Final branch gate

Before offering branch-completion choices:

1. confirm every planned commit checkpoint is present;
2. confirm the worktree is clean;
3. if code changed after the last successful native run, run `system=$(nix eval --impure --raw --expr builtins.currentSystem)` and then `scripts/check-native.sh "$system"`;
4. rerun `nix flake check --accept-flake-config --print-build-logs`;
5. obtain a final fresh adversarial review;
6. present evidence and remaining risks to the user.

Use `superpowers:finishing-a-development-branch` only after these conditions pass. If the user chooses local integration, offer a squash merge into `main`, not a regular merge. Never push without explicit approval.


## Appendix A: Pi 0.84.4 normalized package lock

This gzip/Base64 payload is part of the reviewed plan. Task 4 decodes it and verifies the fixed SHA-256 hash before use.

<!-- PI_LOCK_GZIP_BASE64_BEGIN -->
```text
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
nHoF3pvh+pfj3//+l/8DeNNvSEvDAQA=
```
<!-- PI_LOCK_GZIP_BASE64_END -->
