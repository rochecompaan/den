#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  printf 'usage: %s <native-runner>\n' "${0##*/}" >&2
  exit 2
fi

runner_source=$1
root=$(mktemp -d)
cleanup() {
  rm -rf "$root"
}
trap cleanup EXIT HUP INT TERM

printf '#!%s\n' "$BASH" > "$root/runner"
cat >> "$root/runner" <<'FAKE_LIFECYCLE'
set -euo pipefail
resolver_deferred_signal=0
resolver_defer_signals() { :; }
resolver_restore_signals() { :; }
resolver_stop_helper_deferred() { return 0; }
resolver_lifecycle_status() { return "$1"; }
start_resolver_helper() { printf 'resolver\n' >> "$DEN_FAKE_EVENT_LOG"; }
stop_resolver_helper() { return 0; }
FAKE_LIFECYCLE
cat "$runner_source" >> "$root/runner"
chmod +x "$root/runner"

printf '#!%s\n' "$BASH" > "$root/settings-merge"
cat >> "$root/settings-merge" <<'FAKE_SETTINGS'
set -euo pipefail
printf 'settings\n' >> "$DEN_FAKE_EVENT_LOG"
printf 'fixture complete\n'
FAKE_SETTINGS

printf '#!%s\n' "$BASH" > "$root/claude-startup"
cat >> "$root/claude-startup" <<'FAKE_STARTUP'
set -euo pipefail
printf 'startup\n' >> "$DEN_FAKE_EVENT_LOG"
startup_status=${DEN_FAKE_STARTUP_STATUS:-0}
if [[ $startup_status -ne 0 ]]; then
  exit "$startup_status"
fi
printf 'complete\n' > "$DEN_NATIVE_HOST_ROOT/claude-startup.complete"
FAKE_STARTUP

printf '#!%s\n' "$BASH" > "$root/native-tests"
cat >> "$root/native-tests" <<'FAKE_GO'
set -euo pipefail
[[ -f $DEN_NATIVE_HOST_ROOT/claude-startup.complete ]]
[[ $(<"$DEN_NATIVE_HOST_ROOT/claude-startup.complete") == complete ]]
printf 'go\n' >> "$DEN_FAKE_EVENT_LOG"
FAKE_GO

printf '#!%s\n' "$BASH" > "$root/resolver-helper"
cat >> "$root/resolver-helper" <<'FAKE_MARKER'
exit 0
FAKE_MARKER
cp "$root/resolver-helper" "$root/sandbox-exec"
chmod +x "$root/settings-merge" "$root/claude-startup" "$root/native-tests" \
  "$root/resolver-helper" "$root/sandbox-exec"

mkdir -p "$root/home" "$root/runtime/den-native-enforcement"
printf 'host-user data\n' > "$root/runtime/den-native-enforcement/host-user-marker"
export HOME=$root/home
export XDG_RUNTIME_DIR=$root/runtime
export DEN_NATIVE_HOST_SYSTEM=aarch64-darwin
export DEN_NATIVE_SETTINGS_MERGE=$root/settings-merge
export DEN_NATIVE_CLAUDE_STARTUP=$root/claude-startup
export DEN_NATIVE_TEST_BINARY=$root/native-tests
export DEN_NATIVE_RESOLVER_HELPER=$root/resolver-helper
export DEN_NATIVE_SANDBOX_EXEC=$root/sandbox-exec
export DEN_FAKE_EVENT_LOG=$root/events

run_runner() {
  local label=$1
  shift
  : > "$DEN_FAKE_EVENT_LOG"
  set +e
  "$@" "$root/runner" > "$root/$label.stdout" 2> "$root/$label.stderr"
  status=$?
  set -e
}

assert_runner_cleanup() {
  [[ $(<"$root/runtime/den-native-enforcement/host-user-marker") == 'host-user data' ]]
  if compgen -G "$root/runtime/den-native-enforcement/run.*" > /dev/null; then
    printf 'runner cleanup left a runner-owned host root\n' >&2
    exit 1
  fi
}

run_runner success env -u DEN_FAKE_STARTUP_STATUS
if [[ $status -ne 0 ]]; then
  cat "$root/success.stderr" >&2
  exit 1
fi
[[ $(<"$DEN_FAKE_EVENT_LOG") == $'settings\nstartup\nresolver\ngo' ]]
assert_runner_cleanup

run_runner startup-failure env DEN_FAKE_STARTUP_STATUS=19
[[ $status -eq 19 ]]
[[ $(<"$DEN_FAKE_EVENT_LOG") == $'settings\nstartup' ]]
assert_runner_cleanup

printf 'native runner tests passed\n'
