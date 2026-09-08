# shellcheck shell=bash
set -euo pipefail

: "${DEN_NATIVE_HOST_ROOT:?native host root is required}"
: "${DEN_NATIVE_PI_STARTUP_PI:?Pi executable is required}"
: "${DEN_NATIVE_PI_STARTUP_SANDBOX:?Pi sandbox is required}"
: "${DEN_NATIVE_PI_STARTUP_MANIFEST:?Pi manifest is required}"
: "${DEN_NATIVE_PI_STARTUP_LAUNCHER:?Pi launcher is required}"
: "${DEN_NATIVE_PI_STARTUP_FENCE:?Fence executable is required}"

for path in "$DEN_NATIVE_PI_STARTUP_PI" "$DEN_NATIVE_PI_STARTUP_SANDBOX" \
  "$DEN_NATIVE_PI_STARTUP_LAUNCHER" "$DEN_NATIVE_PI_STARTUP_FENCE"; do
  case "$path" in
    /*) test -x "$path" ;;
    *) printf 'Darwin Pi startup input is not an absolute executable: %s\n' "$path" >&2; exit 1 ;;
  esac
done

fixture_root=$DEN_NATIVE_HOST_ROOT/pi-darwin-startup
rm -rf "$fixture_root"
mkdir -m 0700 -p "$fixture_root/home" "$fixture_root/invoking-home" \
  "$fixture_root/agent" "$fixture_root/sessions" "$fixture_root/worktree"
printf 'fixture CA\n' > "$fixture_root/ca.pem"
chmod 0400 "$fixture_root/ca.pem"

# The manifest owns the immutable security extension. Its path and the private
# policy are revalidated by den-pi-agent immediately before Pi starts.
jq -e '
  .agent.name == "pi" and
  .agent.securityAdapter.kind == "pi-extension" and
  .agent.securityAdapter.arguments == ["--extension", .agent.securityAdapter.path] and
  (.agent.securityAdapter.path | startswith("/nix/store/"))
' "$DEN_NATIVE_PI_STARTUP_MANIFEST" >/dev/null

(
  cd "$fixture_root/worktree"
  HOME="$fixture_root/home" \
  DEN_NATIVE_INVOKING_HOME="$fixture_root/invoking-home" \
  PI_CODING_AGENT_DIR="$fixture_root/agent" \
  PI_CODING_AGENT_SESSION_DIR="$fixture_root/sessions" \
  REPOWOLF_ENDPOINT=https://broker.example.test/ \
  REPOWOLF_TOKEN=rw1_AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE \
  REPOWOLF_CA_FILE="$fixture_root/ca.pem" \
  "$DEN_NATIVE_PI_STARTUP_SANDBOX" --version > "$fixture_root/version"
)
test "$(<"$fixture_root/version")" = 0.84.4
printf 'complete\n' > "$DEN_NATIVE_HOST_ROOT/pi-darwin-startup.complete"
