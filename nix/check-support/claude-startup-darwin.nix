{ inputs, pkgs }:

let
  fakes = import ./fakes.nix { inherit inputs pkgs; };
  inheritedSandbox = fakes.mkSandbox { };
  inside = "/private/tmp/den-task12-startup/worktree/.den-claude";
  outside = "/private/tmp/den-task12-startup-outside";
  insideExplicit = fakes.mkSandbox { configDir = inside; };
  outsideExplicit = fakes.mkSandbox { configDir = outside; };
  overlapInside = fakes.mkSandbox { configDir = "/private/tmp/den-task12-startup/home/.claude/child"; };
  overlapOutside = fakes.mkSandbox { configDir = "/private/tmp/den-task12-startup/home"; };
  symlinkInside = fakes.mkSandbox { configDir = "/private/tmp/den-task12-startup/worktree/link-state"; };
  symlinkOutside = fakes.mkSandbox { configDir = "/private/tmp/den-task12-startup-outside-link"; };
in
pkgs.runCommand "claude-startup"
  { nativeBuildInputs = [ pkgs.jq pkgs.gitMinimal ]; }
  ''
    set -eu
    root=/private/tmp/den-task12-startup
    outside=/private/tmp/den-task12-startup-outside
    rm -rf "$root" "$outside" /private/tmp/den-task12-startup-outside-link
    mkdir -m 0700 -p "$root/home" "$root/worktree"
    printf '.den-claude/\n' > "$root/worktree/.gitignore"
    printf fixture-ca > "$root/ca.pem"
    chmod 0400 "$root/ca.pem"
    token=rw1_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
    uid=$(${pkgs.coreutils}/bin/id -u)
    cd "$root/worktree"
    git init -q
    git add .gitignore
    run_native() { (cd "$root/worktree" && "$@"); }

    ${pkgs.coreutils}/bin/env -i \
      HOME="$root/home" \
      REPOWOLF_ENDPOINT=https://broker.example.test/ \
      REPOWOLF_TOKEN="$token" \
      REPOWOLF_CA_FILE="$root/ca.pem" \
      DEN_FAKE_STATE_MODE=default \
      DEN_FAKE_EXPECT_NO_PLUGIN_SEED=1 \
      DEN_FAKE_POLICY_COPY="$root/default-policy.json" \
      ${inheritedSandbox}/bin/claude
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
      selected=$1
      mkdir -p "$selected/skills/user-skill" "$selected/plugins/user-plugin"
      printf user-skill > "$selected/skills/user-skill/SKILL.md"
      printf user-plugin > "$selected/plugins/user-plugin/plugin.json"
      printf '{"hooks":{"PostToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"true"}]}]}}\n' > "$selected/settings.json"
      printf '{"mcpServers":{"user":{"command":"offline-fixture"}}}\n' > "$selected/mcp.json"
      (cd "$selected" && sha256sum skills/user-skill/SKILL.md plugins/user-plugin/plugin.json settings.json mcp.json) > "$root/resources.before"
    }

    run_custom() {
      wrapper=$1; selection=$2; kind=$3; label=$4
      rm -rf "$selection"
      if test "$kind" = inherited; then export CLAUDE_CONFIG_DIR="$selection"; else unset CLAUDE_CONFIG_DIR || true; fi
      export HOME="$root/home"
      export REPOWOLF_ENDPOINT=https://broker.example.test/
      export REPOWOLF_TOKEN="$token"
      export REPOWOLF_CA_FILE="$root/ca.pem"
      export DEN_FAKE_STATE_MODE=custom DEN_FAKE_EXPECT_UID="$uid"
      export DEN_FAKE_POLICY_COPY="$root/$label-policy.json"
      run_native "$wrapper"
      test "$(${pkgs.coreutils}/bin/stat -c %a "$selection")" = 700
      test "$(${pkgs.coreutils}/bin/stat -c %u "$selection")" = "$uid"
      test -f "$selection/fake-state"
      test ! -e "$HOME/.claude" && test ! -e "$HOME/.claude.json" && test ! -e "$HOME/.config/claude"
      rm "$selection/fake-state"
      seed_resources "$selection"
      run_native "$wrapper" --plugin-dir "$selection/plugins/user-plugin" --mcp-config "$selection/mcp.json" --strict-mcp-config
      (cd "$selection" && sha256sum skills/user-skill/SKILL.md plugins/user-plugin/plugin.json settings.json mcp.json) > "$root/resources.after"
      cmp "$root/resources.before" "$root/resources.after"
      jq -e --arg selected "$selection" --arg home "$HOME" '
        (.filesystem.allowWrite | index($selected)) != null and
        (.filesystem.denyWrite | index($home + "/.claude")) != null and
        (.filesystem.denyWrite | index($home + "/.claude.json")) != null and
        (.filesystem.denyWrite | index($home + "/.config/claude")) != null
      ' "$root/$label-policy.json"
    }

    run_custom ${insideExplicit}/bin/claude ${inside} explicit inside-explicit
    git check-ignore -q ${inside}
    run_custom ${outsideExplicit}/bin/claude ${outside} explicit outside-explicit
    run_custom ${inheritedSandbox}/bin/claude ${inside} inherited inside-inherited
    git check-ignore -q ${inside}
    run_custom ${inheritedSandbox}/bin/claude ${outside} inherited outside-inherited
    unset CLAUDE_CONFIG_DIR DEN_FAKE_STATE_MODE DEN_FAKE_EXPECT_UID DEN_FAKE_POLICY_COPY

    export DEN_FAKE_FENCE_MARKER="$root/fence.marker"
    export DEN_FAKE_AGENT_LOG="$root/agent.log"
    export DEN_FAKE_POLICY_COPY="$root/rejected-policy.json"
    expect_rejected() {
      rm -f "$root/fence.marker" "$root/agent.log" "$root/rejected-policy.json" "$root/rejected.out" "$root/rejected.err"
      if run_native "$1" > "$root/rejected.out" 2> "$root/rejected.err"; then exit 1; fi
      test ! -s "$root/rejected.out"
      test ! -e "$root/fence.marker" && test ! -e "$root/agent.log" && test ! -e "$root/rejected-policy.json"
      if grep -Fq "$token" "$root/rejected.err"; then exit 1; fi
    }

    mkdir -p "$root/home/.claude" "$root/symlink-target"
    chmod 0700 "$root/home/.claude" "$root/symlink-target" "$root/worktree"
    ln -s "$root/symlink-target" "$root/worktree/link-state"
    ln -s "$root/symlink-target" /private/tmp/den-task12-startup-outside-link
    expect_rejected ${overlapInside}/bin/claude
    expect_rejected ${overlapOutside}/bin/claude
    expect_rejected ${symlinkInside}/bin/claude
    expect_rejected ${symlinkOutside}/bin/claude
    export CLAUDE_CONFIG_DIR="$root/home/.claude/child"; expect_rejected ${inheritedSandbox}/bin/claude
    export CLAUDE_CONFIG_DIR="$root/home"; expect_rejected ${inheritedSandbox}/bin/claude
    export CLAUDE_CONFIG_DIR="$root/worktree/link-state"; expect_rejected ${inheritedSandbox}/bin/claude
    export CLAUDE_CONFIG_DIR=/private/tmp/den-task12-startup-outside-link; expect_rejected ${inheritedSandbox}/bin/claude

    rm -rf "$root" "$outside" /private/tmp/den-task12-startup-outside-link
    touch "$out"
  ''
