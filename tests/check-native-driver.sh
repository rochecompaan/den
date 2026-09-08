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
printf '%s\n' 'event execute native-runner' >&2
FAKE_RUNNER
chmod +x "$fake_runner/bin/native-enforcement"

cat > "$root/bin/df" <<'FAKE_DF'
#!/usr/bin/env bash
if [[ ${DEN_FAKE_DF_STATUS:-0} -ne 0 ]]; then
  exit "$DEN_FAKE_DF_STATUS"
fi
printf '%s\n' 'Filesystem 1024-blocks Used Available Capacity Mounted on'
printf '%s\n' '/dev/fake 100000 90000 10000 90% /'
FAKE_DF

cat > "$root/bin/du" <<'FAKE_DU'
#!/usr/bin/env bash
if [[ ${DEN_FAKE_DU_STATUS:-0} -ne 0 ]]; then
  exit "$DEN_FAKE_DU_STATUS"
fi
printf '1\t%s\n' "${*: -1}"
FAKE_DU
chmod +x "$root/bin/df" "$root/bin/du"

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
  printf '%s\n' 'event build claude' >&2
  exit
fi
if [[ $1 == build && $* == *packages.*.pi* ]]; then
  printf '%s\n' 'build pi' >> "$DEN_FAKE_EVENT_LOG"
  exit
fi
if [[ $1 == build && $* == *native-enforcement* ]]; then
  printf '%s\n' 'build native-runner' >> "$DEN_FAKE_EVENT_LOG"
  printf '%s\n' 'event build native-runner' >&2
  printf '%s\n' "$DEN_FAKE_RUNNER"
  exit
fi
if [[ $1 == build && $* == *checks.* ]]; then
  for argument in "$@"; do
    case "$argument" in
      .#checks.*) printf 'build %s\n' "${argument##*.}" >> "$DEN_FAKE_EVENT_LOG"; exit ;;
    esac
  done
fi
FAKE_NIX
chmod +x "$root/bin/nix"

export DEN_FAKE_STORE_DIR=$fake_store
export DEN_FAKE_RUNNER=$fake_runner
export DEN_FAKE_EVENT_LOG=$root/events.log
export DEN_FAKE_DERIVATION_JSON='{"derivations":{},"version":3}'
export DEN_FAKE_NIX_LOG=$root/nix.log
normal_checks=$'claude-startup\nlauncher-unit\npi-adapter\npi-module-api\npi-package\npi-package-api\npi-resources\npi-security-extension'
darwin_pi_graph='{"derivations":{"pi-startup.drv":{"name":"pi-darwin-startup"},"pi-tests.drv":{"name":"den-pi-native-tests"},"pi-launcher.drv":{"name":"den-native-pi-launcher"}},"version":3}'

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

run_driver missing-pi-check x86_64-linux 0 $'claude-startup\nlauncher-unit\npi-adapter\npi-module-api\npi-package\npi-package-api\npi-resources' "$darwin_pi_graph"
if [[ $status -eq 0 ]]; then
  printf 'driver accepted normal checks without pi-security-extension\n' >&2
  exit 1
fi
! grep -Fq 'checks.x86_64-linux.native-enforcement' "$DEN_FAKE_NIX_LOG"

missing_pi_startup_graph='{"derivations":{"pi-tests.drv":{"name":"den-pi-native-tests"},"pi-launcher.drv":{"name":"den-native-pi-launcher"}},"version":3}'
run_driver missing-pi-startup-graph x86_64-linux 0 "$normal_checks" "$missing_pi_startup_graph"
if [[ $status -eq 0 ]]; then
  printf 'driver accepted Darwin graph without pi-darwin-startup\n' >&2
  exit 1
fi
! grep -Fq 'checks.x86_64-linux.native-enforcement' "$DEN_FAKE_NIX_LOG"

home_was_set=${HOME+x}
home_value=${HOME-}
unset HOME
export DEN_CI_DISK_TELEMETRY=1 DEN_FAKE_DF_STATUS=47 DEN_FAKE_DU_STATUS=48
run_driver darwin-telemetry aarch64-darwin 0 "$normal_checks" "$darwin_pi_graph"
unset DEN_CI_DISK_TELEMETRY DEN_FAKE_DF_STATUS DEN_FAKE_DU_STATUS
if [[ -n $home_was_set ]]; then
  export HOME=$home_value
fi
if [[ $status -ne 0 ]]; then
  cat "$root/darwin-telemetry.stderr" >&2
  exit 1
fi
expected=$'darwin disk telemetry phase=before-builds\nevent build claude\ndarwin disk telemetry phase=after-claude-build\nevent build native-runner\ndarwin disk telemetry phase=before-native-runner-execution\nevent execute native-runner'
actual=$(grep -E '^(darwin disk telemetry phase=|event (build|execute))' \
  "$root/darwin-telemetry.stderr")
[[ $actual == "$expected" ]]

for system in x86_64-linux aarch64-linux x86_64-darwin aarch64-darwin; do
  label=${system//_/-}
  run_driver "$label" "$system" 0 "$normal_checks" "$darwin_pi_graph"
  if [[ $status -ne 0 ]]; then
    cat "$root/$label.stderr" >&2
    exit 1
  fi
  expected=$'build claude\nbuild pi\nbuild native-runner\nexecute native-runner\nbuild claude-startup\nbuild launcher-unit\nbuild pi-adapter\nbuild pi-module-api\nbuild pi-package\nbuild pi-package-api\nbuild pi-resources\nbuild pi-security-extension'
  actual=$(grep -E '^(build claude|build pi|build claude-startup|build launcher-unit|build pi-adapter|build pi-module-api|build pi-package|build pi-package-api|build pi-resources|build pi-security-extension|build native-runner|execute native-runner)$' \
    "$DEN_FAKE_EVENT_LOG")
  [[ $actual == "$expected" ]]
  ! grep -Fq 'config show allowed-impure-host-deps' "$DEN_FAKE_NIX_LOG"
done

safe_runtime_literal='{"derivations":{"pi-startup.drv":{"name":"pi-darwin-startup"},"pi-tests.drv":{"name":"den-pi-native-tests"},"pi-launcher.drv":{"name":"den-native-pi-launcher"},"fixture.drv":{"env":{"builderScript":"exec /bin/ls -lde /tmp/state"}}},"version":3}'
run_driver darwin-safe-runtime-literal aarch64-darwin 0 "$normal_checks" "$safe_runtime_literal"
if [[ $status -ne 0 ]]; then
  cat "$root/darwin-safe-runtime-literal.stderr" >&2
  exit 1
fi
grep -Fq 'derivation show --recursive .#checks.x86_64-darwin.native-enforcement' "$DEN_FAKE_NIX_LOG"
grep -Fq 'derivation show --recursive .#checks.aarch64-darwin.claude-startup' "$DEN_FAKE_NIX_LOG"

nested_forbidden='{"derivations":{"pi-startup.drv":{"name":"pi-darwin-startup"},"pi-tests.drv":{"name":"den-pi-native-tests"},"pi-launcher.drv":{"name":"den-native-pi-launcher"},"parent.drv":{"inputDrvs":{"child.drv":{"outputs":["out"]}}},"child.drv":{"structuredAttrs":{"__impureHostDeps":["/bin/sh","/bin/ls"]}}},"version":3}'
run_driver darwin-nested-forbidden aarch64-darwin 0 "$normal_checks" "$nested_forbidden"
if [[ $status -eq 0 ]]; then
  printf 'driver accepted nested forbidden Darwin impure host dependency\n' >&2
  exit 1
fi
grep -Fq 'derivations.child.drv.structuredAttrs.__impureHostDeps' \
  "$root/darwin-nested-forbidden.stderr"
! grep -Fq 'build native-runner' "$DEN_FAKE_EVENT_LOG"
