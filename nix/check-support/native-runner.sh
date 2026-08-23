# shellcheck shell=bash
set -euo pipefail

if [[ $# -ne 0 ]]; then
  printf 'usage: %s\n' "${0##*/}" >&2
  exit 2
fi

: "${DEN_NATIVE_TEST_BINARY:?packaged native test binary is required}"
: "${DEN_NATIVE_HOST_SYSTEM:?packaged host system is required}"
: "${DEN_NATIVE_SETTINGS_MERGE:?packaged Claude settings merge fixture is required}"

case "$DEN_NATIVE_HOST_SYSTEM" in
  *-linux)
    : "${DEN_NATIVE_ACL:?packaged setfacl is required}"
    : "${DEN_NATIVE_GETFACL:?packaged getfacl is required}"
    : "${DEN_NATIVE_UNSHARE:?packaged unshare is required}"
    : "${DEN_NATIVE_MOUNT:?packaged mount is required}"
    : "${DEN_NATIVE_IP:?packaged ip is required}"
    : "${DEN_NATIVE_BASH:?packaged Bash is required}"
    ;;
  *-darwin)
    : "${DEN_NATIVE_RESOLVER_HELPER:?packaged resolver helper is required}"
    : "${DEN_NATIVE_CLAUDE_STARTUP:?packaged Darwin Claude startup fixture is required}"
    : "${DEN_NATIVE_SANDBOX_EXEC:?Darwin sandbox-exec path is required}"
    test -x "$DEN_NATIVE_CLAUDE_STARTUP"
    test -x "$DEN_NATIVE_SANDBOX_EXEC"
    ;;
  *)
    printf 'unsupported native runner system: %s\n' "$DEN_NATIVE_HOST_SYSTEM" >&2
    exit 2
    ;;
esac

printf 'executing Claude settings merge fixture as the invoking host user\n'
settings_merge_output=$("$DEN_NATIVE_SETTINGS_MERGE")
if [[ $settings_merge_output != "fixture complete" ]]; then
  printf 'Claude settings merge fixture returned unexpected output: %q\n' \
    "$settings_merge_output" >&2
  exit 1
fi
printf '%s\n' "$settings_merge_output"
export DEN_NATIVE_SETTINGS_MERGE_COMPLETED=1

host_base=${XDG_RUNTIME_DIR:-${HOME:?HOME is required}/.cache}
host_parent=$host_base/den-native-enforcement
mkdir -p "$host_parent"
chmod 700 "$host_parent"
host_root=$(mktemp -d "$host_parent/run.XXXXXX")
cleanup() {
  primary_status=$?
  helper_status=0
  fixture_status=0
  final_status=0
  resolver_defer_signals
  trap - EXIT

  if resolver_stop_helper_deferred; then
    helper_status=0
  else
    helper_status=$?
  fi
  if rm -rf "$host_root"; then
    fixture_status=0
  else
    fixture_status=$?
  fi
  resolver_restore_signals
  # Defined by the packaged resolver lifecycle prelude.
  # shellcheck disable=SC2154
  deferred_status=$resolver_deferred_signal
  trap - HUP INT TERM
  if (( primary_status == 0 && deferred_status != 0 )); then
    primary_status=$deferred_status
  fi
  if resolver_lifecycle_status "$primary_status" "$helper_status"; then
    final_status=0
  else
    final_status=$?
  fi
  if (( final_status == 0 && fixture_status != 0 )); then
    final_status=1
  fi
  exit "$final_status"
}
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

export DEN_NATIVE_HOST_ROOT=$host_root/fixtures
mkdir -m 700 "$DEN_NATIVE_HOST_ROOT"

if [[ $DEN_NATIVE_HOST_SYSTEM == *-darwin ]]; then
  export TMPDIR=${TMPDIR:-/tmp}
  export DEN_NATIVE_DNS_PORT=38415
  printf 'executing Darwin Claude startup fixture as the invoking host user\n'
  "$DEN_NATIVE_CLAUDE_STARTUP"
  completion=$DEN_NATIVE_HOST_ROOT/claude-startup.complete
  if [[ ! -f $completion || $(<"$completion") != complete ]]; then
    printf 'Darwin Claude startup fixture did not produce its completion artifact\n' >&2
    exit 1
  fi
  start_resolver_helper /usr/bin/sudo -n "$DEN_NATIVE_RESOLVER_HELPER"

  test_status=0
  if "$DEN_NATIVE_TEST_BINARY" -test.count=1 -test.timeout=2m; then
    test_status=0
  else
    test_status=$?
  fi
  helper_status=0
  if stop_resolver_helper; then
    helper_status=0
  else
    helper_status=$?
  fi
  final_status=0
  if resolver_lifecycle_status "$test_status" "$helper_status"; then
    final_status=0
  else
    final_status=$?
  fi
  exit "$final_status"
fi

acl_fixture=$host_root/acl-state
mkdir -m 700 "$acl_fixture"
"$DEN_NATIVE_ACL" -m u:nobody:r-x "$acl_fixture"
export DEN_NATIVE_ACL_FIXTURE=$acl_fixture
printf 'native enforcement caller uid=%s: named u:nobody:r-x ACL fixture created\n' "$(id -u)"

resolver=$host_root/resolv.conf
nsswitch=$host_root/nsswitch.conf
namespace_tmp=$host_root/tmp
printf 'nameserver 127.0.0.1\noptions timeout:1 attempts:1\n' > "$resolver"
sed 's/^hosts:.*/hosts: files dns/' /etc/nsswitch.conf > "$nsswitch"
mkdir -m 1777 "$namespace_tmp"

# Expansion belongs to the namespaced Bash process.
# shellcheck disable=SC2016
"$DEN_NATIVE_UNSHARE" -Ur -m -n "$DEN_NATIVE_BASH" -c '
  set -euo pipefail
  "$DEN_NATIVE_IP" link set lo up
  "$DEN_NATIVE_MOUNT" --bind "$1" /etc/resolv.conf
  "$DEN_NATIVE_MOUNT" --bind "$2" /etc/nsswitch.conf
  "$DEN_NATIVE_MOUNT" --bind "$3" /tmp
  export TMPDIR=/tmp
  exec "$DEN_NATIVE_TEST_BINARY" -test.count=1 -test.timeout=2m
' den-native "$resolver" "$nsswitch" "$namespace_tmp"
