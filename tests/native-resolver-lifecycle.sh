#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  printf 'usage: %s <lifecycle-library>\n' "${0##*/}" >&2
  exit 2
fi
# shellcheck source=/dev/null
source "$1"

root=$(mktemp -d)
cleanup() {
  rm -rf "$root"
}
trap cleanup EXIT

cat > "$root/helper" <<'HELPER'
set -euo pipefail
status=${1:?status required}
marker=${2:?marker required}
printf 'READY\n'
cat >/dev/null
printf 'cleaned\n' >> "$marker"
exit "$status"
HELPER
cat > "$root/not-ready" <<'HELPER'
printf 'NOT_READY\n'
exit 0
HELPER

injected_point=
injected_signal=
injected_once=false
resolver_lifecycle_test_hook() {
  local point=${1:?transition point required}
  if [[ $point == "$injected_point" ]] && ! $injected_once; then
    injected_once=true
    kill -s "$injected_signal" "$BASHPID"
  fi
}

expected_signal_status() {
  case "$1" in
    HUP) printf '129\n' ;;
    INT) printf '130\n' ;;
    TERM) printf '143\n' ;;
    *) return 2 ;;
  esac
}

run_start_signal_case() {
  local signal_name=$1
  local point=$2
  local marker=$root/start-$signal_name-$point
  injected_signal=$signal_name
  injected_point=$point
  injected_once=false
  local status=0
  if start_resolver_helper "$BASH" "$root/helper" 0 "$marker"; then
    printf 'start transition signal unexpectedly succeeded: %s %s\n' "$signal_name" "$point" >&2
    exit 1
  else
    status=$?
  fi
  [[ $status -eq $(expected_signal_status "$signal_name") ]]
  [[ -z $resolver_helper_pid && -z $resolver_helper_input && -z $resolver_helper_output ]]
  if [[ $point == start_before_coproc ]]; then
    [[ ! -e $marker ]]
  else
    grep -qx cleaned "$marker"
  fi
}

run_stop_signal_case() {
  local signal_name=$1
  local point=$2
  local marker=$root/stop-$signal_name-$point
  injected_point=
  injected_signal=
  injected_once=false
  start_resolver_helper "$BASH" "$root/helper" 0 "$marker"
  injected_signal=$signal_name
  injected_point=$point
  injected_once=false
  local status=0
  if stop_resolver_helper; then
    printf 'stop transition signal unexpectedly succeeded: %s %s\n' "$signal_name" "$point" >&2
    exit 1
  else
    status=$?
  fi
  [[ $status -eq $(expected_signal_status "$signal_name") ]]
  grep -qx cleaned "$marker"
  [[ -z $resolver_helper_pid && -z $resolver_helper_input && -z $resolver_helper_output ]]
}

injected_point=
start_resolver_helper "$BASH" "$root/helper" 0 "$root/success"
stop_resolver_helper
grep -qx cleaned "$root/success"

start_resolver_helper "$BASH" "$root/helper" 23 "$root/helper-failure"
helper_status=0
if stop_resolver_helper; then
  printf 'failing helper cleanup unexpectedly succeeded\n' >&2
  exit 1
else
  helper_status=$?
fi
[[ $helper_status -eq 23 ]]

primary_status=0
if resolver_lifecycle_status 37 23; then
  printf 'primary failure was not preserved\n' >&2
  exit 1
else
  primary_status=$?
fi
[[ $primary_status -eq 37 ]]

cleanup_status=0
if resolver_lifecycle_status 0 23; then
  printf 'helper cleanup failure did not fail success\n' >&2
  exit 1
else
  cleanup_status=$?
fi
[[ $cleanup_status -eq 1 ]]
resolver_lifecycle_status 0 0

for signal_status in 129 130 143; do
  propagated_status=0
  if resolver_lifecycle_status 0 "$signal_status"; then
    printf 'helper signal status unexpectedly succeeded: %s\n' "$signal_status" >&2
    exit 1
  else
    propagated_status=$?
  fi
  [[ $propagated_status -eq $signal_status ]]
done

for signal_name in HUP INT TERM; do
  run_start_signal_case "$signal_name" start_after_coproc
  run_stop_signal_case "$signal_name" stop_after_input_close
done
run_start_signal_case HUP start_before_coproc
run_start_signal_case TERM start_after_state_record
run_start_signal_case INT start_after_readiness
run_start_signal_case TERM start_after_readiness_pipe_close
run_stop_signal_case HUP stop_before_close
run_stop_signal_case TERM stop_after_state_clear

injected_point=
if start_resolver_helper "$BASH" "$root/not-ready"; then
  printf 'invalid helper readiness unexpectedly succeeded\n' >&2
  exit 1
fi
[[ -z $resolver_helper_pid && -z $resolver_helper_input && -z $resolver_helper_output ]]

printf 'resolver lifecycle tests passed\n'
