#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  printf 'usage: %s <runtime-manifest-library>\n' "${0##*/}" >&2
  exit 2
fi
# shellcheck source=/dev/null
source "$1"

root=$(mktemp -d)
cleanup() {
  rm -rf "$root"
}
trap cleanup EXIT

base="$root/base.json"
inherited="$root/inherited.json"
invalid_inherited_base="$root/invalid-inherited-base.json"
explicit="$root/explicit.json"
mutated="$root/mutated.json"

cat > "$base" <<'JSON'
{
  "version": 1,
  "platform": "darwin",
  "explicitConfigDir": null,
  "aclProbe": ["/bin/ls", "-lde"],
  "agent": {"name": "claude"},
  "filesystem": {"sentinel": "unchanged"}
}
JSON

export DEN_CLAUDE_STARTUP_ACL_PROBE=/usr/bin/ls

den_adapt_claude_startup_manifest "$base" "$inherited" inherited ""
jq -e '.explicitConfigDir == null and .aclProbe == ["/usr/bin/ls", "-lde"] and
  has("explicitConfigDir") and has("aclProbe")' "$inherited" >/dev/null
den_validate_claude_startup_manifest "$base" "$inherited"

jq '.explicitConfigDir = "/private/etc/invalid"' "$base" > "$invalid_inherited_base"
if den_adapt_claude_startup_manifest "$invalid_inherited_base" \
  "$root/invalid-inherited.json" inherited ""; then
  printf 'inherited adaptation accepted a non-null base configuration directory\n' >&2
  exit 1
fi

den_adapt_claude_startup_manifest "$base" "$explicit" explicit /private/etc/claude
jq -e '.explicitConfigDir == "/private/etc/claude" and .aclProbe == ["/usr/bin/ls", "-lde"] and
  has("explicitConfigDir") and has("aclProbe")' "$explicit" >/dev/null
den_validate_claude_startup_manifest "$base" "$explicit"

jq '.filesystem.sentinel = "mutated"' "$explicit" > "$mutated"
if den_validate_claude_startup_manifest "$base" "$mutated"; then
  printf 'runtime manifest mutation unexpectedly validated\n' >&2
  exit 1
fi

if den_adapt_claude_startup_manifest "$base" "$root/unknown.json" unknown ""; then
  printf 'unknown adaptation mode unexpectedly succeeded\n' >&2
  exit 1
fi

if den_adapt_claude_startup_manifest "$base" "$root/relative.json" explicit relative/path; then
  printf 'relative explicit configuration directory unexpectedly succeeded\n' >&2
  exit 1
fi

printf 'runtime manifest adapter tests passed\n'
