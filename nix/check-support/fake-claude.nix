{ pkgs }:

pkgs.writeShellScriptBin "claude" ''
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
  if test -n "''${DEN_FAKE_EXPECT_SIMPLE-}"; then
    test "''${CLAUDE_CODE_SIMPLE-}" = "$DEN_FAKE_EXPECT_SIMPLE"
  fi
  if test -n "''${DEN_FAKE_EXPECT_SIMPLE_SCRUB-}"; then
    test -z "''${CLAUDE_CODE_SIMPLE-}"
  fi
  if test -n "''${DEN_FAKE_EXPECT_NO_PLUGIN_SEED-}"; then
    test -z "''${CLAUDE_CODE_PLUGIN_SEED_DIR-}"
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
      test -d "$CLAUDE_CONFIG_DIR"
      test "$(${pkgs.coreutils}/bin/stat -c %a "$CLAUDE_CONFIG_DIR")" = 700
      if test -n "''${DEN_FAKE_EXPECT_UID-}"; then
        test "$(${pkgs.coreutils}/bin/stat -c %u "$CLAUDE_CONFIG_DIR")" = "$DEN_FAKE_EXPECT_UID"
      fi
      printf state > "$CLAUDE_CONFIG_DIR/fake-state"
      ;;
  esac

  if test -n "''${DEN_FAKE_GIT_ROOT-}"; then
    rm -rf "$DEN_FAKE_GIT_ROOT"
    mkdir -p "$DEN_FAKE_GIT_ROOT"
    git clone -q https://github.com/fixture/repo.git "$DEN_FAKE_GIT_ROOT/clone" >/dev/null 2>&1
    git -C "$DEN_FAKE_GIT_ROOT/clone" fetch -q origin >/dev/null 2>&1
    printf pushed > "$DEN_FAKE_GIT_ROOT/clone/pushed"
    git -C "$DEN_FAKE_GIT_ROOT/clone" add pushed
    git -C "$DEN_FAKE_GIT_ROOT/clone" -c user.name=fixture -c user.email=fixture@example.test commit -qm pushed
    git -C "$DEN_FAKE_GIT_ROOT/clone" push -q origin HEAD:refs/heads/pushed >/dev/null 2>&1
    gh repo view >/dev/null
  fi

  case "''${DEN_FAKE_PROCESS_MODE-}" in
    stdio)
      IFS= read -r line
      printf 'stdout:%s\n' "$line"
      printf 'stderr:%s\n' "$line" >&2
      ;;
    pty-wait)
      test -t 0 && test -t 1 && test -t 2
      trap 'printf INT >> "$DEN_FAKE_SIGNAL_LOG"; exit 41' INT
      trap 'printf TERM >> "$DEN_FAKE_SIGNAL_LOG"; exit 42' TERM
      trap 'printf HUP >> "$DEN_FAKE_SIGNAL_LOG"; exit 43' HUP
      trap 'printf QUIT >> "$DEN_FAKE_SIGNAL_LOG"; exit 44' QUIT
      trap 'printf WINCH >> "$DEN_FAKE_SIGNAL_LOG"' WINCH
      trap 'printf TSTP >> "$DEN_FAKE_SIGNAL_LOG"' TSTP
      trap 'printf CONT >> "$DEN_FAKE_SIGNAL_LOG"' CONT
      printf '%s' "$$" > "$DEN_FAKE_PROCESS_PID_FILE"
      printf '%s' "$PPID" > "$DEN_FAKE_PROCESS_PID_FILE.ppid"
      printf ready > "$DEN_FAKE_READY_FILE"
      while :; do sleep 1; done
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
''
