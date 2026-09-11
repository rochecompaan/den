#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 3 ]]; then
  printf 'usage: %s <pi-darwin-startup> <test-extension> <manifest-extension>\n' "${0##*/}" >&2
  exit 2
fi

startup_source=$1
security_extension=$2
manifest_extension=$3
root=$(mktemp -d)
cleanup() {
  rm -rf "$root"
}
trap cleanup EXIT HUP INT TERM

write_executable() {
  local path=$1
  shift
  printf '#!%s\nset -euo pipefail\n' "$BASH" > "$path"
  printf '%s\n' "$@" >> "$path"
  chmod +x "$path"
}

write_executable "$root/pass" 'exit 0'
# These variables must expand when the generated executable runs.
# shellcheck disable=SC2016
write_executable "$root/node" \
  'cat > "$DEN_PI_DARWIN_STARTUP_ROOT/assertions.report" <<'"'"'REPORT'"'"'' \
  'allowed-bash-after-no-change-helper' \
  'fail-closed:deny' \
  'fail-closed:rewrite' \
  'fail-closed:malformed' \
  'fail-closed:failed' \
  'native-user-bash-parity' \
  'user-and-project-hostile-extensions-loaded-through-real-scopes' \
  'hostile-user-project-extensions-cannot-replace-entrypoints' \
  'identity-change-fails-closed' \
  'helper-created-no-http-or-socks-listener' \
  'outer-fence-required-for-shell-entrypoints' \
  'REPORT' \
  'printf "no-listener\n" > "$DEN_PI_DARWIN_HELPER_LISTENER_REPORT"'

write_executable "$root/extension-mismatch" \
  "printf '%s\\n' 'Pi command security extension identity changed' >&2" \
  'exit 1'
write_executable "$root/policy-mismatch" \
  "printf '%s\\n' 'Pi command security input changed' >&2" \
  'exit 1'

printf '#!%s\n' "$BASH" > "$root/sandbox"
cat >> "$root/sandbox" <<'SANDBOX'
set -euo pipefail
if [[ ${DEN_PI_DARWIN_TEST_REQUIRE_C_LOCALE:-0} == 1 && ( ${LC_ALL:-} != C || ${LANG:-} != C ) ]]; then
  printf 'packaged launch did not use the C locale\n' >&2
fi
user_hostile=$PI_CODING_AGENT_DIR/extensions/user-hostile.ts
project_hostile=$PWD/.pi/extensions/project-hostile.ts
security_winner=$(jq -r .agent.securityAdapter.path "$DEN_NATIVE_PI_STARTUP_MANIFEST")
if [[ -f $user_hostile || -f $project_hostile ]]; then
  if [[ -f $user_hostile ]]; then
    printf 'Error: Failed to load extension "%s": Tool "bash" conflicts with %s\n' \
      "$user_hostile" "$security_winner" >&2
  fi
  if [[ -f $project_hostile ]]; then
    printf 'Error: Failed to load extension "%s": Tool "bash" conflicts with %s\n' \
      "$project_hostile" "$security_winner" >&2
  fi
  printf 'hostile-collision\n' >> "$DEN_PI_DARWIN_TEST_EVENTS"
  exit 1
fi

test -f "$PWD/.pi/extensions/project-probe.ts"
if [[ ${DEN_PI_DARWIN_TEST_FAIL_POSITIVE:-0} == 1 ]]; then
  printf 'forced positive launch failure\n' >&2
  exit 23
fi
printf 'started\n' > "$DEN_PI_DARWIN_PI_START_MARKER"
if [[ ${DEN_PI_DARWIN_EXPECT_OUTER_FENCE:-0} == 1 ]]; then
  printf 'denied\n' > "$DEN_PI_DARWIN_DIRECT_REPORT"
fi
printf 'positive-launch\n' >> "$DEN_PI_DARWIN_TEST_EVENTS"
SANDBOX
chmod +x "$root/sandbox"

printf 'user hostile\n' > "$root/user-hostile.ts"
printf 'project hostile\n' > "$root/project-hostile.ts"
printf 'project probe\n' > "$root/project-probe.ts"
mkdir "$root/package-root" "$root/host"
printf '{"agent":{"name":"pi","securityAdapter":{"kind":"pi-extension","arguments":["--extension","%s"],"path":"%s"}}}\n' \
  "$manifest_extension" "$manifest_extension" > "$root/manifest.json"

export DEN_NATIVE_HOST_ROOT=$root/host
export DEN_NATIVE_PI_STARTUP_PI=$root/pass
export DEN_NATIVE_PI_STARTUP_SANDBOX=$root/sandbox
export DEN_NATIVE_PI_STARTUP_PRESTART_EXTENSION_MISMATCH_SANDBOX=$root/extension-mismatch
export DEN_NATIVE_PI_STARTUP_PRESTART_POLICY_MISMATCH_SANDBOX=$root/policy-mismatch
export DEN_NATIVE_PI_STARTUP_MANIFEST=$root/manifest.json
export DEN_NATIVE_PI_STARTUP_LAUNCHER=$root/pass
export DEN_NATIVE_PI_STARTUP_FENCE=$root/pass
export DEN_NATIVE_PI_STARTUP_NODE=$root/node
export DEN_NATIVE_PI_STARTUP_PACKAGE_ROOT=$root/package-root
export DEN_NATIVE_PI_STARTUP_SECURITY_TEST_EXTENSION=$security_extension
export DEN_NATIVE_PI_STARTUP_HELPER=$root/pass
export DEN_NATIVE_PI_STARTUP_USER_REPLACEMENT_EXTENSION=$root/user-hostile.ts
export DEN_NATIVE_PI_STARTUP_PROJECT_REPLACEMENT_EXTENSION=$root/project-hostile.ts
export DEN_NATIVE_PI_STARTUP_PROJECT_PROBE_EXTENSION=$root/project-probe.ts
export DEN_PI_DARWIN_TEST_EVENTS=$root/events
export DEN_PI_DARWIN_TEST_REQUIRE_C_LOCALE=1

if ! "$BASH" "$startup_source"; then
  for output in hostile-collisions extension-version policy-version version assertions.report; do
    if [[ -f $DEN_NATIVE_HOST_ROOT/pi-darwin-startup/$output ]]; then
      printf '%s:\n' "$output" >&2
      cat "$DEN_NATIVE_HOST_ROOT/pi-darwin-startup/$output" >&2
    fi
  done
  printf 'Darwin Pi startup shell regression failed\n' >&2
  exit 1
fi
cmp -s <(printf 'complete\n') "$DEN_NATIVE_HOST_ROOT/pi-darwin-startup.complete"
cmp -s <(printf 'hostile-collision\npositive-launch\n') "$DEN_PI_DARWIN_TEST_EVENTS"

failure_output=$root/failure-output
if DEN_PI_DARWIN_TEST_FAIL_POSITIVE=1 "$BASH" "$startup_source" > "$failure_output" 2>&1; then
  printf 'forced positive launch failure unexpectedly succeeded\n' >&2
  exit 1
fi
if ! grep -Fqx 'Darwin Pi startup fixture failed during clean packaged launch' "$failure_output"; then
  printf 'missing Darwin Pi startup phase diagnostic\n' >&2
  exit 1
fi
grep -Fqx 'version:' "$failure_output"
grep -Fqx 'forced positive launch failure' "$failure_output"
printf 'Darwin Pi startup shell tests passed\n'
