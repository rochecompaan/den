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
cleanup() {
  rm -rf "$host_root"
}
trap cleanup EXIT HUP INT TERM

export DEN_NATIVE_HOST_ROOT=$host_root/fixtures
mkdir -m 700 "$DEN_NATIVE_HOST_ROOT"

if [[ $DEN_NATIVE_HOST_SYSTEM == *-darwin ]]; then
  export TMPDIR=${TMPDIR:-/tmp}
  "$DEN_NATIVE_TEST_BINARY" -test.count=1
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
"$DEN_NATIVE_UNSHARE" -Ur -m -n bash -c '
  set -euo pipefail
  "$DEN_NATIVE_IP" link set lo up
  "$DEN_NATIVE_MOUNT" --bind "$1" /etc/resolv.conf
  "$DEN_NATIVE_MOUNT" --bind "$2" /etc/nsswitch.conf
  "$DEN_NATIVE_MOUNT" --bind "$3" /tmp
  exec "$DEN_NATIVE_TEST_BINARY" -test.count=1
' den-native "$resolver" "$nsswitch" "$namespace_tmp"
