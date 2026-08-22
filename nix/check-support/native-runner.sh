# shellcheck shell=bash
set -euo pipefail

if [[ $# -ne 0 ]]; then
  printf 'usage: %s\n' "${0##*/}" >&2
  exit 2
fi

: "${DEN_NATIVE_TEST_BINARY:?packaged native test binary is required}"
: "${DEN_NATIVE_HOST_SYSTEM:?packaged host system is required}"

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
    test -x /usr/bin/sandbox-exec
    ;;
  *)
    printf 'unsupported native runner system: %s\n' "$DEN_NATIVE_HOST_SYSTEM" >&2
    exit 2
    ;;
esac

host_base=${XDG_RUNTIME_DIR:-${HOME:?HOME is required}/.cache}
host_parent=$host_base/den-native-enforcement
mkdir -p "$host_parent"
chmod 700 "$host_parent"
host_root=$(mktemp -d "$host_parent/run.XXXXXX")
darwin_resolver_target=
darwin_resolver_staging=
darwin_resolver_staging_identity=
darwin_resolver_staging_owned=false
darwin_resolver_installed=false
darwin_resolver_dir_created=false
darwin_resolver_dir_identity=
deferred_signal=0
resolver_identity() {
  /usr/bin/stat -f '%d:%i' "$1"
}
defer_signals() {
  deferred_signal=0
  trap 'deferred_signal=129' HUP
  trap 'deferred_signal=130' INT
  trap 'deferred_signal=143' TERM
}
restore_signals() {
  trap 'exit 129' HUP
  trap 'exit 130' INT
  trap 'exit 143' TERM
  if (( deferred_signal != 0 )); then
    signal_status=$deferred_signal
    deferred_signal=0
    exit "$signal_status"
  fi
}
cleanup() {
  status=$?
  cleanup_failed=0
  trap - EXIT HUP INT TERM

  if $darwin_resolver_installed; then
    if ! $darwin_resolver_staging_owned || [[ ! -e $darwin_resolver_staging ]]; then
      printf 'native registry resolver identity witness disappeared before cleanup\n' >&2
      cleanup_failed=1
    elif [[ $(resolver_identity "$darwin_resolver_staging") != "$darwin_resolver_staging_identity" ]]; then
      printf 'native registry resolver identity witness was replaced before cleanup\n' >&2
      cleanup_failed=1
    elif [[ ! -e $darwin_resolver_target && ! -L $darwin_resolver_target ]]; then
      printf 'native registry resolver entry disappeared before cleanup\n' >&2
      cleanup_failed=1
    elif [[ ! $darwin_resolver_target -ef $darwin_resolver_staging ]]; then
      printf 'native registry resolver entry was replaced before cleanup\n' >&2
      cleanup_failed=1
    elif ! /usr/bin/sudo -n /bin/rm "$darwin_resolver_target"; then
      printf 'native registry resolver entry cleanup failed\n' >&2
      cleanup_failed=1
    elif [[ -e $darwin_resolver_target || -L $darwin_resolver_target ]]; then
      printf 'native registry resolver entry remains after cleanup\n' >&2
      cleanup_failed=1
    fi
  elif [[ -n $darwin_resolver_target && ( -e $darwin_resolver_target || -L $darwin_resolver_target ) ]]; then
    printf 'foreign native registry resolver entry detected; refusing to remove it\n' >&2
    cleanup_failed=1
  fi

  if $darwin_resolver_staging_owned; then
    if [[ ! -e $darwin_resolver_staging ]]; then
      printf 'owned native registry resolver staging file disappeared before cleanup\n' >&2
      cleanup_failed=1
    elif [[ $(resolver_identity "$darwin_resolver_staging") != "$darwin_resolver_staging_identity" ]]; then
      printf 'owned native registry resolver staging file was replaced; refusing to remove it\n' >&2
      cleanup_failed=1
    elif ! /usr/bin/sudo -n /bin/rm "$darwin_resolver_staging"; then
      printf 'native registry resolver staging cleanup failed\n' >&2
      cleanup_failed=1
    elif [[ -e $darwin_resolver_staging ]]; then
      printf 'native registry resolver staging remains after cleanup\n' >&2
      cleanup_failed=1
    fi
  fi

  if $darwin_resolver_dir_created; then
    if [[ ! -d /etc/resolver ]] || [[ $(resolver_identity /etc/resolver) != "$darwin_resolver_dir_identity" ]]; then
      printf 'owned native registry resolver directory was replaced before cleanup\n' >&2
      cleanup_failed=1
    elif ! /usr/bin/sudo -n /bin/rmdir /etc/resolver; then
      printf 'native registry resolver directory cleanup failed\n' >&2
      cleanup_failed=1
    elif [[ -d /etc/resolver ]]; then
      printf 'native registry resolver directory remains after cleanup\n' >&2
      cleanup_failed=1
    fi
  fi
  if ! rm -rf "$host_root"; then
    printf 'native enforcement fixture cleanup failed\n' >&2
    cleanup_failed=1
  fi
  if (( status == 0 && cleanup_failed != 0 )); then
    status=1
  fi
  exit "$status"
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
  darwin_resolver_target=/etc/resolver/registry.npmjs.org
  if [[ -e $darwin_resolver_target || -L $darwin_resolver_target ]]; then
    printf 'native registry resolver entry already exists\n' >&2
    exit 1
  fi
  /usr/bin/sudo -n /usr/bin/true
  if [[ ! -d /etc/resolver ]]; then
    directory_status=0
    defer_signals
    if /usr/bin/sudo -n /bin/mkdir /etc/resolver; then
      darwin_resolver_dir_created=true
      if darwin_resolver_dir_identity=$(resolver_identity /etc/resolver); then
        :
      else
        directory_status=$?
      fi
    else
      directory_status=$?
    fi
    restore_signals
    if (( directory_status != 0 )) || [[ -z $darwin_resolver_dir_identity ]]; then
      printf 'native registry resolver directory setup failed\n' >&2
      exit 1
    fi
  fi
  if [[ ! -d /etc/resolver ]]; then
    printf 'native registry resolver directory setup failed\n' >&2
    exit 1
  fi

  printf 'nameserver 127.0.0.1\nport %s\n' "$DEN_NATIVE_DNS_PORT" > "$host_root/registry-resolver"
  staging_candidate=
  staging_status=0
  defer_signals
  if staging_candidate=$(/usr/bin/sudo -n /usr/bin/mktemp /etc/resolver/.den-native-registry.XXXXXX); then
    case "$staging_candidate" in
      /etc/resolver/.den-native-registry.*)
        if [[ -f $staging_candidate ]]; then
          if staging_identity=$(resolver_identity "$staging_candidate") && [[ -n $staging_identity ]]; then
            darwin_resolver_staging=$staging_candidate
            darwin_resolver_staging_identity=$staging_identity
            darwin_resolver_staging_owned=true
          else
            staging_status=1
          fi
        else
          staging_status=1
        fi
        ;;
      *) staging_status=1 ;;
    esac
  else
    staging_status=$?
  fi
  restore_signals
  if (( staging_status != 0 )) || ! $darwin_resolver_staging_owned; then
    printf 'native registry resolver staging setup failed\n' >&2
    exit 1
  fi

  if [[ ! -f $darwin_resolver_staging ]] ||
    [[ $(resolver_identity "$darwin_resolver_staging") != "$darwin_resolver_staging_identity" ]]; then
    printf 'owned native registry resolver staging file changed before install\n' >&2
    exit 1
  fi
  /usr/bin/sudo -n /bin/cp "$host_root/registry-resolver" "$darwin_resolver_staging"
  /usr/bin/sudo -n /bin/chmod 0644 "$darwin_resolver_staging"
  if [[ ! -f $darwin_resolver_staging ]] ||
    [[ $(resolver_identity "$darwin_resolver_staging") != "$darwin_resolver_staging_identity" ]] ||
    ! /usr/bin/cmp -s "$host_root/registry-resolver" "$darwin_resolver_staging"; then
    printf 'native registry resolver staging content verification failed\n' >&2
    exit 1
  fi

  link_status=0
  defer_signals
  if /usr/bin/sudo -n /bin/ln "$darwin_resolver_staging" "$darwin_resolver_target"; then
    if [[ $darwin_resolver_target -ef $darwin_resolver_staging ]]; then
      darwin_resolver_installed=true
    else
      link_status=1
    fi
  else
    link_status=$?
  fi
  restore_signals
  if (( link_status != 0 )) || ! $darwin_resolver_installed; then
    printf 'native registry resolver entry appeared during setup\n' >&2
    exit 1
  fi
  "$DEN_NATIVE_TEST_BINARY" -test.count=1 -test.timeout=2m
  exit
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
  exec "$DEN_NATIVE_TEST_BINARY" -test.count=1 -test.timeout=2m
' den-native "$resolver" "$nsswitch" "$namespace_tmp"
