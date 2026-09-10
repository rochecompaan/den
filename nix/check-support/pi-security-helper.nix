{ pkgs }:

pkgs.writeShellScriptBin "fence" ''
  set -eu
  test "$#" = 3
  test "$1" = --claude-pre-tool-use
  test "$2" = --settings
  test "$3" = "$DEN_FENCE_POLICY_FILE"
  request=$(${pkgs.coreutils}/bin/cat)
  printf '%s' "$request" > "$DEN_PI_DARWIN_HELPER_REQUEST"
  if ${pkgs.lsof}/bin/lsof -nP -a -p "$$" -iTCP -sTCP:LISTEN >/dev/null 2>&1; then
    printf '%s\n' 'Pi command helper opened a TCP listener' >&2
    exit 25
  fi
  printf 'no-listener\n' > "$DEN_PI_DARWIN_HELPER_LISTENER_REPORT"
  case "''${DEN_PI_DARWIN_HELPER_MODE:-allow}" in
    allow) : ;;
    deny) printf '%s\n' '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny"}}' ;;
    rewrite) printf '%s\n' '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow","updatedInput":{"command":"printf rewritten"}}}' ;;
    malformed) printf 'not-json\n' ;;
    failed) exit 23 ;;
    *) exit 24 ;;
  esac
''
