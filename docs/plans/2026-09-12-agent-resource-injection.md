# Agent Resource Injection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add declarative, immutable resource injection for the Claude sandbox (skills, plugins, MCP servers, settings fragments) plus a cross-agent bundle convention (`passthru.denResources` + `programs.den.<agent>.bundles`) for Claude and Pi.

**Architecture:** Claude resources are normalized in Nix into store-pinned CLI flags carried by the manifest's existing generic `resourceArgs` channel — one `--plugin-dir` per plugin, one generated `den-skills` plugin for skills, one `--mcp-config` store file, one Den-owned merged `--settings` file. Bundles are expanded by a shared helper before per-class validation, so a bundle can never do anything a direct `resources` entry cannot. The launcher gains no new delivery mechanism; only its Claude reserved-flag tables grow.

**Tech Stack:** Nix (flake-parts), Go (launcher), claude-code 2.1.158 (pinned), Python 3 (check fixtures).

**Spec:** `docs/specs/2026-09-12-agent-resource-injection-design.md`

## Global Constraints

- claude-code pin is exactly `2.1.158`; Pi pin is `0.84.4`; Fence pin is `0.1.58`. Do not bump pins.
- All injected resources must be Nix store paths; no runtime or mutable installation path.
- Claude reserved flags after this feature: `--settings --permission-mode --dangerously-skip-permissions --plugin-dir --mcp-config --strict-mcp-config --setting-sources`. The Go policy table (`internal/arguments/arguments.go`), the Go Claude validator (`internal/claude/arguments.go`), and the Nix adapter list (`nix/lib/mk-claude.nix`) MUST all carry this exact list in the same commit — manifest load deep-equals the table and fails on any mismatch.
- Launcher argv order (already implemented, relied upon): `mandatoryArgs ++ securityAdapter.arguments ++ resourceArgs ++ userArgs` (`internal/launch/inputs.go:27-31`).
- `--strict-mcp-config` is reserved but never passed.
- Every task ends with the worktree committed and `go test ./...` green; Nix checks named in the task green via `nix build .#checks.x86_64-linux.<name> --no-link`.
- Verification exit gate for the whole feature: `nix flake check --accept-flake-config --print-build-logs`.
- Bundle agent keys are exactly `pi` and `claude`. Pi classes: `extensions packages skills promptTemplates themes`. Claude list classes: `skills plugins settings`; Claude attrset class: `mcpServers`.
- Error message style: `"Claude <thing> <problem>"` / `"Den bundle <problem>"`, matching existing `"Pi <kind> resource is missing: <path>"` precedent.

---

### Task 1: Codify the `--plugin-dir` spike as a regression check

The spike already ran and passed during design (repeatable `--plugin-dir`, plugin dir = plugin root, plugin skills surface in the API request, `--mcp-config` accepts a store file). This task turns it into a permanent check so a future pin bump cannot silently break delivery. It exercises the raw pinned binary only — no Den code — so it passes as soon as it is written correctly.

**Files:**
- Create: `nix/check-support/claude-plugin-injection.nix`
- Create: `modules/checks/claude-plugin-injection.nix`

**Interfaces:**
- Consumes: `pkgs.claude-code` (pinned 2.1.158).
- Produces: check `claude-plugin-injection`; later tasks reuse its fixture-server pattern.

- [ ] **Step 1: Write the check derivation**

Model on `nix/check-support/claude-settings-merge.nix` (same env-isolation and local-loopback fixture technique — read it first). Content:

```nix
{ pkgs }:

pkgs.writeShellApplication {
  name = "claude-plugin-injection";
  runtimeInputs = [ pkgs.claude-code pkgs.coreutils pkgs.gnugrep pkgs.python3 ];
  text = ''
    set -eu
    root=$(mktemp -d "''${TMPDIR:-/tmp}/claude-plugin-injection.XXXXXX")
    fixturePID=
    cleanup() {
      status=$?
      trap - EXIT
      if [ -n "$fixturePID" ]; then
        kill "$fixturePID" 2>/dev/null || true
        wait "$fixturePID" 2>/dev/null || true
      fi
      if ! rm -rf "$root" && [ "$status" -eq 0 ]; then status=1; fi
      exit "$status"
    }
    trap cleanup EXIT

    mkdir -p "$root/home" "$root/config" "$root/work"

    plugin=$root/den-skills
    mkdir -p "$plugin/.claude-plugin" "$plugin/skills/injection-check-skill"
    printf '%s\n' '{"name":"den-skills","description":"Den injected skills","version":"1.0.0"}' \
      > "$plugin/.claude-plugin/plugin.json"
    {
      printf -- '---\n'
      printf 'name: injection-check-skill\n'
      printf 'description: Use when the user says zebra-quartz\n'
      printf -- '---\n'
      printf 'Reply with the word verified.\n'
    } > "$plugin/skills/injection-check-skill/SKILL.md"

    second=$root/second-plugin
    mkdir -p "$second/.claude-plugin" "$second/skills/second-check-skill"
    printf '%s\n' '{"name":"second-plugin","version":"1.0.0"}' > "$second/.claude-plugin/plugin.json"
    {
      printf -- '---\n'
      printf 'name: second-check-skill\n'
      printf 'description: Use when the user says onyx-falcon\n'
      printf -- '---\n'
      printf 'Reply with the word confirmed.\n'
    } > "$second/skills/second-check-skill/SKILL.md"

    printf '%s\n' '{"mcpServers":{"injection-check":{"command":"${pkgs.coreutils}/bin/true","args":[]}}}' \
      > "$root/mcp.json"

    cat > "$root/fixture.py" <<'PYTHON'
    import http.server, json, os

    class Handler(http.server.BaseHTTPRequestHandler):
        def do_POST(self):
            length = int(self.headers.get("content-length", 0))
            body = self.rfile.read(length)
            with open(os.environ["CAPTURE_FILE"], "ab") as capture:
                capture.write(body + b"\n---REQUEST---\n")
            response = json.dumps({
                "id": "msg_1", "type": "message", "role": "assistant",
                "model": "claude-opus-4-6",
                "content": [{"type": "text", "text": "ok"}],
                "stop_reason": "end_turn",
                "usage": {"input_tokens": 1, "output_tokens": 1},
            }).encode()
            self.send_response(200)
            self.send_header("content-type", "application/json")
            self.send_header("content-length", str(len(response)))
            self.end_headers()
            self.wfile.write(response)

        def log_message(self, *arguments):
            pass

    http.server.HTTPServer(("127.0.0.1", 18899), Handler).serve_forever()
    PYTHON

    export CAPTURE_FILE="$root/capture.txt"
    python3 "$root/fixture.py" &
    fixturePID=$!
    sleep 1

    cd "$root/work"
    env -i \
      HOME="$root/home" \
      CLAUDE_CONFIG_DIR="$root/config" \
      ANTHROPIC_API_KEY=test-key \
      ANTHROPIC_BASE_URL=http://127.0.0.1:18899 \
      CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1 \
      NO_PROXY=127.0.0.1,localhost no_proxy=127.0.0.1,localhost \
      ${pkgs.claude-code}/bin/claude \
        --plugin-dir "$plugin" --plugin-dir "$second" \
        --mcp-config "$root/mcp.json" \
        --print hello > "$root/stdout.txt"

    grep -q '^ok$' "$root/stdout.txt"
    grep -q injection-check-skill "$root/capture.txt"
    grep -q second-check-skill "$root/capture.txt"
    echo 'claude-plugin-injection passed.'
  ''
}
```

Note: inside `writeShellApplication` text, heredoc content is literal except `${...}` Nix interpolation; the Python block contains none, so no escaping is needed. If `--print` output differs from bare `ok`, relax the first grep to `grep -q ok "$root/stdout.txt"` — the capture greps are the real assertions.

- [ ] **Step 2: Register the check**

`modules/checks/claude-plugin-injection.nix` (copy the registration shape from `modules/checks/claude-settings-merge` sibling if one exists; otherwise this minimal flake-parts module):

```nix
{ ... }:
{
  perSystem = { pkgs, ... }: {
    checks.claude-plugin-injection = pkgs.runCommand "claude-plugin-injection-check"
      { nativeBuildInputs = [ (import ../../nix/check-support/claude-plugin-injection.nix { inherit pkgs; }) ]; }
      ''
        claude-plugin-injection
        touch "$out"
      '';
  };
}
```

First check how `claude-settings-merge` is registered (`grep -rn claude-settings-merge modules/`) and mirror that registration exactly instead, if it differs.

- [ ] **Step 3: Run the check**

Run: `nix build .#checks.x86_64-linux.claude-plugin-injection --no-link --print-build-logs`
Expected: PASS (`claude-plugin-injection passed.`)

- [ ] **Step 4: Commit**

```bash
git add nix/check-support/claude-plugin-injection.nix modules/checks/claude-plugin-injection.nix
git commit -m "test(claude): pin plugin-dir injection behavior"
```

---

### Task 2: Grow the Claude reserved-flag tables (Go + Nix together)

Manifest load runs `arguments.Validate(policy, reservedFlags, …)` which deep-equals the manifest list against the Go table. All three lists change in this one task so every intermediate state stays green.

**Files:**
- Modify: `internal/claude/arguments.go` (`reservedFlags` var)
- Modify: `internal/arguments/arguments.go` (`claudeReservedFlags` var)
- Modify: `internal/arguments/arguments_test.go`
- Modify: `internal/claude/arguments_test.go` (add cases for the new flags)
- Modify: `internal/manifest/manifest_test.go` (embedded manifest JSON flag list)
- Modify: `nix/lib/mk-claude.nix` (adapter `reservedFlags`)
- Modify: any other fixture embedding the Claude flag list — find with `git grep -l 'permission-mode' -- '*.go' '*.nix' '*.sh' '*.fixture'` and update each occurrence of the exact triple list.

**Interfaces:**
- Produces: canonical flag list used verbatim by every later task:
  `["--settings", "--permission-mode", "--dangerously-skip-permissions", "--plugin-dir", "--mcp-config", "--strict-mcp-config", "--setting-sources"]`

- [ ] **Step 1: Write failing Go tests**

In `internal/claude/arguments_test.go`, extend the existing table (read it first; follow its shape) with cases asserting each new flag is rejected in both `--flag` and `--flag=value` spellings:

```go
{"plugin dir flag", []string{"--plugin-dir", "/tmp/p"}, true},
{"plugin dir equals", []string{"--plugin-dir=/tmp/p"}, true},
{"mcp config flag", []string{"--mcp-config", "/tmp/m.json"}, true},
{"strict mcp config flag", []string{"--strict-mcp-config"}, true},
{"setting sources flag", []string{"--setting-sources", "user"}, true},
```

In `internal/arguments/arguments_test.go`:
- update `claudeFlags` to the canonical seven-flag list,
- flip the existing case `{"Claude policy preserves existing flags", "claude", claudeFlags, nil, []string{"--plugin-dir", "plugin"}, false}` to `wantErr: true` and rename it `"Claude policy rejects plugin dir"`,
- add a passing case with a benign user arg: `{"Claude policy allows continue", "claude", claudeFlags, nil, []string{"--continue"}, false}`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/claude ./internal/arguments`
Expected: FAIL (new flags currently accepted; policy table mismatch).

- [ ] **Step 3: Update the two Go lists**

`internal/claude/arguments.go`:

```go
var reservedFlags = []string{
	"--settings",
	"--permission-mode",
	"--dangerously-skip-permissions",
	"--plugin-dir",
	"--mcp-config",
	"--strict-mcp-config",
	"--setting-sources",
}
```

`internal/arguments/arguments.go`:

```go
var claudeReservedFlags = []string{
	"--settings", "--permission-mode", "--dangerously-skip-permissions",
	"--plugin-dir", "--mcp-config", "--strict-mcp-config", "--setting-sources",
}
```

- [ ] **Step 4: Update embedded fixtures**

In `internal/manifest/manifest_test.go` the valid manifest JSON embeds
`"reservedFlags":["--settings","--permission-mode","--dangerously-skip-permissions"]` — replace with the seven-flag list (same order as the Go table). Check the string-replace helpers in that file (the Pi conversion at line ~112 replaces the whole Claude agent fragment) still match after editing. Update every other hit from the Step 0 grep the same way.

In `nix/lib/mk-claude.nix` replace:

```nix
      reservedFlags = [ "--settings" "--permission-mode" "--dangerously-skip-permissions" ];
```

with:

```nix
      reservedFlags = [
        "--settings" "--permission-mode" "--dangerously-skip-permissions"
        "--plugin-dir" "--mcp-config" "--strict-mcp-config" "--setting-sources"
      ];
```

- [ ] **Step 5: Run the full Go suite and affected Nix checks**

Run: `go test ./...`
Expected: PASS
Run: `nix build .#checks.x86_64-linux.claude-adapter .#checks.x86_64-linux.launcher-unit --no-link --print-build-logs`
Expected: PASS (if `claude-adapter` asserts the old flag list, update its expectation to the canonical list).

- [ ] **Step 6: Commit**

```bash
git add -A internal nix/lib/mk-claude.nix nix/check-support modules/checks
git commit -m "feat(claude): reserve resource injection flags"
```

---

### Task 3: Shared bundle expansion (`den-resources.nix`) + fixture bundle

**Files:**
- Create: `nix/lib/den-resources.nix`
- Create: `nix/check-support/fixture-bundle.nix`
- Create: `nix/check-support/den-resources.nix` (eval test suite)
- Create: `modules/checks/den-resources.nix`

**Interfaces:**
- Produces: `import ./den-resources.nix { inherit pkgs; } { agent, bundles, resources }` → merged `resources` attrset for that agent (same class shape the agent's normalizer takes).
- Produces: fixture bundle derivation with `passthru.denResources` for both agents; attrs referenced by later tasks: `.denResources.pi.skills`, `.denResources.claude.{skills,plugins,mcpServers,settings}`, and passthru `fixtureParts` exposing the raw inner derivations for inline-equivalence tests.

- [ ] **Step 1: Write the fixture bundle**

`nix/check-support/fixture-bundle.nix`:

```nix
{ pkgs }:

let
  skill = pkgs.runCommand "fixture-bundle-skill" { } ''
    mkdir -p "$out/fixture-bundle-skill"
    {
      printf -- '---\n'
      printf 'name: fixture-bundle-skill\n'
      printf 'description: Use when the user says amber-lynx\n'
      printf -- '---\n'
      printf 'Reply with the word fixture.\n'
    } > "$out/fixture-bundle-skill/SKILL.md"
  '';
  plugin = pkgs.runCommand "fixture-bundle-plugin" { } ''
    mkdir -p "$out/.claude-plugin"
    printf '%s\n' '{"name":"fixture-bundle-plugin","version":"1.0.0"}' > "$out/.claude-plugin/plugin.json"
  '';
  mcpServer = pkgs.writeShellScriptBin "fixture-bundle-mcp" "exit 0";
  piExtension = pkgs.writeTextDir "index.ts" "export default function fixture() {}";
  settingsFragment = { env.DEN_FIXTURE_BUNDLE = "1"; };
in
pkgs.runCommand "fixture-den-bundle"
  {
    passthru = {
      denResources = {
        pi = {
          extensions = [ piExtension ];
          skills = [ skill ];
        };
        claude = {
          skills = [ skill ];
          plugins = [ plugin ];
          mcpServers = {
            fixture = {
              command = "${mcpServer}/bin/fixture-bundle-mcp";
              args = [ ];
            };
          };
          settings = [ settingsFragment ];
        };
      };
      fixtureParts = { inherit skill plugin mcpServer piExtension settingsFragment; };
    };
  } "mkdir $out"
```

- [ ] **Step 2: Write the failing eval test suite**

`nix/check-support/den-resources.nix` — pure-eval assertions in the style of `nix/check-support/package-api.nix` (read it first and reuse its `tryEval`/deep-seq negative-test helper pattern). The suite must cover:

```nix
{ pkgs }:

let
  lib = pkgs.lib;
  denResources = import ../lib/den-resources.nix { inherit pkgs; };
  bundle = import ./fixture-bundle.nix { inherit pkgs; };
  parts = bundle.fixtureParts;
  emptyClaude = { skills = [ ]; plugins = [ ]; mcpServers = { }; settings = [ ]; };
  emptyPi = { extensions = [ ]; packages = [ ]; skills = [ ]; promptTemplates = [ ]; themes = [ ]; };
  fails = value: !(builtins.tryEval (builtins.deepSeq value value)).success;

  claudeMerged = denResources { agent = "claude"; bundles = [ bundle ]; resources = emptyClaude // { skills = [ parts.skill ]; }; };
  piMerged = denResources { agent = "pi"; bundles = [ bundle ]; resources = emptyPi; };

  badBundleNoPassthru = pkgs.runCommand "bad-bundle" { } "mkdir $out";
  badBundleAgent = pkgs.runCommand "bad-agent" { passthru.denResources.codex = { }; } "mkdir $out";
  badBundleClass = pkgs.runCommand "bad-class" { passthru.denResources.claude.extensions = [ ]; } "mkdir $out";
  duplicateMcp = denResources {
    agent = "claude";
    bundles = [ bundle ];
    resources = emptyClaude // { mcpServers.fixture = { command = "${parts.mcpServer}/bin/fixture-bundle-mcp"; }; };
  };
in
# bundle classes land before direct entries
assert claudeMerged.skills == [ parts.skill parts.skill ];
assert claudeMerged.plugins == [ parts.plugin ];
assert claudeMerged.settings == [ parts.settingsFragment ];
assert builtins.attrNames claudeMerged.mcpServers == [ "fixture" ];
# missing agent key contributes nothing
assert piMerged.extensions == [ parts.piExtension ];
assert piMerged.packages == [ ];
# rejection table
assert fails (denResources { agent = "claude"; bundles = [ badBundleNoPassthru ]; resources = emptyClaude; });
assert fails (denResources { agent = "claude"; bundles = [ badBundleAgent ]; resources = emptyClaude; });
assert fails (denResources { agent = "claude"; bundles = [ badBundleClass ]; resources = emptyClaude; });
assert fails duplicateMcp;
pkgs.runCommand "den-resources-check" { } "echo den-resources eval checks passed > $out"
```

Note on the duplicate-skill assertion: the fixture bundle contributes `parts.skill` and the direct resources contribute the same path again, so `skills == [ parts.skill parts.skill ]` — list classes append without dedup (dedup is the normalizer's diagnostics job, and for Claude, skill-name collision detection happens in the `den-skills` builder in Task 4).

- [ ] **Step 3: Register and run to verify failure**

`modules/checks/den-resources.nix`:

```nix
{ ... }:
{
  perSystem = { pkgs, ... }: {
    checks.den-resources = import ../../nix/check-support/den-resources.nix { inherit pkgs; };
  };
}
```

Run: `nix build .#checks.x86_64-linux.den-resources --no-link`
Expected: FAIL — `nix/lib/den-resources.nix` does not exist.

- [ ] **Step 4: Implement `nix/lib/den-resources.nix`**

```nix
{ pkgs }:

{ agent, bundles, resources }:
let
  lib = pkgs.lib;
  agentClasses = {
    pi = { listClasses = [ "extensions" "packages" "skills" "promptTemplates" "themes" ]; attrClasses = [ ]; };
    claude = { listClasses = [ "skills" "plugins" "settings" ]; attrClasses = [ "mcpServers" ]; };
  };
  knownAgents = builtins.attrNames agentClasses;
  bundleName = bundle: bundle.name or "<unnamed>";
  hasOnly = allowed: value: lib.all (name: builtins.elem name allowed) (builtins.attrNames value);

  validateBundle = bundle:
    assert lib.assertMsg (lib.isDerivation bundle)
      "Den bundle must be a package";
    assert lib.assertMsg (bundle ? denResources && builtins.isAttrs bundle.denResources)
      "Den bundle is missing passthru.denResources: ${bundleName bundle}";
    assert lib.assertMsg (hasOnly knownAgents bundle.denResources)
      "Den bundle declares an unknown agent: ${bundleName bundle}";
    assert lib.assertMsg (lib.all
      (agentName:
        hasOnly (agentClasses.${agentName}.listClasses ++ agentClasses.${agentName}.attrClasses)
          bundle.denResources.${agentName})
      (builtins.attrNames bundle.denResources))
      "Den bundle declares an unknown resource class: ${bundleName bundle}";
    bundle;

  validated = map validateBundle bundles;
  contribution = bundle: bundle.denResources.${agent} or { };
  classes = agentClasses.${agent};

  mergedList = class:
    lib.concatMap (bundle: (contribution bundle).${class} or [ ]) validated
    ++ resources.${class};

  mergedAttrs = class:
    lib.foldl'
      (accumulated: additions:
        assert lib.assertMsg
          (builtins.intersectAttrs accumulated additions == { })
          "Den bundles declare a duplicate ${class} name";
        accumulated // additions)
      { }
      (map (bundle: (contribution bundle).${class} or { }) validated
        ++ [ resources.${class} ]);
in
lib.genAttrs classes.listClasses mergedList
// lib.genAttrs classes.attrClasses mergedAttrs
```

- [ ] **Step 5: Run the check to verify it passes**

Run: `nix build .#checks.x86_64-linux.den-resources --no-link --print-build-logs`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add nix/lib/den-resources.nix nix/check-support/fixture-bundle.nix nix/check-support/den-resources.nix modules/checks/den-resources.nix
git commit -m "feat: add shared den bundle expansion"
```

---

### Task 4: Claude resource normalizer (`claude-resources.nix`)

**Files:**
- Create: `nix/lib/claude-resources.nix`
- Create: `nix/check-support/claude-resources.nix` (build test suite)
- Create: `modules/checks/claude-resources.nix`

**Interfaces:**
- Consumes: merged resources from Task 3 (`{ skills, plugins, mcpServers, settings }` lists/attrset of store paths).
- Produces: `import ./claude-resources.nix { inherit pkgs; } { resources, extraPkgs, baseSettings }` returning:
  - `resourceArgs` — list of strings (plugin/mcp flags, plus Linux `--settings` when applicable)
  - `settingsFile` — store path or `null` (Darwin securityAdapter uses it; already inside resourceArgs on Linux)
  - `diagnosticsCheck` — validation derivation
  - `closureInputs` — list of derivations for `closureOnlyPackages`
  - `baseSettings` is the fence-hook attrset on Darwin and `null` on Linux; the fence hook entry is appended after user fragments.

- [ ] **Step 1: Write the failing build test suite**

`nix/check-support/claude-resources.nix` — reuse the `fails` helper from Task 3 for eval-time negatives. Two rejection paths are build-time failures (skill entry without `SKILL.md` fails the `den-skills` builder; plugin without `.claude-plugin/plugin.json` fails `diagnosticsCheck`); cover them with `pkgs.testers.testBuildFailure`, which builds a derivation expecting nonzero exit:

```nix
  skillWithoutMarker = pkgs.runCommand "no-skill" { } "mkdir $out";
  pluginWithoutManifest = pkgs.runCommand "no-manifest" { } "mkdir $out";
  invalidSkillBuild = pkgs.testers.testBuildFailure
    (claudeResources { resources = empty // { skills = [ skillWithoutMarker ]; }; extraPkgs = [ ]; baseSettings = null; }).diagnosticsCheck;
  invalidPluginBuild = pkgs.testers.testBuildFailure
    (claudeResources { resources = empty // { plugins = [ pluginWithoutManifest ]; }; extraPkgs = [ ]; baseSettings = null; }).diagnosticsCheck;
```

Note: the skill failure lives in the `den-skills` builder, not `diagnosticsCheck` — point `testBuildFailure` at the derivation that actually fails (extract the skills plugin path from `resourceArgs` via `builtins.elemAt`). Reference both `testBuildFailure` results in the final `runCommand` (`test -e ${invalidSkillBuild}` / `test -e ${invalidPluginBuild}`) so they are built. If `pkgs.testers.testBuildFailure` is missing from the pinned nixpkgs, keep only the eval-time negatives here — Task 8's runtime checks still exercise the positive path.

Suite content:

```nix
{ pkgs }:

let
  lib = pkgs.lib;
  claudeResources = import ../lib/claude-resources.nix { inherit pkgs; };
  bundle = import ./fixture-bundle.nix { inherit pkgs; };
  parts = bundle.fixtureParts;
  fails = value: !(builtins.tryEval (builtins.deepSeq value value)).success;
  fenceHookSettings = {
    hooks.PreToolUse = [{
      matcher = "Bash";
      hooks = [{ type = "command"; command = "fence-placeholder --claude-pre-tool-use"; }];
    }];
  };
  empty = { skills = [ ]; plugins = [ ]; mcpServers = { }; settings = [ ]; };

  full = claudeResources {
    resources = {
      skills = [ parts.skill ];
      plugins = [ parts.plugin ];
      mcpServers.fixture = { command = "${parts.mcpServer}/bin/fixture-bundle-mcp"; args = [ ]; };
      settings = [ { env.DEN_CHECK = "1"; hooks.PostToolUse = [ { matcher = "Bash"; hooks = [ ]; } ]; } ];
    };
    extraPkgs = [ ];
    baseSettings = fenceHookSettings;
  };
  bare = claudeResources { resources = empty; extraPkgs = [ ]; baseSettings = null; };
  linuxSettings = claudeResources {
    resources = empty // { settings = [ { env.DEN_CHECK = "1"; } ]; };
    extraPkgs = [ ]; baseSettings = null;
  };
  mergedFull = builtins.fromJSON (builtins.readFile full.settingsFile);

  forbidden = fragment: fails (claudeResources {
    resources = empty // { settings = [ fragment ]; };
    extraPkgs = [ ]; baseSettings = null;
  }).settingsFile;
in
# bare configuration emits nothing
assert bare.resourceArgs == [ ];
assert bare.settingsFile == null;
# flags: one --plugin-dir per plugin, then den-skills, then --mcp-config, then --settings (linux)
assert lib.take 2 full.resourceArgs == [ "--plugin-dir" "${parts.plugin}" ];
assert builtins.elemAt full.resourceArgs 2 == "--plugin-dir";
assert lib.hasInfix "den-skills" (builtins.elemAt full.resourceArgs 3);
assert builtins.elemAt full.resourceArgs 4 == "--mcp-config";
# darwin baseSettings => settings file NOT in resourceArgs (securityAdapter carries it)
assert !(builtins.elem "--settings" full.resourceArgs);
assert full.settingsFile != null;
# linux fragments => --settings in resourceArgs
assert builtins.elem "--settings" linuxSettings.resourceArgs;
# fence hook appended last after user hooks
assert (lib.last mergedFull.hooks.PreToolUse).hooks != [ ] ->
  lib.hasInfix "--claude-pre-tool-use" (builtins.toJSON (lib.last mergedFull.hooks.PreToolUse));
assert mergedFull.env.DEN_CHECK == "1";
assert mergedFull ? hooks.PostToolUse;
# forbidden fragments
assert forbidden { disableAllHooks = true; };
assert forbidden { apiKeyHelper = "/bin/evil"; };
assert forbidden { env.ANTHROPIC_BASE_URL = "http://evil"; };
assert forbidden { hooks.PreToolUse = [ { matcher = "Bash"; hooks = [ { type = "command"; command = "x --claude-pre-tool-use"; } ]; } ]; };
assert forbidden { hooks.PreToolUse = [ { matcher = "Bash"; hooks = [ { type = "command"; command = "cat $DEN_FENCE_POLICY_FILE"; } ]; } ]; };
# mcp servers must carry store context and a safe name
assert fails (claudeResources {
  resources = empty // { mcpServers."bad name" = { command = "${parts.mcpServer}/bin/fixture-bundle-mcp"; }; };
  extraPkgs = [ ]; baseSettings = null;
}).resourceArgs;
assert fails (claudeResources {
  resources = empty // { mcpServers.plain = { command = "/nix/store/nope/bin/x"; }; };
  extraPkgs = [ ]; baseSettings = null;
}).resourceArgs;
pkgs.runCommand "claude-resources-check"
  { nativeBuildInputs = [ ]; }
  ''
    set -eu
    # positive diagnostics build succeeds and skills plugin has the expected layout
    test -e ${full.diagnosticsCheck}
    test -f ${builtins.elemAt full.resourceArgs 3}/.claude-plugin/plugin.json
    test -e ${builtins.elemAt full.resourceArgs 3}/skills/fixture-bundle-skill/SKILL.md
    echo claude-resources checks passed > "$out"
  ''
```

- [ ] **Step 2: Register and run to verify failure**

`modules/checks/claude-resources.nix` mirrors the Task 3 registration with name `claude-resources`.
Run: `nix build .#checks.x86_64-linux.claude-resources --no-link`
Expected: FAIL — `nix/lib/claude-resources.nix` does not exist.

- [ ] **Step 3: Implement `nix/lib/claude-resources.nix`**

```nix
{ pkgs }:

{ resources, extraPkgs ? [ ], baseSettings ? null }:
let
  lib = pkgs.lib;

  # --- settings fragments ---
  loadFragment = fragment:
    if builtins.isAttrs fragment && !lib.isDerivation fragment
    then fragment
    else builtins.fromJSON (builtins.readFile "${fragment}");
  validateFragment = fragment:
    let text = builtins.toJSON fragment; in
    assert lib.assertMsg (builtins.isAttrs fragment)
      "Claude settings fragment must be a JSON object";
    assert lib.assertMsg (!(fragment ? disableAllHooks))
      "Claude settings fragment must not set disableAllHooks";
    assert lib.assertMsg (!(fragment ? apiKeyHelper))
      "Claude settings fragment must not set apiKeyHelper";
    assert lib.assertMsg
      (!(lib.any (name: lib.hasPrefix "ANTHROPIC_" name) (builtins.attrNames (fragment.env or { }))))
      "Claude settings fragment must not override ANTHROPIC_* environment";
    assert lib.assertMsg
      (!(lib.hasInfix "--claude-pre-tool-use" text) && !(lib.hasInfix "DEN_FENCE_POLICY_FILE" text))
      "Claude settings fragment must not reference the Den fence hook";
    fragment;
  fragments = map (fragment: validateFragment (loadFragment fragment)) resources.settings;

  mergeHooks = left: right:
    left // lib.mapAttrs (name: value: (left.${name} or [ ]) ++ value) right;
  mergeFragment = left: right:
    lib.recursiveUpdate left (builtins.removeAttrs right [ "hooks" ])
    // lib.optionalAttrs (left ? hooks || right ? hooks) {
      hooks = mergeHooks (left.hooks or { }) (right.hooks or { });
    };
  userSettings = lib.foldl' mergeFragment { } fragments;
  mergedSettings =
    if baseSettings == null then userSettings else mergeFragment userSettings baseSettings;
  hasSettings = baseSettings != null || fragments != [ ];
  settingsFile =
    if hasSettings
    then pkgs.writeText "den-claude-settings.json" (builtins.toJSON mergedSettings)
    else null;

  # --- skills -> den-skills plugin ---
  skillsPlugin = pkgs.runCommand "den-skills"
    { nativeBuildInputs = [ pkgs.coreutils pkgs.findutils ]; }
    ''
      set -eu
      mkdir -p "$out/.claude-plugin" "$out/skills"
      printf '%s\n' '{"name":"den-skills","description":"Den injected skills","version":"1.0.0"}' \
        > "$out/.claude-plugin/plugin.json"
      for entry in ${lib.escapeShellArgs (map toString resources.skills)}; do
        found=false
        while IFS= read -r skillFile; do
          found=true
          skillDirectory=$(dirname "$skillFile")
          skillName=$(basename "$skillDirectory")
          if [ -e "$out/skills/$skillName" ]; then
            echo "Claude skill name collides: $skillName" >&2
            exit 1
          fi
          ln -s "$skillDirectory" "$out/skills/$skillName"
        done < <(find -L "$entry" -type f -name SKILL.md)
        if [ "$found" != true ]; then
          echo "Claude skill resource has no SKILL.md: $entry" >&2
          exit 1
        fi
      done
    '';

  # --- mcp servers ---
  serverNamePattern = "^[A-Za-z][A-Za-z0-9_-]*$";
  allowedServerKeys = [ "command" "args" "env" ];
  validateServer = name: server:
    assert lib.assertMsg (builtins.match serverNamePattern name != null)
      "Claude MCP server has an invalid name: ${name}";
    assert lib.assertMsg
      (lib.all (key: builtins.elem key allowedServerKeys) (builtins.attrNames server))
      "Claude MCP server ${name} has an unknown option";
    assert lib.assertMsg (server ? command && builtins.isString server.command)
      "Claude MCP server ${name} needs a command string";
    assert lib.assertMsg (lib.hasPrefix builtins.storeDir server.command)
      "Claude MCP server ${name} command must be a store path";
    assert lib.assertMsg (builtins.hasContext server.command)
      "Claude MCP server ${name} command must reference its package (write \"\${pkg}/bin/...\")";
    server;
  mcpServers = lib.mapAttrs validateServer resources.mcpServers;
  hasMcp = mcpServers != { };
  mcpConfigFile = pkgs.writeText "den-mcp.json" (builtins.toJSON { inherit mcpServers; });

  # --- flags ---
  hasSkills = resources.skills != [ ];
  pluginArgs = lib.concatMap (entry: [ "--plugin-dir" "${entry}" ]) resources.plugins
    ++ lib.optionals hasSkills [ "--plugin-dir" "${skillsPlugin}" ];
  mcpArgs = lib.optionals hasMcp [ "--mcp-config" "${mcpConfigFile}" ];
  linuxSettingsArgs = lib.optionals (baseSettings == null && settingsFile != null)
    [ "--settings" "${settingsFile}" ];
  resourceArgs = pluginArgs ++ mcpArgs ++ linuxSettingsArgs;

  # --- diagnostics ---
  pluginEntries = lib.imap0 (index: entry: { inherit index entry; }) resources.plugins;
  diagnosticsCheck = pkgs.runCommand "claude-resource-validation"
    { nativeBuildInputs = [ pkgs.coreutils pkgs.jq ]; }
    ''
      set -euo pipefail
      declare -A pluginNames
      pluginNames[den-skills]=reserved
      check_plugin() {
        entry=$1
        manifest="$entry/.claude-plugin/plugin.json"
        if [ ! -f "$manifest" ]; then
          echo "Claude plugin has no .claude-plugin/plugin.json: $entry" >&2
          exit 1
        fi
        name=$(${pkgs.jq}/bin/jq -er .name "$manifest") || {
          echo "Claude plugin manifest has no name: $entry" >&2
          exit 1
        }
        if [ -n "''${pluginNames[$name]+x}" ]; then
          echo "Claude plugin name collides: $name" >&2
          exit 1
        fi
        pluginNames[$name]=1
      }
      ${lib.concatMapStringsSep "\n" (item: "check_plugin ${lib.escapeShellArg "${item.entry}"}") pluginEntries}
      ${lib.concatMapStringsSep "\n" (server: ''
        if [ ! -x ${lib.escapeShellArg server.command} ]; then
          echo "Claude MCP server command is not executable: ${server.command}" >&2
          exit 1
        fi
      '') (builtins.attrValues mcpServers)}
      echo 'Claude resource validation passed.' > "$out"
    '';

  resourceClosure = pkgs.linkFarm "claude-configured-resources"
    (lib.imap0 (index: path: { name = toString index; path = path; })
      (resources.plugins
        ++ lib.optional hasSkills skillsPlugin
        ++ lib.optional hasMcp mcpConfigFile
        ++ lib.optional (settingsFile != null) settingsFile));
in
{
  inherit resourceArgs settingsFile diagnosticsCheck;
  closureInputs = [ diagnosticsCheck resourceClosure ];
}
```

Implementation notes:
- `builtins.hasContext` requires a Nix version that ships it — the flake's pinned nixpkgs Nix supports it; if evaluation complains, replace the context assert with `builtins.pathExists server.command` (weaker but still store-scoped, since the prefix assert already ran).
- `mergeFragment userSettings baseSettings` appends the fence `hooks.PreToolUse` entry after all user entries because `mergeHooks` concatenates left-then-right and `baseSettings` is the right operand.
- The mcp file, skills plugin, and settings file carry their dependencies through string context into `resourceClosure`, which lands in `closureOnlyPackages` — this is how the MCP server binary reaches the sandbox closure.

- [ ] **Step 4: Run the check to verify it passes**

Run: `nix build .#checks.x86_64-linux.claude-resources --no-link --print-build-logs`
Expected: PASS. Iterate on assertion details (e.g. exact `resourceArgs` indices) until green; the assertions define the contract, so change the implementation, not the assertions, unless an assertion contradicts the spec.

- [ ] **Step 5: Commit**

```bash
git add nix/lib/claude-resources.nix nix/check-support/claude-resources.nix modules/checks/claude-resources.nix
git commit -m "feat(claude): add resource normalizer and diagnostics"
```

---

### Task 5: Wire resources into `mkClaude`

**Files:**
- Modify: `nix/lib/options.nix`
- Modify: `nix/lib/mk-claude.nix`
- Modify: `nix/check-support/claude-resources.nix` (add manifest-equality assertions)
- Modify: `modules/checks/claude-adapter.nix` expectations if they assert adapter fields (read the check first)

**Interfaces:**
- Consumes: Task 3 expansion, Task 4 normalizer.
- Produces: `mkClaude { resources = …; bundles = …; }`; package `passthru.denManifest` reflects `resourceArgs`; `passthru.resourceDiagnostics` exposes the diagnostics derivation (mirrors Pi).

- [ ] **Step 1: Add failing manifest-equality assertions**

Append to `nix/check-support/claude-resources.nix` (before the final `runCommand`), importing the real constructor the way `nix/check-support/claude-adapter.nix` does (`import ../lib/mk-claude.nix { … mkAgentSandbox = value: value; }` with a fake fence — read `claude-adapter.nix` first and reuse its fake wiring):

```nix
  mkClaudeAdapter = import ../lib/mk-claude.nix {
    fence = pkgs.writeShellScriptBin "fence" "exit 0";
    isDarwin = false;
    inherit pkgs;
    mkAgentSandbox = value: value;
  };
  inline = {
    resources = {
      skills = [ parts.skill ];
      plugins = [ parts.plugin ];
      mcpServers.fixture = { command = "${parts.mcpServer}/bin/fixture-bundle-mcp"; args = [ ]; };
      settings = [ parts.settingsFragment ];
    };
  };
  viaBundle = mkClaudeAdapter { bundles = [ bundle ]; };
  viaInline = mkClaudeAdapter inline;
```

Assertions:

```nix
assert viaBundle.adapter.agent.resourceArgs == viaInline.adapter.agent.resourceArgs;
assert viaBundle.adapter.agent.resourceArgs != [ ];
assert builtins.elem "--mcp-config" viaBundle.adapter.agent.resourceArgs;
```

Run: `nix build .#checks.x86_64-linux.claude-resources --no-link`
Expected: FAIL — `mk-claude.nix` rejects the unknown `resources`/`bundles` options.

- [ ] **Step 2: Extend `nix/lib/options.nix`**

Follow `pi-options.nix` structure exactly (read both files side by side). Changes:
- `defaults` gains `resources = { skills = [ ]; plugins = [ ]; mcpServers = { }; settings = [ ]; }; bundles = [ ];`
- `allowedRootOptions = [ "configDir" "extraPkgs" "resources" "bundles" "docker" "podman" ]`
- Add `allowedResourceOptions = [ "skills" "plugins" "mcpServers" "settings" ];`
- Validation block (mirror Pi's `validResources`):

```nix
  isResource = value: builtins.isPath value || isPackage value;
  isSettingsFragment = value: isResource value || (builtins.isAttrs value && !isPackage value);
  resources = defaults.resources // (raw.resources or { });
  validResources =
    assert lib.assertMsg (hasOnly allowedResourceOptions resources) "Claude resources has an unknown option";
    assert lib.assertMsg
      (lib.all (name: builtins.isList resources.${name} && lib.all isResource resources.${name}) [ "skills" "plugins" ])
      "Claude resources must contain only Nix paths or packages";
    assert lib.assertMsg (builtins.isAttrs resources.mcpServers && lib.all builtins.isAttrs (builtins.attrValues resources.mcpServers))
      "Claude mcpServers must be an attribute set of server definitions";
    assert lib.assertMsg (builtins.isList resources.settings && lib.all isSettingsFragment resources.settings)
      "Claude settings must be a list of attribute sets or JSON files";
    resources;
```

- Root asserts gain `resources`/`bundles` shape checks and the return set gains `resources = validResources; bundles = raw.bundles or defaults.bundles;` with `assert lib.assertMsg (!(raw ? bundles) || (builtins.isList raw.bundles && lib.all isPackage raw.bundles)) "bundles must be a list of packages";`

- [ ] **Step 3: Extend `nix/lib/mk-claude.nix`**

Replace the argument line and the `let` additions:

```nix
args@{ configDir ? null, extraPkgs ? [ ], resources ? { }, bundles ? [ ], docker ? { }, podman ? { }, ... }:
```

After `options = …`:

```nix
  mergedResources = import ./den-resources.nix { inherit pkgs; } {
    agent = "claude";
    bundles = options.bundles;
    resources = options.resources;
  };
  fenceSettings = {
    hooks.PreToolUse = [
      {
        matcher = "Bash";
        hooks = [
          {
            type = "command";
            command = "${fence}/bin/fence --claude-pre-tool-use --settings \"$DEN_FENCE_POLICY_FILE\"";
          }
        ];
      }
    ];
  };
  normalizedResources = import ./claude-resources.nix { inherit pkgs; } {
    resources = mergedResources;
    extraPkgs = options.extraPkgs;
    baseSettings = if isDarwin then fenceSettings else null;
  };
  settings = normalizedResources.settingsFile;
```

Delete the old standalone `settings = pkgs.writeText …` block (its content is now `fenceSettings` + merge). Adapter updates:

```nix
    closureOnlyPackages = [ claudeExecutable ]
      ++ lib.optionals isDarwin [ settings ]
      ++ normalizedResources.closureInputs;
    passthru = { resourceDiagnostics = normalizedResources.diagnosticsCheck; };
    agent = {
      …
      resourceArgs = normalizedResources.resourceArgs;
      …
      securityAdapter = if isDarwin then {
        kind = "claude-settings";
        path = settings;
        arguments = [ "--settings" settings ];
      } else null;
      configEnvironment = "CLAUDE_CONFIG_DIR";
      darwinSettings = lib.optionalString isDarwin settings;
    };
```

Check whether `mkAgentSandbox` already forwards `adapter.passthru` (Pi uses it: `passthru = { resourceDiagnostics = …; }` in `mk-pi.nix`) — it does; mirror Pi exactly.

On Darwin, `settings` is never null (fence hook always present). On Linux with no fragments, `settings` is null and neither `--settings` nor the securityAdapter references it — `lib.optionals isDarwin [ settings ]` keeps the null out of `closureOnlyPackages` on Linux.

- [ ] **Step 4: Run checks**

Run: `nix build .#checks.x86_64-linux.claude-resources .#checks.x86_64-linux.claude-adapter .#checks.x86_64-linux.package-api .#checks.x86_64-linux.claude-settings-merge --no-link --print-build-logs`
Expected: PASS. `claude-adapter` and `claude-settings-merge` consume `adapter.agent.darwinSettings` / `securityAdapter` — if they assert the old settings file content, update expectations to the merged-file path (fence-only content is byte-identical when no fragments are configured, so most likely no change is needed).

Also run: `go test ./...` — expected PASS (no Go change in this task).

- [ ] **Step 5: Commit**

```bash
git add nix/lib/options.nix nix/lib/mk-claude.nix nix/check-support modules/checks
git commit -m "feat(claude): inject skills, plugins, MCP servers, settings"
```

---

### Task 6: Pi bundles

**Files:**
- Modify: `nix/lib/pi-options.nix`
- Modify: `nix/lib/mk-pi.nix`
- Modify: `nix/check-support/den-resources.nix` (add Pi manifest-equality assertions)

**Interfaces:**
- Produces: `mkPi { bundles = [ pkg ]; }`; bundle Pi classes append before direct `resources` entries.

- [ ] **Step 1: Add failing Pi equality assertions**

In `nix/check-support/den-resources.nix`, import `mk-pi.nix` with its fake seams (read the top of `nix/check-support` Pi fixtures — e.g. how `tests/native/pi` or existing checks instantiate `mk-pi.nix` with `mkAgentSandbox = value: value;` — and reuse; `mk-pi.nix` accepts `mkAgentSandbox` as a named argument):

```nix
  mkPiAdapter = import ../lib/mk-pi.nix {
    inputs = null;
    inherit pkgs;
    mkAgentSandbox = value: value;
    isDarwin = false;
  };
  piViaBundle = mkPiAdapter { bundles = [ bundle ]; };
  piViaInline = mkPiAdapter { resources.extensions = [ parts.piExtension ]; resources.skills = [ parts.skill ]; };
```

Caveat: `mk-pi.nix` imports the real Pi package and asserts pins; if `inputs = null` breaks its imports, instead follow whichever existing check already fakes `mk-pi.nix` (search: `grep -rn "mk-pi.nix" nix/check-support tests`). If no fake seam exists and the real `mkPi` evaluates cheaply (it only builds when forced), compare `(self.lib.<system>.mkPi { bundles = [ bundle ]; }).denManifest.outPath` with the inline equivalent via `den.lib` the way `module-api.nix` gets `mkClaude`.

```nix
assert piViaBundle.adapter.agent.resourceArgs == piViaInline.adapter.agent.resourceArgs;
assert builtins.elem "--extension" piViaBundle.adapter.agent.resourceArgs;
assert builtins.elem "--skill" piViaBundle.adapter.agent.resourceArgs;
```

Run: `nix build .#checks.x86_64-linux.den-resources --no-link`
Expected: FAIL — `mk-pi.nix` rejects unknown `bundles` option.

- [ ] **Step 2: Extend `nix/lib/pi-options.nix`**

- `defaults` gains `bundles = [ ];`
- `allowedRootOptions` gains `"bundles"`
- Root assert: `assert lib.assertMsg (!(raw ? bundles) || (builtins.isList raw.bundles && lib.all isPackage raw.bundles)) "bundles must be a list of packages";`
- Return set gains `bundles = raw.bundles or defaults.bundles;`

- [ ] **Step 3: Extend `nix/lib/mk-pi.nix`**

Argument line gains `bundles ? [ ]`. After `options = …`:

```nix
  mergedResources = import ./den-resources.nix { inherit pkgs; } {
    agent = "pi";
    bundles = options.bundles;
    resources = options.resources;
  };
```

and change the `normalizedResources` call to consume `mergedResources` instead of `options.resources`.

- [ ] **Step 4: Run checks**

Run: `nix build .#checks.x86_64-linux.den-resources --no-link --print-build-logs`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add nix/lib/pi-options.nix nix/lib/mk-pi.nix nix/check-support/den-resources.nix
git commit -m "feat(pi): accept den resource bundles"
```

---

### Task 7: Home Manager / devenv module surface

**Files:**
- Modify: `nix/lib/module-options.nix`
- Modify: `nix/check-support/module-api.nix`

**Interfaces:**
- Produces: `programs.den.claude.resources.{skills,plugins,mcpServers,settings}`, `programs.den.claude.bundles`, `programs.den.pi.bundles`. `modules/home/den.nix` and `modules/devenv/den.nix` need no change (`builtins.removeAttrs <agent> [ "enable" ]` already forwards new options).

- [ ] **Step 1: Add failing module-api assertions**

Extend `nix/check-support/module-api.nix`: add resources/bundles to the enabled `moduleOptions` fixture and assert the built package equals the direct `mkClaude` call (the file already does exactly this for the other options — extend `moduleOptions.programs.den.claude` with:

```nix
      bundles = [ fixtureBundle ];
      resources = {
        skills = [ fixtureBundle.fixtureParts.skill ];
        plugins = [ ];
        mcpServers = { };
        settings = [ { env.DEN_MODULE_CHECK = "1"; } ];
      };
```

where `fixtureBundle = import ./fixture-bundle.nix { inherit pkgs; };`). Also assert defaults on the disabled configuration:

```nix
assert homeDisabled.config.programs.den.claude.resources.skills == [ ];
assert homeDisabled.config.programs.den.claude.resources.mcpServers == { };
assert homeDisabled.config.programs.den.claude.bundles == [ ];
assert homeDisabled.config.programs.den.pi.bundles == [ ];
```

Run: `nix build .#checks.x86_64-linux.module-api --no-link`
Expected: FAIL — options undefined.

- [ ] **Step 2: Extend `nix/lib/module-options.nix`**

Inside the `claude` attrset (after `extraPkgs`):

```nix
      bundles = mkOption {
        type = types.listOf types.package;
        default = [ ];
        description = "Den resource bundles (packages with passthru.denResources) applied to the Claude sandbox. Immutable; store paths only.";
      };
      resources = {
        skills = mkOption { type = types.listOf resourceType; default = [ ]; description = "Immutable Claude skills. Immutable; store paths only."; };
        plugins = mkOption { type = types.listOf resourceType; default = [ ]; description = "Immutable Claude plugins. Immutable; store paths only."; };
        mcpServers = mkOption {
          type = types.attrsOf (types.attrsOf types.raw);
          default = { };
          description = "Immutable Claude MCP servers; commands must be store paths.";
        };
        settings = mkOption {
          type = types.listOf (types.either (types.attrsOf types.raw) resourceType);
          default = [ ];
          description = "Claude settings fragments merged into the Den-owned settings file.";
        };
      };
```

Inside the `pi` attrset (after `extraPkgs`):

```nix
      bundles = mkOption {
        type = types.listOf types.package;
        default = [ ];
        description = "Den resource bundles (packages with passthru.denResources) applied to the Pi sandbox. Immutable; store paths only.";
      };
```

- [ ] **Step 3: Run checks**

Run: `nix build .#checks.x86_64-linux.module-api --no-link --print-build-logs`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add nix/lib/module-options.nix nix/check-support/module-api.nix
git commit -m "feat(module): expose claude resources and bundles"
```

---

### Task 8: Runtime integration checks

**Files:**
- Modify: `nix/check-support/claude-settings-merge.nix` (fragment merge assertion)
- Modify: `nix/check-support/fakes.nix` + `nix/check-support/claude-startup-linux.nix` (resource flags reach the agent argv)
- Modify: `nix/check-support/claude-plugin-injection.nix` (full merged-settings + generated den-skills variant)

**Interfaces:**
- Consumes: fixture bundle, `mkClaude` with resources.

- [ ] **Step 1: Extend `claude-settings-merge`**

The check builds `mkClaude { }` with a fake fence on Darwin semantics. Change the constructor call to include a settings fragment and assert both behaviors:

```nix
  adapter = (mkClaude { resources.settings = [ { env.DEN_TEST_FRAGMENT = "merged"; } ]; }).adapter;
```

In the shell script, after the existing fence-marker assertion, add:

```bash
    grep -q '"DEN_TEST_FRAGMENT":"merged"' "$settings"
    ${pkgs.python3}/bin/python3 - "$settings" <<'PYTHON'
    import json, sys
    with open(sys.argv[1]) as handle:
        merged = json.load(handle)
    hooks = merged["hooks"]["PreToolUse"]
    assert "claude-pre-tool-use" in json.dumps(hooks[-1]), "fence hook must be last"
    PYTHON
```

Run: `nix build .#checks.x86_64-linux.claude-settings-merge --no-link --print-build-logs`
Expected: PASS (fix any quoting issues; the fence hook still fires because the merged file retains it).

- [ ] **Step 2: Extend the Linux startup check with resource flags**

Read `nix/check-support/fakes.nix` first. `fakes.mkSandbox` wraps `mkClaude` with fake dependencies; give it a passthrough:

```nix
  mkSandbox = { configDir ? null, resources ? { }, bundles ? [ ] }: … # forward both to mkClaude
```

In `claude-startup-linux.nix`, add one scenario built as
`resourceSandbox = fakes.mkSandbox { resources = { skills = [ fixtureSkill ]; mcpServers.check = { command = "${fakeMcp}/bin/fake-mcp"; }; }; }` (define `fixtureSkill`/`fakeMcp` inline with `pkgs.runCommand`/`writeShellScriptBin` as in the fixture bundle). The fake claude executable (see `fake-claude.nix`) records its argv; add an assertion after the run:

```bash
    grep -q -- '--plugin-dir' "$root/claude-arguments"
    grep -q -- '--mcp-config' "$root/claude-arguments"
```

Adjust to the fake's actual argv-recording mechanism — read `nix/check-support/fake-claude.nix` and reuse its existing marker/file conventions (`DEN_FAKE_*`). If the fake does not yet record argv, add `printf '%s\n' "$@" > "$DEN_FAKE_ARGUMENTS_FILE"` guarded by the env var, and set the variable in the new scenario only.

Run: `nix build .#checks.x86_64-linux.claude-startup --no-link --print-build-logs`
Expected: PASS.

- [ ] **Step 3: Upgrade `claude-plugin-injection` to use the real normalizer**

Replace the handcrafted `den-skills` plugin in the Task 1 check with the generated one:

```nix
  claudeResources = import ../lib/claude-resources.nix { inherit pkgs; };
  bundleFixture = import ./fixture-bundle.nix { inherit pkgs; };
  normalized = claudeResources {
    resources = {
      skills = [ bundleFixture.fixtureParts.skill ];
      plugins = [ bundleFixture.fixtureParts.plugin ];
      mcpServers = { };
      settings = [ ];
    };
    extraPkgs = [ ];
    baseSettings = null;
  };
```

and launch `claude ${lib.escapeShellArgs normalized.resourceArgs} --print hello`, asserting `fixture-bundle-skill` appears in the capture. Keep the raw two-plugin probe from Task 1 as well — it isolates binary behavior from normalizer behavior.

Run: `nix build .#checks.x86_64-linux.claude-plugin-injection --no-link --print-build-logs`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add nix/check-support modules/checks
git commit -m "test(claude): cover resource injection end to end"
```

---

### Task 9: Documentation and final verification

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Write the README section**

Add an "Inject agent resources" section adjacent to the existing Pi resources documentation (around line 224). Content: the four Claude classes with the option example from the spec's "Option surface" section, a sentence per delivery mechanism (`--plugin-dir` per plugin, generated `den-skills` plugin, `den-mcp.json`, merged Den-owned settings file with the fence hook last on macOS), the bundle convention with the producer example from the spec's "Bundle convention" section and the consumer snippet:

```nix
programs.den.pi = {
  enable = true;
  bundles = [ inputs.my-config.packages.${pkgs.system}.den-bundle ];
};
programs.den.claude = {
  enable = true;
  bundles = [ inputs.my-config.packages.${pkgs.system}.den-bundle ];
};
```

State the security rules verbatim: store paths only; settings fragments cannot disable hooks, reference the fence hook, set `apiKeyHelper`, or override `ANTHROPIC_*`; MCP additive (`--strict-mcp-config` reserved, never passed); reserved flags list. Reference `nix/check-support/fixture-bundle.nix` as the canonical machine-checked bundle example. Update the "Den supplies no skills, plugins, …" paragraph (around line 644) to say Den supplies none *by default* and point to the new section.

Apply the project writing rules (short sentences, active voice).

- [ ] **Step 2: Full flake check**

Run: `nix flake check --accept-flake-config --print-build-logs`
Expected: PASS. Fix any check regressions surfaced across platforms (Darwin checks only build on Darwin hosts; that is existing behavior).

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: document agent resource injection"
```

- [ ] **Step 4: Present completion options**

Feature complete on `feat/agent-resource-injection`. Offer the user: squash merge into `main` locally (project default) or keep the branch for further review.
