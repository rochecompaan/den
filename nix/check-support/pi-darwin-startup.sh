# shellcheck shell=bash
set -euo pipefail

: "${DEN_NATIVE_HOST_ROOT:?native host root is required}"
: "${DEN_NATIVE_PI_STARTUP_PI:?Pi executable is required}"
: "${DEN_NATIVE_PI_STARTUP_SANDBOX:?Pi sandbox is required}"
: "${DEN_NATIVE_PI_STARTUP_PRESTART_EXTENSION_MISMATCH_SANDBOX:?Pi pre-start extension mismatch sandbox is required}"
: "${DEN_NATIVE_PI_STARTUP_PRESTART_POLICY_MISMATCH_SANDBOX:?Pi pre-start policy mismatch sandbox is required}"
: "${DEN_NATIVE_PI_STARTUP_MANIFEST:?Pi manifest is required}"
: "${DEN_NATIVE_PI_STARTUP_LAUNCHER:?Pi launcher is required}"
: "${DEN_NATIVE_PI_STARTUP_FENCE:?Fence executable is required}"
: "${DEN_NATIVE_PI_STARTUP_NODE:?Pi Node executable is required}"
: "${DEN_NATIVE_PI_STARTUP_PACKAGE_ROOT:?Pi package root is required}"
: "${DEN_NATIVE_PI_STARTUP_SECURITY_TEST_EXTENSION:?security test extension is required}"
: "${DEN_NATIVE_PI_STARTUP_HELPER:?security helper is required}"
: "${DEN_NATIVE_PI_STARTUP_USER_REPLACEMENT_EXTENSION:?user hostile extension is required}"
: "${DEN_NATIVE_PI_STARTUP_PROJECT_REPLACEMENT_EXTENSION:?project hostile extension is required}"

for path in "$DEN_NATIVE_PI_STARTUP_PI" "$DEN_NATIVE_PI_STARTUP_SANDBOX" \
  "$DEN_NATIVE_PI_STARTUP_PRESTART_EXTENSION_MISMATCH_SANDBOX" \
  "$DEN_NATIVE_PI_STARTUP_PRESTART_POLICY_MISMATCH_SANDBOX" \
  "$DEN_NATIVE_PI_STARTUP_LAUNCHER" "$DEN_NATIVE_PI_STARTUP_FENCE" \
  "$DEN_NATIVE_PI_STARTUP_NODE" "$DEN_NATIVE_PI_STARTUP_HELPER"; do
  case "$path" in
    /*) test -x "$path" ;;
    *) printf 'Darwin Pi startup input is not an absolute executable: %s\n' "$path" >&2; exit 1 ;;
  esac
done

fixture_root=$DEN_NATIVE_HOST_ROOT/pi-darwin-startup
rm -rf "$fixture_root"
mkdir -m 0700 -p "$fixture_root/home" "$fixture_root/invoking-home" \
  "$fixture_root/agent/extensions" "$fixture_root/sessions" \
  "$fixture_root/worktree/.pi/extensions"
printf 'fixture CA\n' > "$fixture_root/ca.pem"
printf '{}\n' > "$fixture_root/policy.json"
chmod 0400 "$fixture_root/ca.pem"
chmod 0600 "$fixture_root/policy.json"
cp "$DEN_NATIVE_PI_STARTUP_USER_REPLACEMENT_EXTENSION" \
  "$fixture_root/agent/extensions/user-hostile.ts"
cp "$DEN_NATIVE_PI_STARTUP_PROJECT_REPLACEMENT_EXTENSION" \
  "$fixture_root/worktree/.pi/extensions/project-hostile.ts"
chmod 0600 "$fixture_root/agent/extensions/user-hostile.ts" \
  "$fixture_root/worktree/.pi/extensions/project-hostile.ts"
printf '{"%s":true}\n' "$fixture_root/worktree" > "$fixture_root/agent/trust.json"
chmod 0600 "$fixture_root/agent/trust.json"

# The production manifest must retain its immutable security adapter. The
# negative launches below prove real launcher-boundary rejection instead of
# treating this shape check as that proof.
security_extension=$(jq -er '
  select(.agent.name == "pi") |
  select(.agent.securityAdapter.kind == "pi-extension") |
  select(.agent.securityAdapter.arguments == ["--extension", .agent.securityAdapter.path]) |
  .agent.securityAdapter.path | select(startswith("/nix/store/"))
' "$DEN_NATIVE_PI_STARTUP_MANIFEST")
test -f "$security_extension" && test ! -L "$security_extension"

cat > "$fixture_root/check.mjs" <<'EOF'
import { existsSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
const { DefaultResourceLoader } = await import(process.env.DEN_NATIVE_PI_STARTUP_PACKAGE_ROOT + "/dist/core/resource-loader.js");
const { ExtensionRunner } = await import(process.env.DEN_NATIVE_PI_STARTUP_PACKAGE_ROOT + "/dist/core/extensions/runner.js");
const assert = (condition, message) => { if (!condition) throw new Error(message); };
const root = process.env.DEN_PI_DARWIN_STARTUP_ROOT;
const report = join(root, "assertions.report");
const record = (line) => writeFileSync(report, (existsSync(report) ? readFileSync(report, "utf8") : "") + line + "\n");
const context = (cwd) => ({ cwd, model: undefined, thinkingLevel: undefined, sessionManager: { getSessionId: () => "startup", getSessionFile: () => undefined } });
const load = async (...paths) => {
  const loader = new DefaultResourceLoader({ cwd: process.env.DEN_PI_DARWIN_WORKTREE, agentDir: process.env.PI_CODING_AGENT_DIR, additionalExtensionPaths: paths, noExtensions: false });
  await loader.reload({ resolveProjectTrust: async () => true });
  const loaded = loader.getExtensions();
  assert(loaded.extensions.length === paths.length + 2, "extension load failed: " + JSON.stringify(loaded.errors));
  const scopes = loaded.extensions.map((extension) => extension.sourceInfo?.scope);
  assert(scopes.filter((scope) => scope === "user").length === 1 && scopes.filter((scope) => scope === "project").length === 1,
    "user and project extensions were not loaded through their real scopes: " + JSON.stringify(scopes));
  return new ExtensionRunner(loaded.extensions, loaded.runtime, process.env.DEN_PI_DARWIN_WORKTREE, {}, {});
};
const reject = async (operation, label) => {
  try { await operation(); } catch { record("fail-closed:" + label); return; }
  throw new Error(label + " unexpectedly executed");
};
const security = process.env.DEN_NATIVE_PI_STARTUP_SECURITY_TEST_EXTENSION;
const runner = await load(security);
const bash = runner.getToolDefinition("bash");
assert(bash, "security extension did not own bash");
const cwd = join(root, "worktree");
await reject(() => bash.execute("outside-fence", { command: "printf unfenced > unfenced" }, undefined, undefined, context(cwd)), "outer-fence-required");
assert(!existsSync(join(cwd, "unfenced")), "command ran outside outer Fence");
record("outer-fence-required-for-shell-entrypoints");
process.env.FENCE_SANDBOX = "1";
process.env.DEN_PI_DARWIN_HELPER_MODE = "allow";
await bash.execute("allowed", { command: "printf allowed > allowed-bash" }, undefined, undefined, context(cwd));
assert(readFileSync(join(cwd, "allowed-bash"), "utf8") === "allowed", "allowed built-in bash did not execute");
assert(JSON.parse(readFileSync(process.env.DEN_PI_DARWIN_HELPER_REQUEST, "utf8")).tool_input.command === "printf allowed > allowed-bash", "helper did not receive unchanged command");
record("allowed-bash-after-no-change-helper");
for (const mode of ["deny", "rewrite", "malformed", "failed"]) {
  process.env.DEN_PI_DARWIN_HELPER_MODE = mode;
  await reject(() => bash.execute(mode, { command: "printf blocked > " + mode }, undefined, undefined, context(cwd)), mode);
  assert(!existsSync(join(cwd, mode)), mode + " command reached execution");
}
const user = await runner.emitUserBash({ type: "user_bash", command: "printf user-allowed", cwd, excludeFromContext: false });
assert(user?.operations, "security extension did not own user_bash");
process.env.DEN_PI_DARWIN_HELPER_MODE = "allow";
let output = "";
const userResult = await user.operations.exec("printf user-allowed", cwd, { onData: (data) => { output += data.toString(); } });
assert(userResult.exitCode === 0 && output === "user-allowed", "native ! shell did not use allowed helper path");
process.env.DEN_PI_DARWIN_HELPER_MODE = "deny";
await reject(() => user.operations.exec("printf user-blocked > user-blocked", cwd, { onData: () => {} }), "user-bash-deny");
assert(!existsSync(join(cwd, "user-blocked")), "blocked native ! shell command executed");
record("native-user-bash-parity");
assert(!existsSync(process.env.DEN_REPLACEMENT_BASH_MARKER), "hostile extension replaced bash");
assert(!existsSync(process.env.DEN_REPLACEMENT_USER_BASH_MARKER), "hostile extension replaced user_bash");
record("user-and-project-hostile-extensions-loaded-through-real-scopes");
record("hostile-user-project-extensions-cannot-replace-entrypoints");
process.env.DEN_PI_DARWIN_HELPER_MODE = "allow";
writeFileSync(process.env.DEN_FENCE_POLICY_FILE, "changed\n");
await reject(() => bash.execute("identity", { command: "printf changed" }, undefined, undefined, context(cwd)), "policy-identity");
record("identity-change-fails-closed");
record("helper-created-no-http-or-socks-listener");
EOF

export HOME="$fixture_root/home"
export PI_CODING_AGENT_DIR="$fixture_root/agent"
export DEN_FENCE_POLICY_FILE="$fixture_root/policy.json"
export DEN_PI_DARWIN_HELPER_REQUEST="$fixture_root/helper-request.json"
export DEN_PI_DARWIN_HELPER_LISTENER_REPORT="$fixture_root/helper-listener.report"
export DEN_REPLACEMENT_BASH_MARKER="$fixture_root/replacement-bash"
export DEN_REPLACEMENT_USER_BASH_MARKER="$fixture_root/replacement-user-bash"
export DEN_PI_DARWIN_STARTUP_ROOT="$fixture_root"
export DEN_PI_DARWIN_WORKTREE="$fixture_root/worktree"
"$DEN_NATIVE_PI_STARTUP_NODE" "$fixture_root/check.mjs"
test "$(<"$DEN_PI_DARWIN_HELPER_LISTENER_REPORT")" = no-listener

prestart_launch() {
  local sandbox=$1 output=$2
  (
    cd "$fixture_root/worktree"
    HOME="$fixture_root/home" \
    DEN_NATIVE_INVOKING_HOME="$fixture_root/invoking-home" \
    PI_CODING_AGENT_DIR="$fixture_root/agent" \
    PI_CODING_AGENT_SESSION_DIR="$fixture_root/sessions" \
    DEN_PI_DARWIN_PI_START_MARKER="$fixture_root/pi-started" \
    REPOWOLF_ENDPOINT=https://broker.example.test/ \
    REPOWOLF_TOKEN=rw1_AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE \
    REPOWOLF_CA_FILE="$fixture_root/ca.pem" \
    "$sandbox" --mode rpc < /dev/null > "$output"
  )
}
expect_prestart_rejection() {
  local sandbox=$1 label=$2
  rm -f "$fixture_root/pi-started"
  if prestart_launch "$sandbox" "$fixture_root/$label-version"; then
    printf '%s identity mismatch reached Pi launch\n' "$label" >&2
    exit 1
  fi
  test ! -e "$fixture_root/pi-started"
  printf 'prestart-%s-mismatch-fails-before-launch\n' "$label" >> "$fixture_root/assertions.report"
}

# Both negatives cross the packaged sandbox, den-launcher, and real outer
# Fence. Their marker is written by a real Pi extension only if Pi begins
# loading extensions, so its absence proves rejection before Pi starts.
expect_prestart_rejection "$DEN_NATIVE_PI_STARTUP_PRESTART_EXTENSION_MISMATCH_SANDBOX" extension
expect_prestart_rejection "$DEN_NATIVE_PI_STARTUP_PRESTART_POLICY_MISMATCH_SANDBOX" policy
printf 'outer Fence synthetic secret\n' > "$fixture_root/outside-secret"
chmod 0600 "$fixture_root/outside-secret"
export DEN_PI_DARWIN_EXPECT_OUTER_FENCE=1
export DEN_PI_DARWIN_OUTSIDE="$fixture_root/outside-secret"
export DEN_PI_DARWIN_DIRECT_REPORT="$fixture_root/direct-extension.report"
rm -f "$fixture_root/pi-started"
prestart_launch "$DEN_NATIVE_PI_STARTUP_SANDBOX" "$fixture_root/version"
unset DEN_PI_DARWIN_EXPECT_OUTER_FENCE DEN_PI_DARWIN_OUTSIDE DEN_PI_DARWIN_DIRECT_REPORT
test ! -s "$fixture_root/version"
test "$(<"$fixture_root/pi-started")" = started
test "$(<"$fixture_root/direct-extension.report")" = denied
printf 'direct-extension-process-outer-fence-constrained\n' >> "$fixture_root/assertions.report"
required_assertions='allowed-bash-after-no-change-helper
fail-closed:deny
fail-closed:rewrite
fail-closed:malformed
fail-closed:failed
native-user-bash-parity
hostile-user-project-extensions-cannot-replace-entrypoints
identity-change-fails-closed
helper-created-no-http-or-socks-listener
outer-fence-required-for-shell-entrypoints
direct-extension-process-outer-fence-constrained
prestart-extension-mismatch-fails-before-launch
prestart-policy-mismatch-fails-before-launch'
while IFS= read -r assertion; do
  grep -Fxq "$assertion" "$fixture_root/assertions.report"
done <<< "$required_assertions"
printf 'complete\n' > "$DEN_NATIVE_HOST_ROOT/pi-darwin-startup.complete"
