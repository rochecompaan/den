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
darwin_resolver=
darwin_resolver_dir_created=false
cleanup() {
  if [[ -n $darwin_resolver ]]; then
    /usr/bin/sudo -n /bin/rm -f "$darwin_resolver" >/dev/null 2>&1 || true
  fi
  if $darwin_resolver_dir_created; then
    /usr/bin/sudo -n /bin/rmdir /etc/resolver >/dev/null 2>&1 || true
  fi
  rm -rf "$host_root"
}
trap cleanup EXIT HUP INT TERM

export DEN_NATIVE_HOST_ROOT=$host_root/fixtures
mkdir -m 700 "$DEN_NATIVE_HOST_ROOT"

if [[ $DEN_NATIVE_HOST_SYSTEM == *-darwin ]]; then
  export TMPDIR=${TMPDIR:-/tmp}
  export DEN_NATIVE_DNS_PORT=38415
  resolver_target=/etc/resolver/registry.npmjs.org
  if [[ -e $resolver_target ]]; then
    printf 'native registry resolver entry already exists\n' >&2
    exit 1
  fi
  darwin_resolver=$resolver_target
  /usr/bin/sudo -n /usr/bin/true
  if [[ ! -d /etc/resolver ]]; then
    /usr/bin/sudo -n /bin/mkdir /etc/resolver
    darwin_resolver_dir_created=true
  fi
  printf 'nameserver 127.0.0.1\nport %s\n' "$DEN_NATIVE_DNS_PORT" > "$host_root/registry-resolver"
  /usr/bin/sudo -n /usr/bin/install -m 0644 "$host_root/registry-resolver" "$resolver_target"
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
