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
mkdir -p "$fixture_root"
chmod 0700 "$fixture_root"
mkdir -p "$root"
chmod 0700 "$root"
mkdir -m 0700 "$root/home" "$root/worktree" "$fixture_root/manifests"

inside=$root/worktree/.den-claude
overlap_inside=$root/home/.claude/child
overlap_outside=$root/home
symlink_inside=$root/worktree/link-state
symlink_outside=$outside_link

inherited_manifest=$fixture_root/manifests/inherited.json
inside_explicit_manifest=$fixture_root/manifests/inside-explicit.json
outside_explicit_manifest=$fixture_root/manifests/outside-explicit.json
overlap_inside_manifest=$fixture_root/manifests/overlap-inside.json
overlap_outside_manifest=$fixture_root/manifests/overlap-outside.json
symlink_inside_manifest=$fixture_root/manifests/symlink-inside.json
symlink_outside_manifest=$fixture_root/manifests/symlink-outside.json

den_adapt_claude_startup_manifest \
  "$DEN_CLAUDE_STARTUP_INHERITED_MANIFEST" \
  "$inherited_manifest" inherited ""
den_adapt_claude_startup_manifest \
  "$DEN_CLAUDE_STARTUP_EXPLICIT_MANIFEST" \
  "$inside_explicit_manifest" explicit "$inside"
den_adapt_claude_startup_manifest \
  "$DEN_CLAUDE_STARTUP_EXPLICIT_MANIFEST" \
  "$outside_explicit_manifest" explicit "$outside"
den_adapt_claude_startup_manifest \
  "$DEN_CLAUDE_STARTUP_EXPLICIT_MANIFEST" \
  "$overlap_inside_manifest" explicit "$overlap_inside"
den_adapt_claude_startup_manifest \
  "$DEN_CLAUDE_STARTUP_EXPLICIT_MANIFEST" \
  "$overlap_outside_manifest" explicit "$overlap_outside"
den_adapt_claude_startup_manifest \
  "$DEN_CLAUDE_STARTUP_EXPLICIT_MANIFEST" \
  "$symlink_inside_manifest" explicit "$symlink_inside"
den_adapt_claude_startup_manifest \
  "$DEN_CLAUDE_STARTUP_EXPLICIT_MANIFEST" \
  "$symlink_outside_manifest" explicit "$symlink_outside"

printf '.den-claude/\n' > "$root/worktree/.gitignore"
printf fixture-ca > "$root/ca.pem"
chmod 0400 "$root/ca.pem"
token='rw1_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA'
uid=$(id -u)
cd "$root/worktree"
git init -q
git add .gitignore

run_native() {
  local manifest=$1
  shift
  (
    cd "$root/worktree"
    exec "$DEN_CLAUDE_STARTUP_LAUNCHER" --manifest "$manifest" -- "$@"
  )
}

# Default startup begins with an empty home and only the three fake broker
# inputs (plus HOME and deterministic observation controls).
(
  cd "$root/worktree"
  exec env -i \
    HOME="$root/home" \
    REPOWOLF_ENDPOINT=https://broker.example.test/ \
    REPOWOLF_TOKEN="$token" \
    REPOWOLF_CA_FILE="$root/ca.pem" \
    DEN_FAKE_STATE_MODE=default \
    DEN_FAKE_EXPECT_NO_PLUGIN_SEED=1 \
    DEN_FAKE_POLICY_COPY="$root/default-policy.json" \
    "$DEN_CLAUDE_STARTUP_LAUNCHER" --manifest "$inherited_manifest" --
)
test -f "$root/home/.claude/fake-state"
test -f "$root/home/.claude.json"
test -f "$root/home/.config/claude/fake-state"
jq -e --arg home "$root/home" '
  (.filesystem.allowWrite | index($home + "/.claude")) != null and
  (.filesystem.allowWrite | index($home + "/.claude.json")) != null and
  (.filesystem.allowWrite | index($home + "/.config/claude")) != null
' "$root/default-policy.json"
rm -rf "$root/home/.claude" "$root/home/.claude.json" "$root/home/.config"

seed_resources() {
  local selected=$1
  mkdir -p "$selected/skills/user-skill" "$selected/plugins/user-plugin"
  printf user-skill > "$selected/skills/user-skill/SKILL.md"
  printf user-plugin > "$selected/plugins/user-plugin/plugin.json"
  printf '{"hooks":{"PostToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"true"}]}]}}\n' > "$selected/settings.json"
  printf '{"mcpServers":{"user":{"command":"offline-fixture"}}}\n' > "$selected/mcp.json"
  (cd "$selected" && sha256sum skills/user-skill/SKILL.md plugins/user-plugin/plugin.json settings.json mcp.json) > "$root/resources.before"
}

run_custom() {
  local manifest=$1 selection=$2 kind=$3 label=$4
  rm -rf "$selection"
  if test "$kind" = inherited; then
    export CLAUDE_CONFIG_DIR="$selection"
  else
    unset CLAUDE_CONFIG_DIR || true
  fi
  export HOME="$root/home"
  export REPOWOLF_ENDPOINT=https://broker.example.test/
  export REPOWOLF_TOKEN="$token"
  export REPOWOLF_CA_FILE="$root/ca.pem"
  export DEN_FAKE_STATE_MODE=custom DEN_FAKE_EXPECT_UID="$uid"
  export DEN_FAKE_POLICY_COPY="$root/$label-policy.json"
  export DEN_CONFIGDIR_ACL_DIAGNOSTIC_LOG="$root/acl-diagnostics.log"
  : > "$DEN_CONFIGDIR_ACL_DIAGNOSTIC_LOG"
  run_native "$manifest" || {
    local status=$?
    printf 'sanitized Darwin ACL diagnostics:\n' >&2
    cat "$DEN_CONFIGDIR_ACL_DIAGNOSTIC_LOG" >&2
    return "$status"
  }
  test "$(stat -c %a "$selection")" = 700
  test "$(stat -c %u "$selection")" = "$uid"
  test -f "$selection/fake-state"
  test ! -e "$HOME/.claude" && test ! -e "$HOME/.claude.json" && test ! -e "$HOME/.config/claude"
  rm "$selection/fake-state"
  seed_resources "$selection"
  : > "$DEN_CONFIGDIR_ACL_DIAGNOSTIC_LOG"
  run_native "$manifest" --plugin-dir "$selection/plugins/user-plugin" --mcp-config "$selection/mcp.json" --strict-mcp-config || {
    local status=$?
    printf 'sanitized Darwin ACL diagnostics:\n' >&2
    cat "$DEN_CONFIGDIR_ACL_DIAGNOSTIC_LOG" >&2
    return "$status"
  }
  (cd "$selection" && sha256sum skills/user-skill/SKILL.md plugins/user-plugin/plugin.json settings.json mcp.json) > "$root/resources.after"
  cmp "$root/resources.before" "$root/resources.after"
  accountHome=$(jq -r --arg home "$HOME" '
    .filesystem.denyRead[] | select(endswith("/.ssh/id_*") and (startswith($home) | not)) |
    sub("/.ssh/id_\\*$"; "")
  ' "$root/$label-policy.json")
  test -n "$accountHome"
  jq -e --arg selected "$selection" --arg home "$HOME" --arg accountHome "$accountHome" '
    (.filesystem.allowWrite | index($selected)) != null and
    (.filesystem.denyWrite | index($home + "/.claude")) != null and
    (.filesystem.denyWrite | index($home + "/.claude.json")) != null and
    (.filesystem.denyWrite | index($home + "/.config/claude")) != null and
    (.filesystem as $fs | ["/.ssh/id_*", "/.aws/**", "/.gitconfig"] | all(. as $suffix |
      ($fs.denyRead | index($home + $suffix)) != null and
      ($fs.denyWrite | index($home + $suffix)) != null and
      ($fs.denyRead | index($accountHome + $suffix)) != null and
      ($fs.denyWrite | index($accountHome + $suffix)) != null)) and
    (.filesystem.denyRead | all(startswith("~/") | not))
  ' "$root/$label-policy.json"
}

run_custom "$inside_explicit_manifest" "$inside" explicit inside-explicit
git check-ignore -q "$inside"
run_custom "$outside_explicit_manifest" "$outside" explicit outside-explicit
run_custom "$inherited_manifest" "$inside" inherited inside-inherited
git check-ignore -q "$inside"
run_custom "$inherited_manifest" "$outside" inherited outside-inherited
unset CLAUDE_CONFIG_DIR DEN_CONFIGDIR_ACL_DIAGNOSTIC_LOG DEN_FAKE_STATE_MODE DEN_FAKE_EXPECT_UID DEN_FAKE_POLICY_COPY

export DEN_FAKE_FENCE_MARKER="$root/fence.marker"
export DEN_FAKE_AGENT_LOG="$root/agent.log"
export DEN_FAKE_POLICY_COPY="$root/rejected-policy.json"
expect_rejected() {
  local manifest=$1
  rm -f "$root/fence.marker" "$root/agent.log" "$root/rejected-policy.json" "$root/rejected.out" "$root/rejected.err"
  if run_native "$manifest" > "$root/rejected.out" 2> "$root/rejected.err"; then
    exit 1
  fi
  test ! -s "$root/rejected.out"
  test ! -e "$root/fence.marker" && test ! -e "$root/agent.log" && test ! -e "$root/rejected-policy.json"
  if grep -Fq "$token" "$root/rejected.err"; then
    exit 1
  fi
}

mkdir -p "$root/home/.claude" "$root/symlink-target"
chmod 0700 "$root/home/.claude" "$root/symlink-target" "$root/worktree"
ln -s "$root/symlink-target" "$symlink_inside"
ln -s "$root/symlink-target" "$symlink_outside"
expect_rejected "$overlap_inside_manifest"
expect_rejected "$overlap_outside_manifest"
expect_rejected "$symlink_inside_manifest"
expect_rejected "$symlink_outside_manifest"
export CLAUDE_CONFIG_DIR="$overlap_inside"; expect_rejected "$inherited_manifest"
export CLAUDE_CONFIG_DIR="$overlap_outside"; expect_rejected "$inherited_manifest"
export CLAUDE_CONFIG_DIR="$symlink_inside"; expect_rejected "$inherited_manifest"
export CLAUDE_CONFIG_DIR="$symlink_outside"; expect_rejected "$inherited_manifest"

printf 'complete\n' > "$DEN_NATIVE_HOST_ROOT/claude-startup.complete"
