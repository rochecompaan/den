#!/usr/bin/env bash
set -euo pipefail

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

nix flake check --no-build
nix build ".#packages.$system.claude" --no-link --print-build-logs

normal_checks=$(nix eval --raw ".#checks.$system" --apply '
  checks:
    builtins.concatStringsSep "\n"
      (builtins.attrNames (builtins.removeAttrs checks [ "native-enforcement" ]))
')
if [[ -z $normal_checks ]]; then
  printf 'normal check enumeration returned no checks\n' >&2
  exit 1
fi
while IFS= read -r check; do
  [[ -n $check ]] || continue
  nix build ".#checks.$system.$check" --no-link --print-build-logs
done <<< "$normal_checks"

runner=$(nix build ".#checks.$system.native-enforcement" \
  --no-link --print-build-logs --print-out-paths)
if [[ $runner != /nix/store/* || $runner == *$'\n'* ]]; then
  printf 'native runner build returned an unexpected output: %q\n' "$runner" >&2
  exit 1
fi
if [[ ! -x $runner/bin/native-enforcement ]]; then
  printf 'native runner is not executable: %s\n' "$runner/bin/native-enforcement" >&2
  exit 1
fi
"$runner/bin/native-enforcement"
