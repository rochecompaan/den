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
printf 'READY\n'
cat >/dev/null
exit "$status"
HELPER
chmod +x "$root/helper"

start_resolver_helper "$BASH" "$root/helper" 0
stop_resolver_helper

start_resolver_helper "$BASH" "$root/helper" 23
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

cat > "$root/not-ready" <<'HELPER'
printf 'NOT_READY\n'
exit 0
HELPER
chmod +x "$root/not-ready"
if start_resolver_helper "$BASH" "$root/not-ready"; then
  printf 'invalid helper readiness unexpectedly succeeded\n' >&2
  exit 1
fi
[[ -z $resolver_helper_pid && -z $resolver_helper_input && -z $resolver_helper_output ]]

printf 'resolver lifecycle tests passed\n'
