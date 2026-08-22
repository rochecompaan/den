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
mkdir "$root/bin"
printf '#!%s\n' "$BASH" > "$root/bin/nix"
cat >> "$root/bin/nix" <<'FAKE_NIX'
set -euo pipefail
printf '%s\n' "$*" >> "$DEN_FAKE_NIX_LOG"
if [[ $1 == eval && $* == *builtins.currentSystem* ]]; then
  printf '%s' "$DEN_FAKE_CURRENT_SYSTEM"
  exit
fi
if [[ $1 == config && $2 == show && $3 == allowed-impure-host-deps ]]; then
  printf '%s\n' "$DEN_FAKE_ALLOWED_IMPURE_HOST_DEPS"
  exit
fi
if [[ $1 == eval && $* == *--apply* ]]; then
  exit "$DEN_FAKE_APPLY_STATUS"
fi
if [[ $1 == build && $* == *native-enforcement* ]]; then
  touch "$DEN_FAKE_NATIVE_RAN"
fi
FAKE_NIX
chmod +x "$root/bin/nix"

export DEN_FAKE_NIX_LOG=$root/nix.log
export DEN_FAKE_NATIVE_RAN=$root/native-ran

run_driver() {
  local label=$1
  local system=$2
  export DEN_FAKE_CURRENT_SYSTEM=$system
  export DEN_FAKE_ALLOWED_IMPURE_HOST_DEPS=$3
  export DEN_FAKE_APPLY_STATUS=$4
  : > "$DEN_FAKE_NIX_LOG"
  rm -f "$DEN_FAKE_NATIVE_RAN"
  set +e
  PATH="$root/bin:$PATH" "$BASH" "$driver" "$system" \
    > "$root/$label.stdout" 2> "$root/$label.stderr"
  status=$?
  set -e
}

run_driver linux-enumeration x86_64-linux '' 41
if [[ $status -eq 0 ]]; then
  printf 'driver accepted forced normal-check enumeration failure\n' >&2
  exit 1
fi
if [[ -e $DEN_FAKE_NATIVE_RAN ]]; then
  printf 'driver ran native check after normal-check enumeration failure\n' >&2
  exit 1
fi
grep -Fq 'packages.x86_64-linux.claude' "$DEN_FAKE_NIX_LOG"
grep -Fq -- '--apply' "$DEN_FAKE_NIX_LOG"
! grep -Fq 'checks.x86_64-linux.native-enforcement' "$DEN_FAKE_NIX_LOG"
! grep -Fq 'config show allowed-impure-host-deps' "$DEN_FAKE_NIX_LOG"

for policy_case in missing broadened; do
  policy=''
  if [[ $policy_case == broadened ]]; then
    policy='/bin/ls /usr/bin/id'
  fi
  run_driver "darwin-$policy_case" aarch64-darwin "$policy" 41
  if [[ $status -eq 0 ]]; then
    printf 'driver accepted %s Darwin impure host dependency policy\n' "$policy_case" >&2
    exit 1
  fi
  grep -Fq 'config show allowed-impure-host-deps' "$DEN_FAKE_NIX_LOG"
  grep -Fq 'Darwin native checks require allowed-impure-host-deps = /bin/ls exactly' \
    "$root/darwin-$policy_case.stderr"
  ! grep -Fq 'flake check' "$DEN_FAKE_NIX_LOG"
done

run_driver darwin-exact aarch64-darwin /bin/ls 41
if [[ $status -eq 0 ]]; then
  printf 'driver did not reach forced enumeration failure with exact Darwin policy\n' >&2
  exit 1
fi
grep -Fq 'config show allowed-impure-host-deps' "$DEN_FAKE_NIX_LOG"
grep -Fq 'flake check' "$DEN_FAKE_NIX_LOG"
grep -Fq 'packages.aarch64-darwin.claude' "$DEN_FAKE_NIX_LOG"
grep -Fq -- '--apply' "$DEN_FAKE_NIX_LOG"
! grep -Fq 'Darwin native checks require allowed-impure-host-deps' \
  "$root/darwin-exact.stderr"
