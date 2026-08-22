{ inputs, pkgs }:

let
  fakes = import ./fakes.nix { inherit inputs pkgs; };
  aclDiagnosticProbe = pkgs.writeShellScriptBin "den-acl-diagnostic-probe" ''
    set -eu
    if test -z "''${DEN_CONFIGDIR_ACL_DIAGNOSTIC_LOG-}"; then
      exec /bin/ls "$@"
    fi

    stdout_file=$(${pkgs.coreutils}/bin/mktemp)
    stderr_file=$(${pkgs.coreutils}/bin/mktemp)
    id_stderr_file=$(${pkgs.coreutils}/bin/mktemp)
    cleanup() {
      ${pkgs.coreutils}/bin/rm -f "$stdout_file" "$stderr_file" "$id_stderr_file"
    }
    trap cleanup EXIT

    status=0
    /bin/ls "$@" > "$stdout_file" 2> "$stderr_file" || status=$?
    owner_name_status=0
    owner_name=$(${pkgs.coreutils}/bin/id -un 2> "$id_stderr_file") || owner_name_status=$?
    owner_uid_status=0
    owner_uid=$(${pkgs.coreutils}/bin/id -u 2>> "$id_stderr_file") || owner_uid_status=$?
    target="''${!#}"

    export DEN_ACL_PROBE_STATUS="$status"
    export DEN_ACL_PROBE_TARGET="$target"
    export DEN_ACL_PROBE_OWNER_NAME="$owner_name"
    export DEN_ACL_PROBE_OWNER_NAME_STATUS="$owner_name_status"
    export DEN_ACL_PROBE_OWNER_UID="$owner_uid"
    export DEN_ACL_PROBE_OWNER_UID_STATUS="$owner_uid_status"
    ${pkgs.python3}/bin/python3 - "$stdout_file" "$stderr_file" "$id_stderr_file" <<'PY'
    import os
    import re
    import sys

    stdout_path, stderr_path, id_stderr_path = sys.argv[1:]
    original = os.environ.get("DEN_CONFIGDIR_ACL_ORIGINAL_PATH", "")
    target = os.environ["DEN_ACL_PROBE_TARGET"]
    owner_name = os.environ["DEN_ACL_PROBE_OWNER_NAME"]

    def scrub(value):
        for path, replacement in sorted(
            ((original, "<original-path>"), (target, "<probe-path>")),
            key=lambda item: len(item[0]),
            reverse=True,
        ):
            if path:
                value = value.replace(path, replacement)
        value = re.sub(
            r"^([bcdlps-][^\s]*\s+\d+\s+)(\S+)(\s+)(\S+)",
            lambda match: match.group(1)
            + ("<invoking-user>" if owner_name and match.group(2) == owner_name else "<other-user>")
            + match.group(3)
            + "<group>",
            value,
            flags=re.MULTILINE,
        )
        value = re.sub(
            r"user:([^\s]+)",
            lambda match: "user:<invoking-user>"
            if owner_name and match.group(1) == owner_name
            else "user:<other-user>",
            value,
        )
        value = re.sub(r"group:([^\s]+)", "group:<group>", value)
        value = re.sub(r"/private/tmp/[^\s]+", "<private-tmp-path>", value)
        if owner_name:
            value = value.replace(owner_name, "<invoking-user>")
        return value

    target_kind = "directory-handle" if target.startswith("/dev/fd/") else "path"
    with open(os.environ["DEN_CONFIGDIR_ACL_DIAGNOSTIC_LOG"], "a", encoding="utf-8") as log:
        log.write(
            "acl-probe command=/bin/ls -lde target=<%s> exit=%s\n"
            % (target_kind, os.environ["DEN_ACL_PROBE_STATUS"])
        )
        log.write(
            "acl-probe id-u exit=%s value=%s; id-un exit=%s value=%s\n"
            % (
                os.environ["DEN_ACL_PROBE_OWNER_UID_STATUS"],
                "<resolved>" if os.environ["DEN_ACL_PROBE_OWNER_UID"] else "<unresolved>",
                os.environ["DEN_ACL_PROBE_OWNER_NAME_STATUS"],
                "<invoking-user>" if owner_name else "<unresolved>",
            )
        )
        for label, path in (
            ("stdout", stdout_path),
            ("stderr", stderr_path),
            ("id-stderr", id_stderr_path),
        ):
            with open(path, "r", encoding="utf-8", errors="replace") as source:
                content = scrub(source.read())
            log.write("acl-probe %s:\n%s" % (label, content or "<empty>\n"))

    PY

    ${pkgs.coreutils}/bin/cat "$stdout_file"
    ${pkgs.coreutils}/bin/cat "$stderr_file" >&2
    exit "$status"
  '';
  withACLDiagnostics = name: package:
    fakes.overrideManifest {
      inherit name package;
      filter = ''.aclProbe = ["${aclDiagnosticProbe}/bin/den-acl-diagnostic-probe", "-lde"]'';
    };
  inheritedSandbox = withACLDiagnostics "claude-startup-inherited" (fakes.mkSandbox { });
  inside = "/private/tmp/den-task12-startup/worktree/.den-claude";
  outside = "/private/tmp/den-task12-startup-outside";
  insideExplicit = withACLDiagnostics "claude-startup-inside-explicit"
    (fakes.mkSandbox { configDir = inside; });
  outsideExplicit = withACLDiagnostics "claude-startup-outside-explicit"
    (fakes.mkSandbox { configDir = outside; });
  overlapInside = withACLDiagnostics "claude-startup-overlap-inside"
    (fakes.mkSandbox { configDir = "/private/tmp/den-task12-startup/home/.claude/child"; });
  overlapOutside = withACLDiagnostics "claude-startup-overlap-outside"
    (fakes.mkSandbox { configDir = "/private/tmp/den-task12-startup/home"; });
  symlinkInside = withACLDiagnostics "claude-startup-symlink-inside"
    (fakes.mkSandbox { configDir = "/private/tmp/den-task12-startup/worktree/link-state"; });
  symlinkOutside = withACLDiagnostics "claude-startup-symlink-outside"
    (fakes.mkSandbox { configDir = "/private/tmp/den-task12-startup-outside-link"; });
in
pkgs.runCommand "claude-startup"
  {
    nativeBuildInputs = [ pkgs.jq pkgs.gitMinimal ];
    # The production Darwin ACL probe is an immutable host executable.
    __impureHostDeps = [ "/bin/ls" ];
  }
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
      export DEN_CONFIGDIR_ACL_DIAGNOSTIC_LOG="$root/acl-diagnostics.log"
      : > "$DEN_CONFIGDIR_ACL_DIAGNOSTIC_LOG"
      run_native "$wrapper" || {
        status=$?
        printf 'sanitized Darwin ACL diagnostics:\n' >&2
        cat "$DEN_CONFIGDIR_ACL_DIAGNOSTIC_LOG" >&2
        return "$status"
      }
      test "$(${pkgs.coreutils}/bin/stat -c %a "$selection")" = 700
      test "$(${pkgs.coreutils}/bin/stat -c %u "$selection")" = "$uid"
      test -f "$selection/fake-state"
      test ! -e "$HOME/.claude" && test ! -e "$HOME/.claude.json" && test ! -e "$HOME/.config/claude"
      rm "$selection/fake-state"
      seed_resources "$selection"
      : > "$DEN_CONFIGDIR_ACL_DIAGNOSTIC_LOG"
      run_native "$wrapper" --plugin-dir "$selection/plugins/user-plugin" --mcp-config "$selection/mcp.json" --strict-mcp-config || {
        status=$?
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

    run_custom ${insideExplicit}/bin/claude ${inside} explicit inside-explicit
    git check-ignore -q ${inside}
    run_custom ${outsideExplicit}/bin/claude ${outside} explicit outside-explicit
    run_custom ${inheritedSandbox}/bin/claude ${inside} inherited inside-inherited
    git check-ignore -q ${inside}
    run_custom ${inheritedSandbox}/bin/claude ${outside} inherited outside-inherited
    unset CLAUDE_CONFIG_DIR DEN_CONFIGDIR_ACL_DIAGNOSTIC_LOG DEN_FAKE_STATE_MODE DEN_FAKE_EXPECT_UID DEN_FAKE_POLICY_COPY

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
