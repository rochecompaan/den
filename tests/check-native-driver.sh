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
  printf 'x86_64-linux'
  exit
fi
if [[ $1 == eval && $* == *--apply* ]]; then
  exit 41
fi
if [[ $1 == build && $* == *native-enforcement* ]]; then
  touch "$DEN_FAKE_NATIVE_RAN"
fi
FAKE_NIX
chmod +x "$root/bin/nix"

export DEN_FAKE_NIX_LOG=$root/nix.log
export DEN_FAKE_NATIVE_RAN=$root/native-ran
set +e
PATH="$root/bin:$PATH" "$BASH" "$driver" x86_64-linux > "$root/stdout" 2> "$root/stderr"
status=$?
set -e
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
