{ inputs, pkgs }:

let
  fakes = import ./fakes.nix { inherit inputs pkgs; };
  inheritedSandbox = fakes.mkSandbox { };
  inside = "/tmp/den-task12-startup/worktree/.den-claude";
  outside = "/tmp/den-task12-startup-outside";
  insideExplicit = fakes.mkSandbox { configDir = inside; };
  outsideExplicit = fakes.mkSandbox { configDir = outside; };
  overlapInside = fakes.mkSandbox { configDir = "/tmp/den-task12-startup/home/.claude/child"; };
  overlapOutside = fakes.mkSandbox { configDir = "/tmp/den-task12-startup/home"; };
  symlinkInside = fakes.mkSandbox { configDir = "/tmp/den-task12-startup/worktree/link-state"; };
  symlinkOutside = fakes.mkSandbox { configDir = "/tmp/den-task12-startup-outside-link"; };
in
pkgs.runCommand "claude-startup"
  {
    nativeBuildInputs = [ pkgs.jq pkgs.gitMinimal pkgs.util-linux ];
  }
  ''
    set -eu
    namespaceTmp="$TMPDIR/namespace-tmp"
    root=/tmp/den-task12-startup
    rootHost="$namespaceTmp/den-task12-startup"
    outside=/tmp/den-task12-startup-outside
    outsideHost="$namespaceTmp/den-task12-startup-outside"
    rm -rf "$namespaceTmp"
    mkdir -m 1777 "$namespaceTmp"
    mkdir -m 0755 "$namespaceTmp/etc"
    for file in passwd group hosts nsswitch.conf services protocols; do
      if test -e "/etc/$file"; then cp "/etc/$file" "$namespaceTmp/etc/$file"; else : > "$namespaceTmp/etc/$file"; fi
    done
    printf 'nameserver 127.0.0.1\n' > "$namespaceTmp/etc/resolv.conf"
    mkdir -m 0700 -p "$rootHost/home" "$rootHost/worktree"
    printf '.den-claude/\n' > "$rootHost/worktree/.gitignore"
    printf fixture-ca > "$rootHost/ca.pem"
    chmod 0400 "$rootHost/ca.pem"
    token=rw1_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
    uid=$(${pkgs.coreutils}/bin/id -u)

    cd "$rootHost/worktree"
    git init -q
    git add .gitignore

    namespace_run() {
      ${pkgs.util-linux}/bin/unshare -Ur -m ${pkgs.bash}/bin/bash -c '
        ${pkgs.util-linux}/bin/mount --bind "$1" /tmp
        ${pkgs.util-linux}/bin/mount --bind "$1/etc" /etc
        cd /tmp/den-task12-startup/worktree
        shift
        exec "$@"
      ' den-task12 "$namespaceTmp" "$@"
    }

    # Default startup begins with an empty home and only the three fake broker
    # inputs (plus HOME and deterministic observation controls).
    ${pkgs.coreutils}/bin/env -i \
      HOME="$root/home" \
      REPOWOLF_ENDPOINT=https://broker.example.test/ \
      REPOWOLF_TOKEN="$token" \
      REPOWOLF_CA_FILE="$root/ca.pem" \
      DEN_FAKE_STATE_MODE=default \
      DEN_FAKE_POLICY_COPY="$root/default-policy.json" \
      ${pkgs.util-linux}/bin/unshare -Ur -m ${pkgs.bash}/bin/bash -c '
        ${pkgs.util-linux}/bin/mount --bind "$1" /tmp
        ${pkgs.util-linux}/bin/mount --bind "$1/etc" /etc
        cd /tmp/den-task12-startup/worktree
        exec ${inheritedSandbox}/bin/claude
      ' den-task12 "$namespaceTmp"
    test -f "$rootHost/home/.claude/fake-state"
    test -f "$rootHost/home/.claude.json"
    test -f "$rootHost/home/.config/claude/fake-state"
    jq -e --arg home "$root/home" '
      (.filesystem.allowWrite | index($home + "/.claude")) != null and
      (.filesystem.allowWrite | index($home + "/.claude.json")) != null and
      (.filesystem.allowWrite | index($home + "/.config/claude")) != null
    ' "$rootHost/default-policy.json"
    rm -rf "$rootHost/home/.claude" "$rootHost/home/.claude.json" "$rootHost/home/.config"

    seed_resources() {
      selectedHost=$1
      mkdir -p "$selectedHost/skills/user-skill" "$selectedHost/plugins/user-plugin"
      printf user-skill > "$selectedHost/skills/user-skill/SKILL.md"
      printf user-plugin > "$selectedHost/plugins/user-plugin/plugin.json"
      printf '{"hooks":{"PostToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"true"}]}]}}\n' > "$selectedHost/settings.json"
      printf '{"mcpServers":{"user":{"command":"offline-fixture"}}}\n' > "$selectedHost/mcp.json"
      (cd "$selectedHost" && sha256sum \
        skills/user-skill/SKILL.md plugins/user-plugin/plugin.json settings.json mcp.json) \
        > "$rootHost/resources.before"
    }

    assert_resources_unchanged() {
      selectedHost=$1
      (cd "$selectedHost" && sha256sum \
        skills/user-skill/SKILL.md plugins/user-plugin/plugin.json settings.json mcp.json) \
        > "$rootHost/resources.after"
      cmp "$rootHost/resources.before" "$rootHost/resources.after"
    }

    run_custom() {
      wrapper=$1
      selection=$2
      selection_kind=$3
      label=$4
      selectedHost="$namespaceTmp''${selection#/tmp}"
      rm -rf "$selectedHost"
      mkdir -p "$(dirname "$selectedHost")"
      if test "$selection_kind" = inherited; then
        export CLAUDE_CONFIG_DIR="$selection"
      else
        unset CLAUDE_CONFIG_DIR || true
      fi
      export HOME="$root/home"
      export REPOWOLF_ENDPOINT=https://broker.example.test/
      export REPOWOLF_TOKEN="$token"
      export REPOWOLF_CA_FILE="$root/ca.pem"
      export DEN_FAKE_STATE_MODE=custom
      export DEN_FAKE_POLICY_COPY="$root/$label-policy.json"
      namespace_run "$wrapper"
      test "$(stat -c %a "$selectedHost")" = 700
      test "$(stat -c %u "$selectedHost")" = "$uid"
      test -f "$selectedHost/fake-state"
      test ! -e "$rootHost/home/.claude"
      test ! -e "$rootHost/home/.claude.json"
      test ! -e "$rootHost/home/.config/claude"
      rm "$selectedHost/fake-state"

      seed_resources "$selectedHost"
      namespace_run "$wrapper" --plugin-dir "$selection/plugins/user-plugin" \
        --mcp-config "$selection/mcp.json" --strict-mcp-config
      assert_resources_unchanged "$selectedHost"
      test -f "$selectedHost/skills/user-skill/SKILL.md"
      test -f "$selectedHost/plugins/user-plugin/plugin.json"
      jq -e --arg selected "$selection" --arg home "$HOME" '
        (.filesystem.allowWrite | index($selected)) != null and
        (.filesystem.denyWrite | index($home + "/.claude")) != null and
        (.filesystem.denyWrite | index($home + "/.claude.json")) != null and
        (.filesystem.denyWrite | index($home + "/.config/claude")) != null
      ' "$rootHost/$label-policy.json"
      if grep -Fq "$token" "$rootHost/$label-policy.json"; then
        echo 'startup policy disclosed fake token' >&2
        exit 1
      fi
    }

    run_custom ${insideExplicit}/bin/claude ${inside} explicit inside-explicit
    git check-ignore -q "$rootHost/worktree/.den-claude"
    run_custom ${outsideExplicit}/bin/claude ${outside} explicit outside-explicit
    run_custom ${inheritedSandbox}/bin/claude ${inside} inherited inside-inherited
    git check-ignore -q "$rootHost/worktree/.den-claude"
    run_custom ${inheritedSandbox}/bin/claude ${outside} inherited outside-inherited
    unset CLAUDE_CONFIG_DIR DEN_FAKE_STATE_MODE DEN_FAKE_POLICY_COPY

    expect_rejected() {
      if namespace_run "$1" > "$rootHost/rejected.out" 2> "$rootHost/rejected.err"; then
        echo 'startup accepted overlap or symbolic final component' >&2
        exit 1
      fi
      test ! -s "$rootHost/rejected.out"
      if grep -Fq "$token" "$rootHost/rejected.err"; then
        echo 'startup rejection disclosed fake token' >&2
        exit 1
      fi
    }

    # Canonical overlap and final-symlink rejection are repeated for both the
    # in-worktree and outside layouts, through explicit and inherited selection.
    mkdir -p "$rootHost/home/.claude" "$rootHost/symlink-target"
    chmod 0700 "$rootHost/home/.claude" "$rootHost/symlink-target" "$rootHost/worktree"
    ln -s "$root/symlink-target" "$rootHost/worktree/link-state"
    ln -s "$root/symlink-target" "$namespaceTmp/den-task12-startup-outside-link"
    expect_rejected ${overlapInside}/bin/claude
    expect_rejected ${overlapOutside}/bin/claude
    expect_rejected ${symlinkInside}/bin/claude
    expect_rejected ${symlinkOutside}/bin/claude

    export CLAUDE_CONFIG_DIR="$root/home/.claude/child"
    expect_rejected ${inheritedSandbox}/bin/claude
    export CLAUDE_CONFIG_DIR="$root/home"
    expect_rejected ${inheritedSandbox}/bin/claude
    export CLAUDE_CONFIG_DIR="$root/worktree/link-state"
    expect_rejected ${inheritedSandbox}/bin/claude
    export CLAUDE_CONFIG_DIR=/tmp/den-task12-startup-outside-link
    expect_rejected ${inheritedSandbox}/bin/claude

    # All executable boundaries are store fakes; no provider, Git host, daemon,
    # or network command is available or contacted by this startup check.
    test ! -e ${fakes.fakeRepoWolfClient}/bin/repowolf
    touch "$out"
  ''
