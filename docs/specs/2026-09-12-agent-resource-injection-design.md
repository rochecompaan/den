# Agent resource injection design

Date: 2026-09-12
Status: validated with user
Branch: `feat/agent-resource-injection`

## Overview

Den launches Claude and Pi with no skills, extensions, plugins, or tools.
This feature adds declarative, immutable resource injection for both agents,
plus a bundle convention so one Nix package can supply resources for both
agents in a single option entry.

Pi already has resource injection on `main`:
`programs.den.pi.resources.{extensions,packages,skills,promptTemplates,themes}`
flows through `pi-resources.nix` into pinned CLI flags carried by the
manifest's `resourceArgs` channel. This design gives Claude an equivalent
pipeline and adds the shared bundle layer.

## Goals

1. Claude resource injection for four classes: skills, plugins, MCP servers,
   and settings fragments.
2. A bundle convention (`passthru.denResources`) and a
   `programs.den.<agent>.bundles` option for both agents.
3. All resources are store-pinned and validated at build time. No runtime or
   mutable installation path.

## Non-goals

- The `roche-pi` bundle export. That is follow-up work in the `roche-pi`
  repository.
- Subagents and slash commands as first-class classes. Plugins can deliver
  both today.
- Passing `--strict-mcp-config`. Injected MCP servers are additive so
  project-level `.mcp.json` approval flows keep working. The flag is reserved
  so users cannot pass it either.
- Plugin marketplaces and any `install`-style runtime mutation. Those
  commands stay reserved.

## Architecture

Delivery rides the manifest's existing generic `resourceArgs` channel. The
launcher needs no new delivery mechanism.

```
Home Manager config                    mkClaude / mkPi args
  programs.den.<agent>.bundles   --+     bundles, resources
  programs.den.<agent>.resources --+            |
                                   v            v
                         den-resources.nix (shared)
                    expand bundles -> merge into per-class lists
                                   |
                 +-----------------+------------------+
                 v                                    v
      claude-resources.nix                   pi-resources.nix (exists)
      normalize + diagnostics                normalize + diagnostics
      . plugins    -> plugins root    -> --plugin-dir
      . skills     -> den-skills plugin -> --plugin-dir
      . mcpServers -> den-mcp.json    -> --mcp-config
      . settings   -> merged file     -> --settings
                 |                                    |
                 +-----------------+------------------+
                                   v
              mkAgentSandbox adapter (unchanged interface)
        resourceArgs + closureOnlyPackages + reservedFlags
                                   v
              manifest JSON -> den-launcher -> agent argv
```

### Components

| File | Change | Responsibility |
| --- | --- | --- |
| `nix/lib/den-resources.nix` | new | Bundle expansion shared by both agents. Reads `passthru.denResources.<agent>` from each bundle, appends list classes, merges `mcpServers` attrsets (duplicate name is an eval error), and returns the merged `resources`. |
| `nix/lib/claude-resources.nix` | new | Normalizes the four Claude classes into `resourceArgs` plus a `diagnosticsCheck` derivation (Claude analogue of `pi-resource-validation`). |
| `nix/lib/options.nix` | extend | Claude constructor validation grows `resources` and `bundles` root options, same assert style as `pi-options.nix`. |
| `nix/lib/pi-options.nix` | extend | Grows a `bundles` root option. Expansion feeds the existing five resource classes. |
| `nix/lib/module-options.nix` | extend | `programs.den.claude.resources.{skills,plugins,mcpServers,settings}` and `programs.den.{claude,pi}.bundles`. |
| `nix/lib/mk-claude.nix` | extend | Wires normalized resources into the adapter: `resourceArgs`, resource store paths into `closureOnlyPackages`, reserved flags grow by `--plugin-dir`, `--mcp-config`, `--strict-mcp-config`, `--setting-sources`. `--settings` becomes Den-owned on both platforms whenever settings content exists. |
| `nix/lib/mk-pi.nix` | extend | Accepts `bundles` and expands them through the shared helper before the existing `pi-resources.nix` path. |
| `internal/claude/arguments.go`, `internal/arguments/arguments.go` | required | The launcher validates the manifest reserved-flag list by deep equality against the hardcoded Claude policy table, and manifest load fails on mismatch. Both lists grow by the four new flags together with the Nix-side list. Data-only change. |

### Claude class delivery

Verified against the pinned claude-code 2.1.158 binary: `--plugin-dir <path>`,
`--mcp-config <files...>`, `--strict-mcp-config`, `--setting-sources`, and
`--settings` all exist. There is no `--skill` flag; skills load from the
config directory, the project directory, or from a plugin's `skills/`
directory.

- **plugins**: one `--plugin-dir <entry>` per plugin. Verified against the
  pinned binary: the flag is repeatable and each path must itself be a plugin
  directory (`.claude-plugin/plugin.json` at its root).
- **skills**: all skill entries are wrapped into one generated `den-skills`
  plugin (a build-time derivation with a `.claude-plugin/plugin.json`
  manifest and a `skills/` directory of symlinks, one per discovered
  `SKILL.md` parent directory) delivered through the same `--plugin-dir`
  channel. Verified: skills inside a `--plugin-dir` plugin surface to the
  model.
- **mcpServers**: rendered to a store file `den-mcp.json` and passed via
  `--mcp-config`. Server commands must be absolute store paths inside the
  sandbox closure.
- **settings**: fragments are deep-merged at build time into one Den-owned
  store file passed via `--settings`. On Darwin the fence `PreToolUse` hook is
  appended last and is not overridable. On Linux the file is only generated
  when fragments exist, so an unconfigured Linux launch stays flag-free.

### Key invariant

A bundle is packaging sugar. After expansion, bundle contributions flow
through the same per-class validation as directly declared resources. The
sandbox policy, protected paths, and launcher argument validation keep their
shape; only the Claude reserved-flag list and the manifest content grow.

## Option surface

Constructor and Home Manager module accept the same values.

```nix
programs.den.claude = {
  enable = true;
  bundles = [ pkg ];                    # packages with passthru.denResources
  resources = {
    skills  = [ path-or-package ];      # dir containing SKILL.md, or a tree of skill dirs
    plugins = [ path-or-package ];      # dir with .claude-plugin/plugin.json
    mcpServers = {                      # name -> definition
      <name> = {
        command = "/nix/store/.../bin/server";  # absolute store path
        args = [ ];                             # optional
        env  = { };                             # optional
      };
    };
    settings = [ attrset-or-json-file ];        # deep-merged in list order
  };
};
```

`programs.den.pi.bundles` has the identical shape; its contributions feed the
existing five Pi classes.

## Bundle convention

A bundle is a derivation with `passthru.denResources`. Agent keys are `pi`
and `claude`; both are optional. Class names mirror the corresponding
`resources` options exactly.

```nix
pkgs.runCommand "example-den-bundle"
  {
    passthru.denResources = {
      pi = {
        extensions = [ "${src}/extensions/handoff" ];
        packages = [ piPackageDir ];
        skills = [ "${superpowers}/skills" ];
        promptTemplates = [ ];
        themes = [ ];
      };
      claude = {
        skills = [ "${superpowers}/skills" ];
        plugins = [ "${src}/claude-plugins/example" ];
        mcpServers = {
          codegraph = {
            command = "${pkgs.codegraph}/bin/codegraph-mcp";
            args = [ "--stdio" ];
          };
        };
        settings = [ { env.BASH_DEFAULT_TIMEOUT_MS = "300000"; } ];
      };
    };
  } "mkdir $out"
```

Merge semantics:

- Each bundle contributes `denResources.<agent>`; a missing agent key
  contributes nothing.
- List classes append in bundle order, then direct `resources` entries.
- `mcpServers` attrsets merge; a duplicate server name across bundles or
  against direct entries is an eval error.
- A bundle without `passthru.denResources`, or with unknown agent keys or
  unknown class names, fails evaluation.

## Validation

Three layers, matching the Pi precedent.

1. **Eval-time asserts** (`options.nix`, `pi-options.nix`,
   `den-resources.nix`): unknown option names rejected; every resource entry
   must be a Nix path or derivation; `mcpServers` names must match
   `^[A-Za-z][A-Za-z0-9_-]*$`; bundle shape errors fail with the offending
   attribute named.
2. **Build-time diagnostics derivation** (`claude-resources.nix`, wired into
   `closureOnlyPackages` like Pi's):
   - skills: each entry contains at least one `SKILL.md`; duplicate canonical
     paths rejected.
   - plugins: each entry has `.claude-plugin/plugin.json` with a valid `name`;
     duplicate plugin names rejected, including collision with `den-skills`.
   - mcpServers: `command` exists, is executable, and resolves inside the
     store closure.
   - settings: fragments must parse as JSON objects. A fragment is rejected
     if it sets `disableAllHooks`, contains hooks referencing
     `--claude-pre-tool-use` or `DEN_FENCE_POLICY_FILE` (fence
     impersonation), or sets `apiKeyHelper` or `env.ANTHROPIC_*`
     (credential redirection).
3. **Launcher runtime** (existing): reserved-flag rejection covers the four
   new flags; the manifest schema already validates `resourceArgs` entries.

### Settings merge semantics

Fragments deep-merge left to right. Later fragments win for scalars. Arrays
under `hooks.*` concatenate instead of replacing, because replacement is how
a fragment could silently drop the fence hook. Den appends the fence
`PreToolUse` entry last on Darwin. The result is a single store file passed
as `--settings`.

### Error handling

Everything fails at `nix build` or `home-manager switch` time with a message
naming the offending entry, for example
`Claude skill resource has no SKILL.md: /nix/store/...`. The only
launch-time failures remain the existing launcher integrity checks.

## Testing

Ordered so the riskiest assumption is retired first.

1. **Regression check: `--plugin-dir` semantics.**
   The spike ran during design and passed: `--plugin-dir` is repeatable,
   accepts a plugin directory, and plugin skills surface in the API request.
   `nix/check-support/claude-plugin-injection.nix`, styled after
   `claude-settings-merge`, codifies the spike as a permanent regression
   check so a future claude-code pin bump cannot silently break delivery.
2. **Eval-time checks** (extend `module-api` / `package-api` patterns):
   - `mkClaude { bundles = [ fixture ]; }` and the inline-equivalent
     `mkClaude { resources = ...; }` yield identical
     `denManifest.agent.resourceArgs` (proves bundles are pure sugar).
   - Rejection table: unknown class in `denResources`, bundle without the
     passthru, duplicate `mcpServers` name, non-store-path entry.
3. **Build-time diagnostics checks**: invalid fixtures for each rejection
   path (skill without `SKILL.md`, plugin without manifest, MCP command
   outside the closure, settings fragment with `disableAllHooks`, fence
   impersonation, `apiKeyHelper`). Each must fail the diagnostics derivation.
4. **Fixture bundle**: `nix/check-support/fixture-bundle.nix`, one package
   with `passthru.denResources` declaring both agents (tiny skill, plugin,
   fake MCP server binary, settings fragment). Reused by the checks above and
   below, and serves as the canonical machine-checked example of the
   convention.
5. **Runtime startup checks** (extend `claude-startup` Linux/Darwin and
   `claude-settings-merge`): launch the sandboxed package with the fixture
   bundle and assert (a) merged settings still trigger the fence marker on
   Darwin, (b) the user settings fragment took effect, (c) the MCP config
   flag is present and the file well-formed, (d) the skill is visible to the
   agent. The Pi side gets the manifest-equality eval check only; bundles do
   not change its runtime path.
6. **Go**: if `internal/claude/arguments.go` needs the four new reserved
   flags, its existing table-driven test covers the change.

Exit gate: `nix flake check --accept-flake-config --print-build-logs` green.

## Documentation

- README gains an "Inject agent resources" section: the four Claude classes,
  the five Pi classes, one worked example per agent, and the bundle
  convention with producer and consumer examples. The fixture bundle is
  referenced as the canonical example.
- Option descriptions in `module-options.nix` state the security posture
  inline ("Immutable ...; store paths only").

## Naming

- `passthru.denResources` — bundle declaration attribute, agent keys `pi`
  and `claude`.
- `programs.den.<agent>.bundles` — consumer option.
- `nix/lib/den-resources.nix` — shared bundle expansion.
- `nix/lib/claude-resources.nix` — Claude normalizer.
- `den-skills` — synthesized plugin wrapping injected Claude skills.

## Risks

| Risk | Mitigation |
| --- | --- |
| A future claude-code pin bump changes `--plugin-dir` behavior | The `claude-plugin-injection` regression check exercises the real binary on every `nix flake check`. |
| Settings merge breaks the Darwin fence-hook validation flow | The merged file is Den-generated and the fence entry is appended last; the extended `claude-settings-merge` check asserts the fence marker still fires. |
| Reserved-flag enforcement is split between manifest data and hardcoded Go | Confirmed: both Go lists and the Nix list must change in one commit because manifest load deep-equals the policy table. The plan updates them together. |
