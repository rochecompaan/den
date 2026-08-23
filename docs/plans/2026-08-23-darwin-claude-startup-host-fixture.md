# Darwin Claude Startup Host Fixture Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restore both Darwin CI jobs by packaging the Darwin Claude startup fixture in Nix and executing it through the existing native host runner.

**Architecture:** Linux keeps its current in-sandbox startup check. Darwin builds an executable startup fixture, then the native runner executes it beneath a unique `DEN_NATIVE_HOST_ROOT` before resolver startup and the native Go suite. A structured derivation-graph guard rejects `/bin/ls` only in impure-host-dependency fields.

**Tech Stack:** Nix and flake-parts, Bash, Python 3 standard library, jq, Go tests, GitHub Actions, Actionlint, and the `gh` CLI.

## Global Constraints

- Work only in `/home/roche/projects/den/.worktrees/den-claude-sandbox-design` on `feat/den-claude-sandbox`.
- Keep PR #1 open and unmerged.
- Keep all four native targets: `x86_64-linux`, `aarch64-linux`, `x86_64-darwin`, and `aarch64-darwin`.
- Keep Nix sandboxing and the existing platform sandbox values.
- Do not add `__noChroot`, broaden host-path access, skip checks, or weaken ACL validation.
- Keep `nix/check-support/claude-startup-linux.nix` unchanged.
- Keep host execution of `claude-settings-merge`.
- Run the Darwin startup fixture after settings merge and before resolver startup.
- Keep every default, custom, explicit, inherited, resource-integrity, policy, overlap, symbolic-link, pre-Fence, token-redaction, and ACL-diagnostic assertion.
- Put every Darwin startup path beneath the unique runner-owned `DEN_NATIVE_HOST_ROOT`.
- Use strict TDD for production behavior. Record a meaningful red result before production edits.
- Do not add automated tests that assert raw GitHub Actions YAML text. Use Actionlint and structured direct validation.
- Request fresh independent review before any commit.
- After creating a new file that Nix reads, run `git add -N <path>` so the uncommitted file enters the flake source. Do not create a commit.
- Create focused Conventional Commits only after review findings are resolved.
- Push the existing branch and monitor both push and pull-request workflow runs.
- Stop without merging.

## File map

### New focused files

- `scripts/check-derivation-impure-host-deps.py` — parse recursive derivation JSON from stdin and reject `/bin/ls` only in impure-host-dependency fields.
- `tests/check-derivation-impure-host-deps.py` — behavioral tests for direct, nested, string, list, and runtime-literal graph cases.
- `nix/check-support/claude-startup-runtime-manifest.sh` — private manifest adapter and normalized-manifest validator.
- `tests/claude-startup-runtime-manifest.sh` — adapter tests for inherited, explicit, and unauthorized mutations.
- `nix/check-support/claude-startup-darwin.sh` — host-executed Darwin startup scenarios beneath `DEN_NATIVE_HOST_ROOT`.
- `tests/native-runner.sh` — fake host-runner harness for order, completion, and fail-fast behavior.

### Existing files to modify

- `tests/check-native-driver.sh:15-93` — add successful fake-runner flows for all systems and reject daemon-policy inspection.
- `scripts/check-native.sh:9-58` — remove daemon-policy checks, require `claude-startup`, run the graph guard, and execute the runner.
- `nix/check-support/claude-startup-darwin.nix:1-262` — replace the build-time scenario derivation with a packaged executable.
- `nix/check-support/native-enforcement.nix:178-224` — accept, validate, and export the Darwin startup package.
- `modules/checks/native-enforcement.nix:1-11` — pass the platform startup output into native enforcement.
- `nix/check-support/native-runner.sh:9-115` — execute the Darwin startup fixture before resolver startup.
- `tests/native/native_test.go:1-35` — enforce the Darwin completion-artifact contract before the suite runs.
- `modules/checks/launcher-unit.nix:10-24` — run the new parser, adapter, and runner tests with their packaged tools.
- `.github/workflows/checks.yml:16-48` — remove only the impure-host-dependency matrix fields and installer line.

### Files that must remain unchanged

- `nix/check-support/claude-startup-linux.nix` — Linux behavior remains the current namespace-based derivation.
- `nix/check-support/claude-startup.nix` — keep the current Linux/Darwin selector.
- `modules/checks/claude-startup.nix` — keep `checks.claude-startup` as the selected platform output.
- `nix/check-support/claude-settings-merge.nix` — keep the existing host fixture and output contract.

---

### Task 1: Add the native-driver behavioral regression and observe RED

**Files:**
- Modify: `tests/check-native-driver.sh:15-93`
- Modify after RED: `scripts/check-native.sh:9-58`

**Interfaces:**
- Consumes: `scripts/check-native.sh SYSTEM` and the fake `nix` executable on `PATH`.
- Produces: a driver that always builds Claude, requires and builds `claude-startup`, builds the native runner, then executes it.
- Produces: no `nix config show allowed-impure-host-deps` call on any system.

- [ ] **Step 1: Replace policy-only fake cases with successful flows for all systems**

Create a temporary fake store and runner inside `tests/check-native-driver.sh`:

```bash
fake_store=$root/store
fake_runner=$fake_store/fake-native-runner
mkdir -p "$root/bin" "$fake_runner/bin"

cat > "$fake_runner/bin/native-enforcement" <<'FAKE_RUNNER'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' 'execute native-runner' >> "$DEN_FAKE_EVENT_LOG"
FAKE_RUNNER
chmod +x "$fake_runner/bin/native-enforcement"

export DEN_FAKE_STORE_DIR=$fake_store
export DEN_FAKE_RUNNER=$fake_runner
export DEN_FAKE_EVENT_LOG=$root/events.log
export DEN_FAKE_DERIVATION_JSON='{"derivations":{},"version":3}'
```

Make the fake `nix` command provide current-system, store-directory, check-list, build, and derivation responses. It must fail daemon-policy inspection:

```bash
if [[ $1 == config && $2 == show && ${3-} == allowed-impure-host-deps ]]; then
  printf 'unexpected allowed-impure-host-deps inspection\n' >&2
  exit 97
fi
if [[ $1 == eval && $* == *builtins.currentSystem* ]]; then
  printf '%s' "$DEN_FAKE_CURRENT_SYSTEM"
  exit
fi
if [[ $1 == eval && $* == *builtins.storeDir* ]]; then
  printf '%s' "$DEN_FAKE_STORE_DIR"
  exit
fi
if [[ $1 == eval && $* == *--apply* ]]; then
  if [[ $DEN_FAKE_APPLY_STATUS -ne 0 ]]; then
    exit "$DEN_FAKE_APPLY_STATUS"
  fi
  printf '%s\n' "$DEN_FAKE_NORMAL_CHECKS"
  exit
fi
if [[ $1 == derivation && $2 == show ]]; then
  printf '%s\n' "$DEN_FAKE_DERIVATION_JSON"
  exit
fi
if [[ $1 == build && $* == *packages.*.claude* ]]; then
  printf '%s\n' 'build claude' >> "$DEN_FAKE_EVENT_LOG"
  exit
fi
if [[ $1 == build && $* == *checks.*.claude-startup* ]]; then
  printf '%s\n' 'build claude-startup' >> "$DEN_FAKE_EVENT_LOG"
  exit
fi
if [[ $1 == build && $* == *native-enforcement* ]]; then
  printf '%s\n' 'build native-runner' >> "$DEN_FAKE_EVENT_LOG"
  printf '%s\n' "$DEN_FAKE_RUNNER"
  exit
fi
```

Change `run_driver` to accept `LABEL SYSTEM APPLY_STATUS NORMAL_CHECKS`. For each run, clear the event and Nix logs.

- [ ] **Step 2: Require successful ordered flows on all four systems**

Add this system loop:

```bash
for system in x86_64-linux aarch64-linux x86_64-darwin aarch64-darwin; do
  label=${system//_/-}
  run_driver "$label" "$system" 0 $'claude-startup\nlauncher-unit'
  if [[ $status -ne 0 ]]; then
    cat "$root/$label.stderr" >&2
    exit 1
  fi
  expected=$'build claude\nbuild claude-startup\nbuild native-runner\nexecute native-runner'
  actual=$(grep -E '^(build claude|build claude-startup|build native-runner|execute native-runner)$' \
    "$DEN_FAKE_EVENT_LOG")
  [[ $actual == "$expected" ]]
  ! grep -Fq 'config show allowed-impure-host-deps' "$DEN_FAKE_NIX_LOG"
done
```

Keep the normal-check enumeration failure case. Add a case where the successful enumeration omits `claude-startup`; require a nonzero status and no native-runner build.

- [ ] **Step 3: Run the regression and record the meaningful RED**

Run:

```bash
bash tests/check-native-driver.sh scripts/check-native.sh
```

Expected: FAIL on the first Darwin success flow because the current driver calls `nix config show allowed-impure-host-deps`. The fake returns status `97`. Save the command and failure line for the final evidence report.

Do not edit production files before this failure is observed.

- [ ] **Step 4: Remove the daemon-policy branch from the production driver**

Delete `scripts/check-native.sh:16-24` completely. Do not replace it with another daemon-policy query.

Resolve the Nix store directory instead of hard-coding it:

```bash
store_dir=$(nix eval --impure --raw --expr builtins.storeDir)
```

Validate the runner against `$store_dir/*`:

```bash
if [[ $runner != "$store_dir"/* || $runner == *$'\n'* ]]; then
  printf 'native runner build returned an unexpected output: %q\n' "$runner" >&2
  exit 1
fi
```

- [ ] **Step 5: Require `claude-startup` in the normal check set**

Track the required check during the existing loop:

```bash
claude_startup_seen=0
while IFS= read -r check; do
  [[ -n $check ]] || continue
  if [[ $check == claude-startup ]]; then
    claude_startup_seen=1
  fi
  printf 'building non-native check %s for %s\n' "$check" "$system"
  nix build ".#checks.$system.$check" --no-link --print-build-logs
done <<< "$normal_checks"

if [[ $claude_startup_seen -ne 1 ]]; then
  printf 'required non-native check claude-startup is missing for %s\n' "$system" >&2
  exit 1
fi
```

- [ ] **Step 6: Run the driver regression and observe GREEN**

Run:

```bash
bash tests/check-native-driver.sh scripts/check-native.sh
```

Expected: PASS for all four success flows. The enumeration and missing-startup negative cases also pass.

Do not commit. The independent-review gate remains open.

---

### Task 2: Add the structured recursive derivation-graph guard

**Files:**
- Create: `scripts/check-derivation-impure-host-deps.py`
- Create: `tests/check-derivation-impure-host-deps.py`
- Modify: `scripts/check-native.sh:40-46`
- Modify: `tests/check-native-driver.sh`
- Modify: `modules/checks/launcher-unit.nix:10-24`

**Interfaces:**
- Consumes: Nix derivation JSON version 3 on stdin.
- Produces: exit `0` when `/bin/ls` is absent from `__impureHostDeps` and `__propagatedImpureHostDeps`.
- Produces: exit `1` plus JSON paths when `/bin/ls` is a complete dependency token in either recognized field.
- Does not inspect unrelated runtime strings.

- [ ] **Step 1: Write parser behavior tests before the parser exists**

Create `tests/check-derivation-impure-host-deps.py`. Execute the helper as a subprocess and cover these documents:

```python
SAFE_RUNTIME_LITERAL = {
    "derivations": {
        "fixture.drv": {"env": {"builderScript": "exec /bin/ls -lde /tmp/state"}}
    },
    "version": 3,
}

DIRECT_STRING = {
    "derivations": {
        "fixture.drv": {"env": {"__impureHostDeps": "/bin/sh /bin/ls /dev/null"}}
    },
    "version": 3,
}

NESTED_LIST = {
    "derivations": {
        "parent.drv": {"inputDrvs": {"child.drv": {"outputs": ["out"]}}},
        "child.drv": {"structuredAttrs": {"__impureHostDeps": ["/bin/sh", "/bin/ls"]}},
    },
    "version": 3,
}
```

Also cover a safe baseline list containing `/bin/sh`, a propagated dependency, malformed JSON, and a non-string field value. Require the error to name the derivation and field path.

- [ ] **Step 2: Run the parser tests and observe RED**

Run:

```bash
python3 tests/check-derivation-impure-host-deps.py \
  scripts/check-derivation-impure-host-deps.py
```

Expected: FAIL because the helper does not exist.

- [ ] **Step 3: Implement the minimal recursive parser**

Create `scripts/check-derivation-impure-host-deps.py` with this core:

```python
#!/usr/bin/env python3
import json
import shlex
import sys

FORBIDDEN = "/bin/ls"
IMPURE_FIELDS = {"__impureHostDeps", "__propagatedImpureHostDeps"}


def dependency_tokens(value):
    if isinstance(value, str):
        return shlex.split(value)
    if isinstance(value, list) and all(isinstance(item, str) for item in value):
        return value
    raise ValueError("impure host dependency field must be a string or string list")


def forbidden_paths(value, path=()):
    matches = []
    if isinstance(value, dict):
        for key, child in value.items():
            child_path = path + (key,)
            if key in IMPURE_FIELDS and FORBIDDEN in dependency_tokens(child):
                matches.append(child_path)
            matches.extend(forbidden_paths(child, child_path))
    elif isinstance(value, list):
        for index, child in enumerate(value):
            matches.extend(forbidden_paths(child, path + (str(index),)))
    return matches


def main():
    document = json.load(sys.stdin)
    matches = forbidden_paths(document)
    for path in matches:
        print(f"forbidden impure host dependency {FORBIDDEN}: {'.'.join(path)}", file=sys.stderr)
    return 1 if matches else 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (json.JSONDecodeError, ValueError) as error:
        print(f"invalid derivation graph: {error}", file=sys.stderr)
        raise SystemExit(2)
```

Make both new files visible to flake source filtering without committing:

```bash
git add -N \
  scripts/check-derivation-impure-host-deps.py \
  tests/check-derivation-impure-host-deps.py
```

- [ ] **Step 4: Run the parser tests and observe GREEN**

Run the Step 2 command.

Expected: PASS. The runtime `/bin/ls` literal remains accepted. Direct and nested impure fields fail.

- [ ] **Step 5: Wire the guard into Darwin native checks**

After non-native builds and before the native-runner build, add this Darwin-only path to `scripts/check-native.sh`:

```bash
if [[ $system == *-darwin ]]; then
  printf 'checking Darwin derivation graph for forbidden /bin/ls impure dependencies\n'
  nix derivation show --recursive \
    ".#checks.$system.claude-startup" \
    ".#checks.$system.native-enforcement" \
    | python3 "$repo_root/scripts/check-derivation-impure-host-deps.py"
fi
```

Define `repo_root` near the top:

```bash
repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
```

Update the fake `nix` command to return `DEN_FAKE_DERIVATION_JSON` for `derivation show`. Add driver cases for a safe runtime literal and a nested forbidden field.

- [ ] **Step 6: Add the parser tests to `checks.launcher-unit`**

Add `pkgs.python3` to `nativeBuildInputs`. Run:

```nix
${pkgs.python3}/bin/python3 tests/check-derivation-impure-host-deps.py \
  "$PWD/scripts/check-derivation-impure-host-deps.py"
```

- [ ] **Step 7: Run focused tests**

Run:

```bash
python3 tests/check-derivation-impure-host-deps.py \
  scripts/check-derivation-impure-host-deps.py
bash tests/check-native-driver.sh scripts/check-native.sh
```

Expected: PASS.

Do not commit.

---

### Task 3: Build and test the private runtime-manifest adapter

**Files:**
- Create: `nix/check-support/claude-startup-runtime-manifest.sh`
- Create: `tests/claude-startup-runtime-manifest.sh`
- Modify: `modules/checks/launcher-unit.nix:10-24`

**Interfaces:**
- Consumes: `den_adapt_claude_startup_manifest BASE OUTPUT MODE CONFIG_DIR`.
- Consumes: `MODE` equal to `inherited` or `explicit`.
- Consumes: absolute `DEN_CLAUDE_STARTUP_ACL_PROBE`.
- Produces: a runtime manifest that changes only `explicitConfigDir` and `aclProbe`.
- Produces: `den_validate_claude_startup_manifest BASE RUNTIME` for explicit mutation checks.

- [ ] **Step 1: Write adapter behavior tests**

Create `tests/claude-startup-runtime-manifest.sh`. Use a base manifest with stable unrelated fields:

```json
{
  "version": 1,
  "platform": "darwin",
  "explicitConfigDir": null,
  "aclProbe": ["/bin/ls", "-lde"],
  "agent": {"name": "claude"},
  "filesystem": {"sentinel": "unchanged"}
}
```

The test must prove:

1. inherited mode leaves `explicitConfigDir` null and replaces only `aclProbe`.
2. explicit mode writes the supplied absolute configuration directory.
3. both fields remain present.
4. a runtime mutation to `.filesystem.sentinel` makes validation fail.
5. an unknown mode and a relative explicit path fail.

Source the helper from the path supplied as `$1`.

- [ ] **Step 2: Run the adapter tests and observe RED**

Run:

```bash
bash tests/claude-startup-runtime-manifest.sh \
  nix/check-support/claude-startup-runtime-manifest.sh
```

Expected: FAIL because the helper does not exist.

- [ ] **Step 3: Implement normalized validation**

Create `nix/check-support/claude-startup-runtime-manifest.sh` with Bash functions only. Do not execute fixture work at source time.

```bash
den_validate_claude_startup_manifest() {
  if [[ $# -ne 2 ]]; then
    printf 'usage: den_validate_claude_startup_manifest BASE RUNTIME\n' >&2
    return 2
  fi
  local base=$1 runtime=$2
  jq -e -n --slurpfile base "$base" --slurpfile runtime "$runtime" '
    ($base | length == 1) and
    ($runtime | length == 1) and
    ($base[0] | has("explicitConfigDir") and has("aclProbe")) and
    ($runtime[0] | has("explicitConfigDir") and has("aclProbe")) and
    (($base[0] | del(.explicitConfigDir, .aclProbe)) ==
     ($runtime[0] | del(.explicitConfigDir, .aclProbe)))
  ' >/dev/null
}
```

Implement the adapter with a private umask, `jq`, exact mode validation, and a final call to the validator:

```bash
den_adapt_claude_startup_manifest() {
  if [[ $# -ne 4 ]]; then
    printf 'usage: den_adapt_claude_startup_manifest BASE OUTPUT MODE CONFIG_DIR\n' >&2
    return 2
  fi
  local base=$1 output=$2 mode=$3 config_dir=$4
  : "${DEN_CLAUDE_STARTUP_ACL_PROBE:?Darwin ACL probe is required}"
  [[ $DEN_CLAUDE_STARTUP_ACL_PROBE == /* ]]

  case "$mode" in
    inherited)
      [[ -z $config_dir ]]
      jq -e '.explicitConfigDir == null' "$base" >/dev/null
      jq --arg probe "$DEN_CLAUDE_STARTUP_ACL_PROBE" \
        '.aclProbe = [$probe, "-lde"]' "$base" > "$output"
      ;;
    explicit)
      [[ $config_dir == /* ]]
      jq --arg probe "$DEN_CLAUDE_STARTUP_ACL_PROBE" --arg config "$config_dir" \
        '.aclProbe = [$probe, "-lde"] | .explicitConfigDir = $config' \
        "$base" > "$output"
      ;;
    *)
      printf 'unknown manifest adaptation mode: %s\n' "$mode" >&2
      return 2
      ;;
  esac

  chmod 0600 "$output"
  den_validate_claude_startup_manifest "$base" "$output"
}
```

Make both files visible to flake source filtering without committing:

```bash
git add -N \
  nix/check-support/claude-startup-runtime-manifest.sh \
  tests/claude-startup-runtime-manifest.sh
```

- [ ] **Step 4: Run the adapter tests and observe GREEN**

Run the Step 2 command.

Expected: PASS.

- [ ] **Step 5: Add the adapter test to `checks.launcher-unit`**

Add `pkgs.jq` to `nativeBuildInputs`. Run the shell test from the check:

```nix
${pkgs.bash}/bin/bash tests/claude-startup-runtime-manifest.sh \
  "$PWD/nix/check-support/claude-startup-runtime-manifest.sh"
```

- [ ] **Step 6: Build the focused check**

Run:

```bash
system=$(nix eval --impure --raw --expr builtins.currentSystem)
nix build ".#checks.$system.launcher-unit" --no-link --print-build-logs
```

Expected: PASS.

Do not commit.

---

### Task 4: Convert Darwin startup into a packaged host fixture

**Files:**
- Rewrite: `nix/check-support/claude-startup-darwin.nix:1-262`
- Create: `nix/check-support/claude-startup-darwin.sh`
- Verify unchanged: `nix/check-support/claude-startup-linux.nix`
- Verify unchanged: `nix/check-support/claude-startup.nix`

**Interfaces:**
- Consumes: `DEN_NATIVE_HOST_ROOT` from the native runner.
- Consumes: packaged base manifests, launcher, and ACL probe through private `DEN_CLAUDE_STARTUP_*` variables.
- Produces: executable `bin/claude-startup` with passthru `denHostFixturePlatform = "darwin"`.
- Produces after success: `$DEN_NATIVE_HOST_ROOT/claude-startup.complete` containing `complete\n`.

- [ ] **Step 1: Run the new graph guard against the current Darwin fixture and observe RED**

Run:

```bash
set -o pipefail
nix derivation show --recursive .#checks.aarch64-darwin.claude-startup \
  | python3 scripts/check-derivation-impure-host-deps.py
```

Expected: FAIL and report the `claude-startup.drv.env.__impureHostDeps` path containing `/bin/ls`.

This is the focused red result for the fixture conversion.

- [ ] **Step 2: Package two immutable base manifests and the ACL diagnostic probe**

Keep the current ACL diagnostic probe logic. Extend its Python scrub inputs so diagnostics cannot expose the runner-owned root:

```python
native_root = os.environ.get("DEN_NATIVE_HOST_ROOT", "")
for path, replacement in sorted(
    (
        (native_root, "<native-host-root>"),
        (original, "<original-path>"),
        (target, "<probe-path>"),
    ),
    key=lambda item: len(item[0]),
    reverse=True,
):
    if path:
        value = value.replace(path, replacement)
```

Replace the many fixed-path fake wrappers with two bases:

```nix
inheritedSandbox = fakes.mkSandbox { };
explicitSandbox = fakes.mkSandbox {
  configDir = "/private/tmp/den-claude-startup-runtime-placeholder";
};
```

The placeholder is manifest data only. The package build must not create or remove that path.

- [ ] **Step 3: Return a `writeShellApplication` instead of `runCommand`**

Use this package shape:

```nix
pkgs.writeShellApplication {
  name = "claude-startup";
  runtimeInputs = [ pkgs.coreutils pkgs.gitMinimal pkgs.gnugrep pkgs.jq ];
  derivationArgs = {
    passthru.denHostFixturePlatform = "darwin";
  };
  text = ''
    export DEN_CLAUDE_STARTUP_INHERITED_MANIFEST=${inheritedSandbox.denManifest}
    export DEN_CLAUDE_STARTUP_EXPLICIT_MANIFEST=${explicitSandbox.denManifest}
    export DEN_CLAUDE_STARTUP_LAUNCHER=${fakes.launcher}/bin/den-launcher
    export DEN_CLAUDE_STARTUP_ACL_PROBE=${aclDiagnosticProbe}/bin/den-acl-diagnostic-probe
    ${builtins.readFile ./claude-startup-runtime-manifest.sh}
    ${builtins.readFile ./claude-startup-darwin.sh}
  '';
}
```

Do not add `__impureHostDeps` or `__noChroot`. `/bin/ls` remains only in manifest data and the host-executed diagnostic probe.

Make the new host script visible to flake source filtering without committing:

```bash
git add -N nix/check-support/claude-startup-darwin.sh
```

- [ ] **Step 4: Move the existing scenario body into the host script**

Start `nix/check-support/claude-startup-darwin.sh` with strict requirements:

```bash
# shellcheck shell=bash
set -euo pipefail

: "${DEN_NATIVE_HOST_ROOT:?native host root is required}"
: "${DEN_CLAUDE_STARTUP_INHERITED_MANIFEST:?inherited manifest is required}"
: "${DEN_CLAUDE_STARTUP_EXPLICIT_MANIFEST:?explicit manifest is required}"
: "${DEN_CLAUDE_STARTUP_LAUNCHER:?launcher is required}"
: "${DEN_CLAUDE_STARTUP_ACL_PROBE:?ACL probe is required}"

fixture_root=$DEN_NATIVE_HOST_ROOT/claude-startup
root=$fixture_root/root
outside=$fixture_root/outside
outside_link=$fixture_root/outside-link
rm -rf "$fixture_root"
mkdir -m 0700 -p "$root/home" "$root/worktree" "$fixture_root/manifests"
```

Replace all fixed paths with paths derived from these variables:

```bash
inside=$root/worktree/.den-claude
overlap_inside=$root/home/.claude/child
overlap_outside=$root/home
symlink_inside=$root/worktree/link-state
symlink_outside=$outside_link
```

The script can remove only `$fixture_root` and its descendants.

- [ ] **Step 5: Adapt manifests at runtime and launch through the packaged launcher**

Create one inherited runtime manifest and explicit manifests for each explicit scenario:

```bash
den_adapt_claude_startup_manifest \
  "$DEN_CLAUDE_STARTUP_INHERITED_MANIFEST" \
  "$fixture_root/manifests/inherited.json" inherited ""

den_adapt_claude_startup_manifest \
  "$DEN_CLAUDE_STARTUP_EXPLICIT_MANIFEST" \
  "$fixture_root/manifests/inside-explicit.json" explicit "$inside"
```

Use this launch interface throughout the moved body:

```bash
run_native() {
  local manifest=$1
  shift
  (
    cd "$root/worktree"
    exec "$DEN_CLAUDE_STARTUP_LAUNCHER" --manifest "$manifest" -- "$@"
  )
}
```

Use the inherited manifest for default and inherited scenarios. Use a separately adapted explicit manifest for each inside, outside, overlap, and symbolic-link scenario.

- [ ] **Step 6: Preserve every existing assertion**

Move the current shell assertions without reducing them. Keep:

- default home-state files and default policy writes.
- explicit and inherited custom paths inside and outside the worktree.
- mode `0700` and invoking-user ownership.
- no default-state files or write grants in custom mode.
- skill, plugin, hook, settings, and MCP checksums.
- selected-directory and account-home policy checks.
- canonical overlap rejection.
- final symbolic-link rejection.
- pre-Fence and pre-agent markers.
- token absence in rejected stderr.
- sanitized ACL diagnostics on launch failure.

Replace only path construction and wrapper invocation.

- [ ] **Step 7: Write the completion artifact only after the final assertion**

End the host script with:

```bash
printf 'complete\n' > "$DEN_NATIVE_HOST_ROOT/claude-startup.complete"
```

Do not create this file in cleanup or before rejected-path assertions finish.

- [ ] **Step 8: Run syntax, evaluation, and graph checks**

Run:

```bash
bash -n nix/check-support/claude-startup-darwin.sh
nix eval --raw .#checks.aarch64-darwin.claude-startup.denHostFixturePlatform
nix derivation show --recursive .#checks.aarch64-darwin.claude-startup \
  | python3 scripts/check-derivation-impure-host-deps.py
git diff --exit-code -- nix/check-support/claude-startup-linux.nix \
  nix/check-support/claude-startup.nix
```

Expected:

- shell syntax passes.
- the passthru value is `darwin`.
- the graph guard passes even though valid runtime command literals still contain `/bin/ls`.
- both Linux files have no diff.

Do not commit.

---

### Task 5: Package and execute the fixture through native enforcement

**Files:**
- Create: `tests/native-runner.sh`
- Modify: `modules/checks/native-enforcement.nix:1-11`
- Modify: `nix/check-support/native-enforcement.nix:1,178-224`
- Modify: `nix/check-support/native-runner.sh:9-115`
- Modify: `tests/native/native_test.go:1-35`
- Modify: `modules/checks/launcher-unit.nix:10-24`

**Interfaces:**
- Consumes: `claudeStartup` derivation argument in `native-enforcement.nix`.
- Produces on Darwin: `DEN_NATIVE_CLAUDE_STARTUP=/nix/store/.../bin/claude-startup`.
- Produces on Darwin: `DEN_NATIVE_SANDBOX_EXEC=/usr/bin/sandbox-exec` as a testable internal host-tool path.
- Consumes in Go: `$DEN_NATIVE_HOST_ROOT/claude-startup.complete` containing `complete\n`.

- [ ] **Step 1: Write the native-runner success and failure harness**

Create `tests/native-runner.sh` with two Darwin-mode runs. Build a temporary executable by concatenating fake lifecycle functions with the production runner:

```bash
cat > "$root/runner" <<'FAKE_LIFECYCLE'
#!/usr/bin/env bash
set -euo pipefail
resolver_deferred_signal=0
resolver_defer_signals() { :; }
resolver_restore_signals() { :; }
resolver_stop_helper_deferred() { return 0; }
resolver_lifecycle_status() { return "$1"; }
start_resolver_helper() { printf 'resolver\n' >> "$DEN_FAKE_EVENT_LOG"; }
stop_resolver_helper() { return 0; }
FAKE_LIFECYCLE
cat "$runner_source" >> "$root/runner"
chmod +x "$root/runner"
```

Create the fake settings, startup, and Go executables:

```bash
cat > "$root/settings-merge" <<'FAKE_SETTINGS'
#!/usr/bin/env bash
set -euo pipefail
printf 'settings\n' >> "$DEN_FAKE_EVENT_LOG"
printf 'fixture complete\n'
FAKE_SETTINGS

cat > "$root/claude-startup" <<'FAKE_STARTUP'
#!/usr/bin/env bash
set -euo pipefail
printf 'startup\n' >> "$DEN_FAKE_EVENT_LOG"
startup_status=${DEN_FAKE_STARTUP_STATUS:-0}
if [[ $startup_status -ne 0 ]]; then
  exit "$startup_status"
fi
printf 'complete\n' > "$DEN_NATIVE_HOST_ROOT/claude-startup.complete"
FAKE_STARTUP

cat > "$root/native-tests" <<'FAKE_GO'
#!/usr/bin/env bash
set -euo pipefail
[[ -f $DEN_NATIVE_HOST_ROOT/claude-startup.complete ]]
[[ $(<"$DEN_NATIVE_HOST_ROOT/claude-startup.complete") == complete ]]
printf 'go\n' >> "$DEN_FAKE_EVENT_LOG"
FAKE_GO

cat > "$root/resolver-helper" <<'FAKE_MARKER'
#!/usr/bin/env bash
exit 0
FAKE_MARKER
cp "$root/resolver-helper" "$root/sandbox-exec"
chmod +x "$root/settings-merge" "$root/claude-startup" "$root/native-tests" \
  "$root/resolver-helper" "$root/sandbox-exec"

mkdir -p "$root/home" "$root/runtime"
export HOME=$root/home
export XDG_RUNTIME_DIR=$root/runtime
export DEN_NATIVE_HOST_SYSTEM=aarch64-darwin
export DEN_NATIVE_SETTINGS_MERGE=$root/settings-merge
export DEN_NATIVE_CLAUDE_STARTUP=$root/claude-startup
export DEN_NATIVE_TEST_BINARY=$root/native-tests
export DEN_NATIVE_RESOLVER_HELPER=$root/resolver-helper
export DEN_NATIVE_SANDBOX_EXEC=$root/sandbox-exec
export DEN_FAKE_EVENT_LOG=$root/events
```

Make the new harness visible to flake source filtering without committing:

```bash
git add -N tests/native-runner.sh
```

For the success case, require exact order:

```text
settings
startup
resolver
go
```

For the failure case, set `DEN_FAKE_STARTUP_STATUS=19`. Require runner status `19` and exact events:

```text
settings
startup
```

This proves resolver and Go execution do not continue.

- [ ] **Step 2: Run the native-runner harness and observe RED**

Run:

```bash
bash tests/native-runner.sh nix/check-support/native-runner.sh
```

Expected: FAIL because the current runner neither requires nor executes `DEN_NATIVE_CLAUDE_STARTUP`.

- [ ] **Step 3: Write Go tests for the completion contract and observe RED**

In `tests/native/native_test.go`, add table-driven coverage for a helper named:

```go
func requireClaudeStartupCompletion(goos, root string) error
```

Cover:

- Linux with an empty root returns nil.
- Darwin with an empty root returns an error.
- Darwin with a missing artifact returns an error.
- Darwin with wrong content returns an error.
- Darwin with `complete\n` returns nil.

Run with the existing native preflight variables set to absolute unused paths:

```bash
env \
  DEN_NATIVE_CLAUDE=/tmp/den-native-unused \
  DEN_NATIVE_SANDBOX=/tmp/den-native-unused \
  DEN_NATIVE_MANIFEST=/tmp/den-native-unused \
  DEN_NATIVE_LAUNCHER=/tmp/den-native-unused \
  DEN_NATIVE_FENCE=/tmp/den-native-unused \
  DEN_NATIVE_REPOWOLF_CLIENT_DIR=/tmp/den-native-unused \
  DEN_NATIVE_REPOWOLF_FIXTURE=/tmp/den-native-unused \
  DEN_NATIVE_SETTINGS_MERGE=/tmp/den-native-unused \
  DEN_NATIVE_UNRELATED_STORE_FILE=/tmp/den-native-unused \
  go test -tags=native ./tests/native \
    -run TestClaudeStartupCompletionContract -count=1
```

Expected: FAIL because the helper does not exist.

- [ ] **Step 4: Pass the selected startup output into native enforcement**

Change `modules/checks/native-enforcement.nix` to create the selected output once and pass it:

```nix
perSystem = { pkgs, self', ... }:
  let
    claudeStartup = import ../../nix/check-support/claude-startup.nix {
      inherit inputs pkgs;
    };
  in
  {
    checks.native-enforcement = import ../../nix/check-support/native-enforcement.nix {
      inherit inputs pkgs claudeStartup;
      claude = self'.packages.claude;
    };
  };
```

Change the support function signature:

```nix
{ inputs, pkgs, claude, claudeStartup ? null }:
```

Before the final package expression, require the Darwin package marker:

```nix
assert pkgs.lib.assertMsg
  (!pkgs.stdenv.isDarwin ||
    (claudeStartup != null &&
     (claudeStartup.denHostFixturePlatform or null) == "darwin"))
  "Darwin native enforcement requires the packaged Darwin Claude startup fixture";
```

- [ ] **Step 5: Export Darwin-only fixture paths from the package**

Inside the `native-enforcement` application text, add:

```nix
${pkgs.lib.optionalString pkgs.stdenv.isDarwin ''
  export DEN_NATIVE_CLAUDE_STARTUP=${claudeStartup}/bin/claude-startup
  export DEN_NATIVE_SANDBOX_EXEC=/usr/bin/sandbox-exec
''}
```

Do not export the Darwin fixture on Linux. Do not add the Linux startup derivation to the Linux native-runner closure.

- [ ] **Step 6: Execute the fixture before resolver startup**

In the Darwin branch of `native-runner.sh`, require both private paths:

```bash
: "${DEN_NATIVE_CLAUDE_STARTUP:?packaged Darwin Claude startup fixture is required}"
: "${DEN_NATIVE_SANDBOX_EXEC:?Darwin sandbox-exec path is required}"
test -x "$DEN_NATIVE_CLAUDE_STARTUP"
test -x "$DEN_NATIVE_SANDBOX_EXEC"
```

After creating `DEN_NATIVE_HOST_ROOT` and before `start_resolver_helper`, add:

```bash
printf 'executing Darwin Claude startup fixture as the invoking host user\n'
"$DEN_NATIVE_CLAUDE_STARTUP"
completion=$DEN_NATIVE_HOST_ROOT/claude-startup.complete
if [[ ! -f $completion || $(<"$completion") != complete ]]; then
  printf 'Darwin Claude startup fixture did not produce its completion artifact\n' >&2
  exit 1
fi
```

Keep settings merge before host-root allocation. Keep resolver startup after this block.

- [ ] **Step 7: Enforce completion in the native Go preflight**

Add `runtime` to the imports. Implement:

```go
const claudeStartupCompletion = "claude-startup.complete"

func requireClaudeStartupCompletion(goos, root string) error {
	if goos != "darwin" {
		return nil
	}
	if root == "" {
		return fmt.Errorf("Darwin Claude startup completion requires DEN_NATIVE_HOST_ROOT")
	}
	path := filepath.Join(root, claudeStartupCompletion)
	contents, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read Darwin Claude startup completion: %w", err)
	}
	if string(contents) != "complete\n" {
		return fmt.Errorf("unexpected Darwin Claude startup completion content: %q", contents)
	}
	return nil
}
```

Call it in `TestMain` before `m.Run()`:

```go
if err := requireClaudeStartupCompletion(runtime.GOOS, os.Getenv("DEN_NATIVE_HOST_ROOT")); err != nil {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
```

- [ ] **Step 8: Add the runner harness to `checks.launcher-unit`**

Run it after the native-driver test:

```nix
${pkgs.bash}/bin/bash tests/native-runner.sh \
  "$PWD/nix/check-support/native-runner.sh"
```

- [ ] **Step 9: Run focused tests and observe GREEN**

Run:

```bash
bash tests/native-runner.sh nix/check-support/native-runner.sh
env \
  DEN_NATIVE_CLAUDE=/tmp/den-native-unused \
  DEN_NATIVE_SANDBOX=/tmp/den-native-unused \
  DEN_NATIVE_MANIFEST=/tmp/den-native-unused \
  DEN_NATIVE_LAUNCHER=/tmp/den-native-unused \
  DEN_NATIVE_FENCE=/tmp/den-native-unused \
  DEN_NATIVE_REPOWOLF_CLIENT_DIR=/tmp/den-native-unused \
  DEN_NATIVE_REPOWOLF_FIXTURE=/tmp/den-native-unused \
  DEN_NATIVE_SETTINGS_MERGE=/tmp/den-native-unused \
  DEN_NATIVE_UNRELATED_STORE_FILE=/tmp/den-native-unused \
  go test -tags=native ./tests/native \
    -run TestClaudeStartupCompletionContract -count=1
system=$(nix eval --impure --raw --expr builtins.currentSystem)
nix build ".#checks.$system.launcher-unit" --no-link --print-build-logs
nix derivation show .#checks.aarch64-darwin.native-enforcement >/dev/null
```

Expected: PASS. Darwin native-enforcement evaluation accepts the Darwin marker and packages the fixture.

Do not commit.

---

### Task 6: Remove the workflow daemon override and validate configuration directly

**Files:**
- Modify: `.github/workflows/checks.yml:16-48`
- No new workflow-content test file.

**Interfaces:**
- Preserves: four exact system and runner pairs.
- Preserves: `sandbox = true` on Linux and `sandbox = relaxed` on Darwin.
- Removes: all `nix_allowed_impure_host_deps` matrix fields.
- Removes: the installer `allowed-impure-host-deps` assignment.

- [ ] **Step 1: Remove only the obsolete workflow fields**

For every matrix entry, delete:

```yaml
nix_allowed_impure_host_deps: ...
```

Keep each `nix_sandbox` field. In the installer `extra-conf`, keep:

```yaml
extra-conf: |
  sandbox = ${{ matrix.nix_sandbox }}
```

Delete only:

```yaml
allowed-impure-host-deps = ${{ matrix.nix_allowed_impure_host_deps }}
```

- [ ] **Step 2: Build pinned Actionlint and yq tools from the flake input**

Run:

```bash
ACTIONLINT=$(nix build --no-link --print-out-paths --impure --expr \
  'let f = builtins.getFlake (toString ./.); s = builtins.currentSystem; in f.inputs.nixpkgs.legacyPackages.${s}.actionlint')
YQ=$(nix build --no-link --print-out-paths --impure --expr \
  'let f = builtins.getFlake (toString ./.); s = builtins.currentSystem; in f.inputs.nixpkgs.legacyPackages.${s}.yq-go')
```

Require each command to return one `/nix/store/...` path.

- [ ] **Step 3: Run Actionlint**

Run:

```bash
"$ACTIONLINT/bin/actionlint" .github/workflows/checks.yml
```

Expected: PASS with no output.

- [ ] **Step 4: Validate the matrix and installer structure without source-text matching**

Run:

```bash
"$YQ/bin/yq" -e '
  (.jobs.native.strategy.matrix.include | length) == 4 and
  .jobs.native.strategy.matrix.include[0].system == "x86_64-linux" and
  .jobs.native.strategy.matrix.include[1].system == "aarch64-linux" and
  .jobs.native.strategy.matrix.include[2].system == "x86_64-darwin" and
  .jobs.native.strategy.matrix.include[3].system == "aarch64-darwin" and
  (([.jobs.native.strategy.matrix.include[] |
      has("nix_allowed_impure_host_deps")] | any | not)) and
  (([.jobs.native.steps[].with."extra-conf" // "" |
      test("(^|\\n)[[:space:]]*allowed-impure-host-deps[[:space:]]*=")] |
    any | not))
' .github/workflows/checks.yml >/dev/null
```

Expected: PASS.

This is direct configuration verification. Do not add raw YAML assertions to repository tests.

- [ ] **Step 5: Re-run the native-driver test**

Run:

```bash
bash tests/check-native-driver.sh scripts/check-native.sh
```

Expected: PASS and no daemon-policy query in any fake system flow.

Do not commit.

---

### Task 7: Run focused, full, and mutation verification

**Files:**
- Verify all changed production, test, workflow, spec, and plan files.
- Temporarily mutate files only through `/tmp` backups, then restore them.

**Interfaces:**
- Produces: fresh command output for each required behavior.
- Produces: expected failure evidence for all seven mutation cases.
- Leaves: the intended working tree unchanged after mutation restoration.

- [ ] **Step 1: Run format and syntax checks**

Run:

```bash
git diff --check
python3 -m py_compile \
  scripts/check-derivation-impure-host-deps.py \
  tests/check-derivation-impure-host-deps.py
bash -n \
  scripts/check-native.sh \
  tests/check-native-driver.sh \
  tests/claude-startup-runtime-manifest.sh \
  tests/native-runner.sh \
  nix/check-support/claude-startup-runtime-manifest.sh \
  nix/check-support/claude-startup-darwin.sh \
  nix/check-support/native-runner.sh
gofmt -d tests/native/native_test.go
```

Expected: every command passes and `gofmt -d` prints nothing.

- [ ] **Step 2: Run all focused behavioral tests**

Run:

```bash
python3 tests/check-derivation-impure-host-deps.py \
  scripts/check-derivation-impure-host-deps.py
bash tests/check-native-driver.sh scripts/check-native.sh
bash tests/claude-startup-runtime-manifest.sh \
  nix/check-support/claude-startup-runtime-manifest.sh
bash tests/native-runner.sh nix/check-support/native-runner.sh
env \
  DEN_NATIVE_CLAUDE=/tmp/den-native-unused \
  DEN_NATIVE_SANDBOX=/tmp/den-native-unused \
  DEN_NATIVE_MANIFEST=/tmp/den-native-unused \
  DEN_NATIVE_LAUNCHER=/tmp/den-native-unused \
  DEN_NATIVE_FENCE=/tmp/den-native-unused \
  DEN_NATIVE_REPOWOLF_CLIENT_DIR=/tmp/den-native-unused \
  DEN_NATIVE_REPOWOLF_FIXTURE=/tmp/den-native-unused \
  DEN_NATIVE_SETTINGS_MERGE=/tmp/den-native-unused \
  DEN_NATIVE_UNRELATED_STORE_FILE=/tmp/den-native-unused \
  go test -tags=native ./tests/native \
    -run TestClaudeStartupCompletionContract -count=1
```

Expected: PASS.

- [ ] **Step 3: Run focused Nix evaluation and builds**

Run:

```bash
system=$(nix eval --impure --raw --expr builtins.currentSystem)
nix flake check --no-build
nix eval --raw .#checks.aarch64-darwin.claude-startup.denHostFixturePlatform
nix derivation show --recursive \
  .#checks.aarch64-darwin.claude-startup \
  .#checks.aarch64-darwin.native-enforcement \
  | python3 scripts/check-derivation-impure-host-deps.py
nix build \
  ".#packages.$system.claude" \
  ".#checks.$system.claude-startup" \
  ".#checks.$system.launcher-unit" \
  ".#checks.$system.native-enforcement" \
  --no-link --print-build-logs
```

Expected: PASS. The marker command prints `darwin`.

- [ ] **Step 4: Run the current host's native flow**

Run:

```bash
scripts/check-native.sh "$system"
```

Expected: PASS. On Linux, this also proves that the unchanged Linux startup check and native runner still operate.

- [ ] **Step 5: Run the full local flake check**

Run:

```bash
nix flake check --accept-flake-config --print-build-logs
```

Expected: PASS.

- [ ] **Step 6: Mutation 1 — reintroduce `/bin/ls` as an impure dependency**

Back up `nix/check-support/claude-startup-darwin.nix` to `/tmp/den-claude-startup-darwin.nix`. Temporarily add this member to the `writeShellApplication` arguments:

```nix
derivationArgs = {
  __impureHostDeps = [ "/bin/ls" ];
  passthru.denHostFixturePlatform = "darwin";
};
```

Run:

```bash
set +e
nix derivation show --recursive .#checks.aarch64-darwin.claude-startup \
  | python3 scripts/check-derivation-impure-host-deps.py
status=$?
set -e
[[ $status -eq 1 ]]
```

Expected: the guard reports `/bin/ls` in `__impureHostDeps`. Restore the backup and rerun the guard successfully.

- [ ] **Step 7: Mutation 2 — remove Darwin startup packaging**

Back up `modules/checks/native-enforcement.nix`. Temporarily pass `claudeStartup = null;` into `native-enforcement.nix`.

Run:

```bash
set +e
nix derivation show .#checks.aarch64-darwin.native-enforcement \
  >/tmp/den-missing-startup.out 2>/tmp/den-missing-startup.err
status=$?
set -e
[[ $status -ne 0 ]]
grep -Fq 'Darwin native enforcement requires the packaged Darwin Claude startup fixture' \
  /tmp/den-missing-startup.err
```

Restore the file. The same evaluation must then pass.

- [ ] **Step 8: Mutation 3 — remove Darwin startup execution**

Back up `nix/check-support/native-runner.sh`. Temporarily replace the exact fixture call:

```bash
"$DEN_NATIVE_CLAUDE_STARTUP"
```

with:

```bash
:
```

Run:

```bash
set +e
bash tests/native-runner.sh nix/check-support/native-runner.sh
status=$?
set -e
[[ $status -ne 0 ]]
```

Expected: the harness rejects the missing startup event or completion artifact. Restore the file and require the harness to pass.

- [ ] **Step 9: Mutation 4 — force startup failure and prove fail-fast order**

Run:

```bash
bash tests/native-runner.sh nix/check-support/native-runner.sh
```

The harness must set `DEN_FAKE_STARTUP_STATUS=19` for its failure subcase. It must assert runner status `19` and exact events:

```text
settings
startup
```

The harness must reject any `resolver` or `go` event.

- [ ] **Step 10: Mutation 5 — restore the workflow override**

Back up `.github/workflows/checks.yml`. Resolve the pinned yq path in this shell:

```bash
YQ=$(nix build --no-link --print-out-paths --impure --expr \
  'let f = builtins.getFlake (toString ./.); s = builtins.currentSystem; in f.inputs.nixpkgs.legacyPackages.${s}.yq-go')
```

Use yq to add a matrix field and installer assignment:

```bash
"$YQ/bin/yq" -i '
  .jobs.native.strategy.matrix.include[].nix_allowed_impure_host_deps = "/bin/ls" |
  (.jobs.native.steps[] | select(.name == "Install Nix") |
    .with."extra-conf") +=
      "allowed-impure-host-deps = ${matrix.nix_allowed_impure_host_deps}\n"
' .github/workflows/checks.yml
```

Run the structured Step 6.4 expression. Expected: FAIL. Restore the workflow and require Actionlint plus the structured expression to pass.

- [ ] **Step 11: Mutation 6 — remove one matrix target**

Back up the workflow. Temporarily remove `aarch64-darwin` structurally:

```bash
"$YQ/bin/yq" -i '
  .jobs.native.strategy.matrix.include |=
    map(select(.system != "aarch64-darwin"))
' .github/workflows/checks.yml
```

Run the structured Step 6.4 expression. Expected: FAIL. Restore the workflow and require the expression to pass.

- [ ] **Step 12: Mutation 7 — route Darwin startup through Linux**

Back up `nix/check-support/claude-startup.nix`. Temporarily change its Darwin branch to:

```nix
else
  import ./claude-startup-linux.nix { inherit inputs pkgs; }
```

Run:

```bash
set +e
nix derivation show .#checks.aarch64-darwin.native-enforcement \
  >/tmp/den-linux-route.out 2>/tmp/den-linux-route.err
status=$?
set -e
[[ $status -ne 0 ]]
grep -Fq 'Darwin native enforcement requires the packaged Darwin Claude startup fixture' \
  /tmp/den-linux-route.err
```

Restore the selector and require Darwin native-enforcement evaluation to pass.

- [ ] **Step 13: Prove all mutation backups were restored**

Run:

```bash
git status --short
git diff --check
git diff --exit-code -- nix/check-support/claude-startup-linux.nix
```

Expected: only intended implementation, test, workflow, spec, and plan changes remain. No backup files remain in the repository.

- [ ] **Step 14: Re-run full verification after all mutations**

Repeat Steps 1 through 5 with fresh output.

Do not commit.

---

### Task 8: Obtain independent review, commit, push, and monitor both CI runs

**Files:**
- Review: the complete uncommitted diff against `88624e7afe7abf947041d7f9e231dea2150483cc`.
- Commit documentation: `docs/specs/2026-08-23-darwin-claude-startup-host-fixture-design.md`, `docs/plans/2026-08-23-darwin-claude-startup-host-fixture.md`.
- Commit implementation: every production, test, workflow, and check-wiring file from Tasks 1-6.

**Interfaces:**
- Consumes: passing focused, full, and mutation evidence.
- Produces: fresh adversarial review with no unresolved blocking findings.
- Produces: focused Conventional Commits on `feat/den-claude-sandbox`.
- Produces: passing push and pull-request runs at the same head SHA.

- [ ] **Step 1: Prepare the review evidence without committing**

Run:

```bash
git status --short
git diff --check
git diff --stat
git diff -- . ':(exclude)docs/plans/2026-08-23-darwin-claude-startup-host-fixture.md'
```

Record:

- base SHA `88624e7afe7abf947041d7f9e231dea2150483cc`.
- current branch `feat/den-claude-sandbox`.
- focused and full verification commands.
- all mutation outcomes.
- known platform limit: native Darwin execution is available only in CI unless the current host is Darwin.

- [ ] **Step 2: Request fresh independent review**

Use the canonical Pi `reviewer` with fresh context. Resolve its active model and thinking override first. Give it:

- the approved spec path.
- this implementation plan path.
- the complete working-tree diff.
- base SHA `88624e7afe7abf947041d7f9e231dea2150483cc`.
- the constraints from this plan.
- the exact verification and mutation evidence.
- the requirement to inspect security, ACL semantics, manifest adaptation, host-root cleanup, fail-fast order, Linux preservation, graph parsing, and workflow coverage.

Require findings with file and line references. Do not ask the reviewer to edit files.

- [ ] **Step 3: Resolve review findings technically**

Use the `receiving-code-review` skill before changing code from review feedback.

For each finding:

1. reproduce or inspect the claimed behavior.
2. accept only findings that match the code and requirements.
3. make the narrowest correction.
4. rerun the affected focused test.
5. rerun Tasks 7.1 through 7.5 after the final correction.
6. rerun any mutation affected by the correction.

Request a second fresh review if the first review found a blocking issue or the correction changed the architecture.

- [ ] **Step 4: Read the commit skill and make the documentation commit**

Read `/home/roche/.pi/agent/skills/commit/SKILL.md` before committing.

Stage only the approved artifacts:

```bash
git add \
  docs/specs/2026-08-23-darwin-claude-startup-host-fixture-design.md \
  docs/plans/2026-08-23-darwin-claude-startup-host-fixture.md
git diff --cached --check
git commit -m "docs: design Darwin startup host fixture"
```

- [ ] **Step 5: Make the focused implementation commit**

Stage exactly:

```bash
git add \
  .github/workflows/checks.yml \
  scripts/check-native.sh \
  scripts/check-derivation-impure-host-deps.py \
  tests/check-native-driver.sh \
  tests/check-derivation-impure-host-deps.py \
  tests/claude-startup-runtime-manifest.sh \
  tests/native-runner.sh \
  nix/check-support/claude-startup-runtime-manifest.sh \
  nix/check-support/claude-startup-darwin.nix \
  nix/check-support/claude-startup-darwin.sh \
  nix/check-support/native-enforcement.nix \
  nix/check-support/native-runner.sh \
  modules/checks/native-enforcement.nix \
  modules/checks/launcher-unit.nix \
  tests/native/native_test.go
git diff --cached --check
git diff --cached --stat
git commit -m "fix(ci): run Darwin startup fixture on host"
```

Run `git status --short` and require a clean tree.

- [ ] **Step 6: Push the existing branch without merging**

Run:

```bash
git push origin HEAD:feat/den-claude-sandbox
head_sha=$(git rev-parse HEAD)
```

Do not force-push. Do not merge PR #1.

- [ ] **Step 7: Find both workflow runs for the pushed SHA**

Use `gh` against `rochecompaan/den`. Wait for one `push` run and one `pull_request` run whose `headSha` equals `$head_sha`:

```bash
for attempt in $(seq 1 30); do
  runs=$(gh run list --repo rochecompaan/den \
    --branch feat/den-claude-sandbox --limit 20 \
    --json databaseId,event,headSha,status,conclusion,url)
  push_id=$(jq -r --arg sha "$head_sha" \
    '.[] | select(.headSha == $sha and .event == "push") | .databaseId' \
    <<<"$runs" | head -1)
  pr_id=$(jq -r --arg sha "$head_sha" \
    '.[] | select(.headSha == $sha and .event == "pull_request") | .databaseId' \
    <<<"$runs" | head -1)
  [[ -n $push_id && -n $pr_id ]] && break
  sleep 10
done
[[ -n $push_id && -n $pr_id ]]
```

- [ ] **Step 8: Monitor both runs to completion**

Run:

```bash
gh run watch "$push_id" --repo rochecompaan/den --exit-status
gh run watch "$pr_id" --repo rochecompaan/den --exit-status
```

Expected: both commands exit zero. If either fails, inspect its logs, apply systematic debugging, and stop with exact blocker evidence if the fault cannot be corrected safely.

- [ ] **Step 9: Capture exact Darwin build and execution evidence**

For each run ID, retrieve the job list and Darwin logs. Require both Darwin architectures to contain these lines in order:

```text
building Claude for <darwin-system>
building non-native check claude-startup for <darwin-system>
building native runner for <darwin-system>
executing native runner as the invoking host user
executing Claude settings merge fixture as the invoking host user
executing Darwin Claude startup fixture as the invoking host user
```

Also require the graph-guard stage:

```text
checking Darwin derivation graph for forbidden /bin/ls impure dependencies
```

Confirm that all Linux and Darwin jobs passed in both runs.

- [ ] **Step 10: Confirm PR state and report without merging**

Run:

```bash
gh pr view 1 --repo rochecompaan/den \
  --json number,state,mergeStateStatus,headRefName,headRefOid,url
```

Require:

- `state` is `OPEN`.
- `headRefName` is `feat/den-claude-sandbox`.
- `headRefOid` equals `$head_sha`.

Report:

- the red regression evidence.
- focused, full, and mutation outcomes.
- independent review outcome.
- commit SHAs.
- push and pull-request run URLs and conclusions.
- exact Darwin build and execution lines for both architectures.
- PR #1 remains open and unmerged.
