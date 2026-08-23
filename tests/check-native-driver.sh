#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  printf 'usage: %s <check-native-script>\n' "${0##*/}" >&2
  exit 2
fi

driver=$1
root=$(mktemp -d)
cleanup() { rm -rf "$root"; }
trap cleanup EXIT HUP INT TERM
fake_store=$root/store
fake_runner=$fake_store/fake-native-runner
mkdir -p "$root/bin" "$fake_runner/bin"

printf '#!%s\n' "$BASH" > "$fake_runner/bin/native-enforcement"
cat >> "$fake_runner/bin/native-enforcement" <<'FAKE_RUNNER'
set -euo pipefail
printf '%s\n' 'execute native-runner' >> "$DEN_FAKE_EVENT_LOG"
FAKE_RUNNER
chmod +x "$fake_runner/bin/native-enforcement"

printf '#!%s\n' "$BASH" > "$root/bin/nix"
cat >> "$root/bin/nix" <<'FAKE_NIX'
set -euo pipefail
printf '%s\n' "$*" >> "$DEN_FAKE_NIX_LOG"
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
FAKE_NIX
chmod +x "$root/bin/nix"

export DEN_FAKE_STORE_DIR=$fake_store
export DEN_FAKE_RUNNER=$fake_runner
export DEN_FAKE_EVENT_LOG=$root/events.log
export DEN_FAKE_DERIVATION_JSON='{"derivations":{},"version":3}'
export DEN_FAKE_NIX_LOG=$root/nix.log

run_driver() {
  local label=$1
  local system=$2
  local apply_status=$3
  local normal_checks=$4
  local derivation_json=${5-'{"derivations":{},"version":3}'}
  export DEN_FAKE_CURRENT_SYSTEM=$system
  export DEN_FAKE_APPLY_STATUS=$apply_status
  export DEN_FAKE_NORMAL_CHECKS=$normal_checks
  export DEN_FAKE_DERIVATION_JSON=$derivation_json
  : > "$DEN_FAKE_EVENT_LOG"
  : > "$DEN_FAKE_NIX_LOG"
  set +e
  PATH="$root/bin:$PATH" "$BASH" "$driver" "$system" \
    > "$root/$label.stdout" 2> "$root/$label.stderr"
  status=$?
  set -e
}

run_driver linux-enumeration x86_64-linux 41 ''
if [[ $status -eq 0 ]]; then
  printf 'driver accepted forced normal-check enumeration failure\n' >&2
  exit 1
fi
! grep -Fq 'checks.x86_64-linux.native-enforcement' "$DEN_FAKE_NIX_LOG"

run_driver missing-claude-startup x86_64-linux 0 launcher-unit
if [[ $status -eq 0 ]]; then
  printf 'driver accepted normal checks without claude-startup\n' >&2
  exit 1
fi
! grep -Fq 'checks.x86_64-linux.native-enforcement' "$DEN_FAKE_NIX_LOG"

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

safe_runtime_literal='{"derivations":{"fixture.drv":{"env":{"builderScript":"exec /bin/ls -lde /tmp/state"}}},"version":3}'
run_driver darwin-safe-runtime-literal aarch64-darwin 0 $'claude-startup\nlauncher-unit' "$safe_runtime_literal"
if [[ $status -ne 0 ]]; then
  cat "$root/darwin-safe-runtime-literal.stderr" >&2
  exit 1
fi
grep -Fq 'derivation show --recursive .#checks.aarch64-darwin.claude-startup .#checks.aarch64-darwin.native-enforcement' \
  "$DEN_FAKE_NIX_LOG"

nested_forbidden='{"derivations":{"parent.drv":{"inputDrvs":{"child.drv":{"outputs":["out"]}}},"child.drv":{"structuredAttrs":{"__impureHostDeps":["/bin/sh","/bin/ls"]}}},"version":3}'
run_driver darwin-nested-forbidden aarch64-darwin 0 $'claude-startup\nlauncher-unit' "$nested_forbidden"
if [[ $status -eq 0 ]]; then
  printf 'driver accepted nested forbidden Darwin impure host dependency\n' >&2
  exit 1
fi
grep -Fq 'derivations.child.drv.structuredAttrs.__impureHostDeps' \
  "$root/darwin-nested-forbidden.stderr"
! grep -Fq 'build native-runner' "$DEN_FAKE_EVENT_LOG"
