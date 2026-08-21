{ inputs, pkgs }:

let
  fakeFence = pkgs.writeShellScriptBin "fence" ''
    set -eu
    if test "''${1-}" = --linux-features; then
      printf 'Feature  Purpose  Status  Detail\n'
      printf 'Network namespace  direct network isolation  ok  available\n'
      exit 0
    fi

    policy=
    while test "$#" -gt 0 && test "$1" != --; do
      if test "$1" = --settings; then
        policy=$2
        shift 2
      else
        shift
      fi
    done
    test -n "$policy"
    test "$(''${STAT:-${pkgs.coreutils}/bin/stat} -c %a "$policy")" = 400
    test "$TMPDIR" = "$DEN_FENCE_TMPDIR"
    test "$TMPDIR" != "$(dirname "$policy")"
    if test -n "''${DEN_FAKE_POLICY_COPY-}"; then
      ${pkgs.coreutils}/bin/cp "$policy" "$DEN_FAKE_POLICY_COPY"
    fi
    if test -n "''${DEN_FAKE_FENCE_LOG-}"; then
      {
        printf 'policy-mode=400\n'
        printf 'separate-scratch=yes\n'
        printf 'tmpdir-controlled=yes\n'
      } > "$DEN_FAKE_FENCE_LOG"
    fi
    test "''${1-}" = --
    shift
    exec "$@"
  '';

  fakeClaude = pkgs.writeShellScriptBin "claude" ''
    set -eu
    if test -n "''${DEN_FAKE_AGENT_LOG-}"; then
      : > "$DEN_FAKE_AGENT_LOG"
      index=0
      for argument in "$@"; do
        printf 'arg[%s]=<%s>\n' "$index" "$argument" >> "$DEN_FAKE_AGENT_LOG"
        index=$((index + 1))
      done
      {
        printf 'path=%s\n' "$PATH"
        printf 'git-ssh=%s\n' "$GIT_SSH_COMMAND"
        printf 'git-count=%s\n' "$GIT_CONFIG_COUNT"
        printf 'endpoint-present=yes\n'
        printf 'token-present=yes\n'
        printf 'ca-present=yes\n'
        printf 'tmpdir-controlled=yes\n'
      } >> "$DEN_FAKE_AGENT_LOG"
    fi

    test -n "$REPOWOLF_ENDPOINT"
    test -n "$REPOWOLF_TOKEN"
    test -n "$REPOWOLF_CA_FILE"
    test "$TMPDIR" = "$DEN_FENCE_TMPDIR"
    test "$GIT_TERMINAL_PROMPT" = 0
    test "$GIT_CONFIG_COUNT" = 3
    test "$GIT_CONFIG_KEY_0" = url.git@github.com:.insteadOf
    test "$GIT_CONFIG_VALUE_0" = https://github.com/
    test "$GIT_CONFIG_KEY_1" = credential.helper
    test -z "$GIT_CONFIG_VALUE_1"
    test "$GIT_CONFIG_KEY_2" = core.sshCommand
    test "$GIT_CONFIG_VALUE_2" = "$GIT_SSH_COMMAND"
    for name in GH_TOKEN GITHUB_TOKEN GH_ENTERPRISE_TOKEN GITHUB_ENTERPRISE_TOKEN \
      SSH_AUTH_SOCK GIT_ASKPASS SSH_ASKPASS GIT_SSH GIT_CONFIG_GLOBAL \
      GIT_CONFIG_SYSTEM GIT_CONFIG_PARAMETERS; do
      if test -n "''${!name-}"; then exit 90; fi
    done
    if test -n "''${DEN_FAKE_EXPECT_AUTH-}"; then
      test "''${CLAUDE_CODE_OAUTH_TOKEN-}" = "$DEN_FAKE_EXPECT_AUTH"
    fi

    case "''${DEN_FAKE_STATE_MODE-}" in
      default)
        mkdir -p "$HOME/.claude" "$HOME/.config/claude"
        printf state > "$HOME/.claude/fake-state"
        printf state > "$HOME/.claude.json"
        printf state > "$HOME/.config/claude/fake-state"
        ;;
      custom)
        test -n "$CLAUDE_CONFIG_DIR"
        mkdir -p "$CLAUDE_CONFIG_DIR"
        printf state > "$CLAUDE_CONFIG_DIR/fake-state"
        ;;
    esac

    if test -n "''${DEN_FAKE_REPOWOLF_LOG-}"; then
      "$GIT_SSH_COMMAND" clone github.com git-upload-pack >/dev/null
      "$GIT_SSH_COMMAND" fetch github.com git-upload-pack >/dev/null
      "$GIT_SSH_COMMAND" push github.com git-receive-pack >/dev/null
      gh repo view >/dev/null
    fi
    case "''${DEN_FAKE_PROCESS_MODE-}" in
      stdio)
        IFS= read -r line
        printf 'stdout:%s\n' "$line"
        printf 'stderr:%s\n' "$line" >&2
        ;;
      pty)
        test -t 0 && test -t 1
        printf 'pty-ok\n'
        ;;
      self-signal)
        kill -TERM "$$"
        sleep 1
        ;;
      wait-signal)
        trap 'printf TERM > "$DEN_FAKE_SIGNAL_LOG"; exit 42' TERM
        printf '%s' "$$" > "$DEN_FAKE_PROCESS_PID_FILE"
        printf ready > "$DEN_FAKE_READY_FILE"
        while :; do sleep 1; done
        ;;
    esac
    exit "''${DEN_FAKE_EXIT_CODE-0}"
  '';

  fakeRepoWolfClient = pkgs.runCommand "fake-repowolf-client" { } ''
    mkdir -p "$out/bin"
    cat > "$out/bin/repowolf-client" <<'EOF'
    #!${pkgs.bash}/bin/bash
    set -eu
    program=$(basename "$0")
    case "$program" in
      gh|repowolf-git-ssh) ;;
      *) exit 91 ;;
    esac
    test -n "$REPOWOLF_ENDPOINT"
    test -n "$REPOWOLF_TOKEN"
    test -n "$REPOWOLF_CA_FILE"
    if test -n "''${DEN_FAKE_REPOWOLF_LOG-}"; then
      printf '%s' "$program" >> "$DEN_FAKE_REPOWOLF_LOG"
      for argument in "$@"; do printf ' <%s>' "$argument" >> "$DEN_FAKE_REPOWOLF_LOG"; done
      printf '\n' >> "$DEN_FAKE_REPOWOLF_LOG"
    fi
    exit 0
    EOF
    chmod 0555 "$out/bin/repowolf-client"
    ln -s repowolf-client "$out/bin/gh"
    ln -s repowolf-client "$out/bin/repowolf-git-ssh"
  '';

  launcher = import ../packages/den-launcher.nix { inherit pkgs; };
  mkAgentSandbox = import ../lib/mk-agent-sandbox.nix { inherit inputs pkgs; };
  dependencies = {
    fence = fakeFence;
    repoWolfClient = fakeRepoWolfClient;
    inherit launcher;
    git = pkgs.gitMinimal;
    bash = pkgs.bash;
    coreutils = pkgs.coreutils;
  } // pkgs.lib.optionalAttrs pkgs.stdenv.isLinux { acl = pkgs.acl; };
  mkSandbox = { configDir ? null }:
    mkAgentSandbox {
      inherit configDir dependencies;
      extraPkgs = [ ];
      docker = { };
      podman = { };
      adapter = {
        runtimePackages = [ fakeClaude ];
        closureOnlyPackages = [ ];
        agent = {
          name = "claude";
          executable = "${fakeClaude}/bin/claude";
          mandatoryArgs = [ "--dangerously-skip-permissions" ];
          reservedFlags = [ "--settings" "--permission-mode" "--dangerously-skip-permissions" ];
          configEnvironment = "CLAUDE_CONFIG_DIR";
        };
      };
    };
in
{
  inherit dependencies fakeClaude fakeFence fakeRepoWolfClient launcher mkSandbox;
}
