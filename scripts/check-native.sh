#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

if [[ $# -ne 1 || -z $1 ]]; then
  printf 'usage: %s <system>\n' "${0##*/}" >&2
  exit 2
fi

system=$1
current_system=$(nix eval --impure --raw --expr builtins.currentSystem)
if [[ $system != "$current_system" ]]; then
  printf 'native checks require current system %s, got %s\n' "$current_system" "$system" >&2
  exit 2
fi

report_darwin_disk_telemetry() {
  local phase=$1 path
  local home=${HOME:-}
  local -a filesystem_paths=("$repo_root" /private/tmp /nix/store)
  local -a usage_paths=(/nix/store "$repo_root")
  if [[ $system != *-darwin || ${DEN_CI_DISK_TELEMETRY:-0} != 1 ]]; then
    return
  fi

  if [[ -n $home ]]; then
    filesystem_paths+=("$home")
    usage_paths+=("$home/Library/Caches/nix" "${XDG_CACHE_HOME:-$home/.cache}/nix")
  fi
  if [[ -n ${RUNNER_TEMP:-} ]]; then
    usage_paths+=("$RUNNER_TEMP")
  fi

  printf 'darwin disk telemetry phase=%s\n' "$phase" >&2
  if ! df -Pk "${filesystem_paths[@]}" >&2; then
    printf 'darwin disk telemetry df unavailable\n' >&2
  fi
  for path in "${usage_paths[@]}"; do
    [[ -e $path ]] || continue
    if ! du -sk "$path" >&2; then
      printf 'darwin disk telemetry du unavailable for %s\n' "$path" >&2
    fi
  done
}

report_darwin_disk_telemetry before-builds
printf 'evaluating flake for %s\n' "$system"
nix flake check --no-build
printf 'building Claude for %s\n' "$system"
nix build ".#packages.$system.claude" --no-link --print-build-logs
report_darwin_disk_telemetry after-claude-build

normal_checks=$(nix eval --raw ".#checks.$system" --apply '
  checks:
    builtins.concatStringsSep "\n"
      (builtins.attrNames (builtins.removeAttrs checks [ "native-enforcement" ]))
')
if [[ -z $normal_checks ]]; then
  printf 'normal check enumeration returned no checks\n' >&2
  exit 1
fi
claude_startup_seen=0
while IFS= read -r check; do
  [[ -n $check ]] || continue
  if [[ $check == claude-startup ]]; then
    claude_startup_seen=1
  fi
done <<< "$normal_checks"

if [[ $claude_startup_seen -ne 1 ]]; then
  printf 'required non-native check claude-startup is missing for %s\n' "$system" >&2
  exit 1
fi

if [[ $system == *-darwin ]]; then
  printf 'checking Darwin derivation graph for forbidden /bin/ls impure dependencies\n'
  nix derivation show --recursive \
    ".#checks.$system.claude-startup" \
    ".#checks.$system.native-enforcement" \
    | python3 "$repo_root/scripts/check-derivation-impure-host-deps.py"
fi

printf 'building native runner for %s\n' "$system"
runner=$(nix build ".#checks.$system.native-enforcement" \
  --no-link --print-build-logs --print-out-paths)
store_dir=$(nix eval --impure --raw --expr builtins.storeDir)
if [[ $runner != "$store_dir"/* || $runner == *$'\n'* ]]; then
  printf 'native runner build returned an unexpected output: %q\n' "$runner" >&2
  exit 1
fi
if [[ ! -x $runner/bin/native-enforcement ]]; then
  printf 'native runner is not executable: %s\n' "$runner/bin/native-enforcement" >&2
  exit 1
fi
report_darwin_disk_telemetry before-native-runner-execution
printf 'executing native runner as the invoking host user\n'
"$runner/bin/native-enforcement"

while IFS= read -r check; do
  [[ -n $check ]] || continue
  printf 'building non-native check %s for %s\n' "$check" "$system"
  nix build ".#checks.$system.$check" --no-link --print-build-logs
done <<< "$normal_checks"
