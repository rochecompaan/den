{ pkgs, fence }:

let
  fenceInfo = import ../lib/fence.nix { inherit pkgs; };
  closure = pkgs.closureInfo {
    rootPaths = [ fence pkgs.bash pkgs.coreutils pkgs.gnugrep pkgs.jq ];
  };
  linuxCapabilities = ''
    set -eu
    fence=${fence}/bin/fence

    "$fence" --version | grep -F 'Version: 0.1.58'
    "$fence" --help > help.txt
    grep -F -- '--settings string' help.txt
    grep -F -- '-c, --c string' help.txt
    grep -F -- '--linux-features' help.txt
    grep -F -- '--expose-host-path stringArray' help.txt

    printf '{}\n' | "$fence" --claude-pre-tool-use >hook.out 2>hook.err
    test ! -s hook.err

    "$fence" --linux-features > features.txt
    test "$(awk '$1 == "Network" && $2 == "namespace" { print $6 }' features.txt)" = ok
    test "$(grep -c '^  Network namespace  ' features.txt)" = 1

    root=$TMPDIR/capabilities
    home=$root/home
    worktree=$root/worktree
    state=$root/state
    scratch=$root/scratch
    policyDir=$worktree/.den-policy
    maskedDir=$worktree/deny-read-mask
    mkdir -m 0700 -p "$home/.npm/_logs" "$home/.fence" "$worktree" "$state" "$scratch" "$policyDir" "$maskedDir"
    printf secret > "$home/secret"
    printf unchanged > "$home/.npm/_logs/implicit"
    printf unchanged > "$home/.fence/debug"
    ca=/tmp/den-fence-capability-ca-$$
    tmpFenceProbe=/tmp/fence/den-capability-$$
    cleanup() {
      rm -f "$ca" "$tmpFenceProbe"
      rmdir /tmp/fence 2>/dev/null || true
    }
    trap cleanup EXIT
    printf ca-read-only > "$ca"
    chmod 0400 "$ca"
    policy=$policyDir/fence.json

    jq -Rn '[inputs]' < ${closure}/store-paths > closure.json
    jq -n \
      --argjson closure "$(cat closure.json)" \
      --arg home "$home" --arg worktree "$worktree" --arg state "$state" \
      --arg scratch "$scratch" --arg policy "$policy" --arg policyDir "$policyDir" --arg maskedDir "$maskedDir" --arg ca "$ca" \
      '{
        allowPty: true,
        network: {
          allowedDomains: [], deniedDomains: [], allowUnixSockets: ["/tmp/nonexistent-den-capability.sock"],
          allowLocalOutbound: true, allowLocalOutboundPorts: [65534]
        },
        filesystem: {
          defaultDenyRead: true, strictDenyRead: true, allowGitConfig: true,
          allowRead: ($closure + [$worktree, $state, $scratch, $policy, $ca]),
          allowExecute: $closure,
          allowWrite: [$worktree, $state, $scratch],
          denyRead: [$home + "/secret", $maskedDir],
          denyWrite: ["~/.npm/_logs", "~/.fence/debug", "/tmp/fence", "/tmp/fence/**", "/private/tmp/fence", "/private/tmp/fence/**", $policy, $policyDir, $maskedDir]
        },
        command: { deny: ["printf denied"], useDefaults: true, acceptSharedBinaryCannotRuntimeDeny: ["chroot"], runtimeExecPolicy: "argv" }
      }' > "$policy"
    chmod 0400 "$policy"

    request='{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"printf allowed","cwd":"/tmp"},"cwd":"/tmp"}'
    printf '%s\n' "$request" | FENCE_SANDBOX=1 DEN_FENCE_TMPDIR="$scratch" TMPDIR="$scratch" \
      "$fence" --claude-pre-tool-use --settings "$policy" > helper-allow.out 2> helper-allow.err
    test ! -s helper-allow.out
    test ! -s helper-allow.err
    request='{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"printf denied","cwd":"/tmp"},"cwd":"/tmp"}'
    printf '%s\n' "$request" | FENCE_SANDBOX=1 DEN_FENCE_TMPDIR="$scratch" TMPDIR="$scratch" \
      "$fence" --claude-pre-tool-use --settings "$policy" > helper-deny.out 2> helper-deny.err
    printf '%s\n' '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny"}}' > helper-deny.expected
    cmp helper-deny.expected helper-deny.out
    test ! -s helper-deny.err
    test -z "$(find "$scratch" -mindepth 1 -print -quit)"

    "$fence" config show --settings "$policy" > parsed.json
    jq -e '
      .filesystem.defaultDenyRead == true and
      .filesystem.strictDenyRead == true and
      .command.runtimeExecPolicy == "argv" and
      .network.allowUnixSockets == ["/tmp/nonexistent-den-capability.sock"] and
      .network.allowLocalOutboundPorts == [65534]
    ' parsed.json
    chmod 0600 "$policy"
    jq 'del(.network.allowUnixSockets, .network.allowLocalOutbound, .network.allowLocalOutboundPorts, .command.runtimeExecPolicy)' "$policy" > "$policy.next"
    mv "$policy.next" "$policy"
    chmod 0400 "$policy"

    export HOME="$home" WORKTREE="$worktree" STATE="$state" SCRATCH="$scratch" MASKED_DIR="$maskedDir"
    export POLICY="$policy" CA="$ca"
    export TMPDIR="$scratch" DEN_FENCE_TMPDIR="$scratch"
    "$fence" --settings "$policy" --expose-host-path "$ca" -- ${pkgs.bash}/bin/bash -c '
      set -eu
      test "$TMPDIR" = "$SCRATCH"
      test "$DEN_FENCE_TMPDIR" = "$SCRATCH"
      printf outer > "$TMPDIR/outer"
      printf work > "$WORKTREE/write"
      printf state > "$STATE/write"
      test "$(cat "$CA")" = ca-read-only
      if cat "$HOME/secret" >/dev/null 2>&1; then exit 20; fi
      if printf escaped > "$MASKED_DIR/created" 2>/dev/null; then exit 21; fi
      if cat ${pkgs.hello}/bin/hello >/dev/null 2>&1; then exit 22; fi
      if printf changed > "$HOME/.npm/_logs/implicit" 2>/dev/null; then exit 23; fi
      if printf changed > "$HOME/.fence/debug" 2>/dev/null; then exit 24; fi
      if printf changed >> "$POLICY" 2>/dev/null; then exit 25; fi
      if printf changed > "$(dirname "$POLICY")/other" 2>/dev/null; then exit 26; fi
      if printf changed > "$CA" 2>/dev/null; then exit 27; fi
    '
    "$fence" --settings "$policy" --expose-host-path "$ca" -c 'test "$TMPDIR" = "$SCRATCH"; test "$DEN_FENCE_TMPDIR" = "$SCRATCH"; printf nested > "$TMPDIR/nested"'
    test "$(cat "$scratch/outer")" = outer
    test "$(cat "$scratch/nested")" = nested
    test "$(cat "$home/.npm/_logs/implicit")" = unchanged
    test "$(cat "$home/.fence/debug")" = unchanged
    test "$(cat "$ca")" = ca-read-only
    test "$(stat -c %a "$policy")" = 400

    mkdir -p /tmp/fence
    printf unchanged > "$tmpFenceProbe"
    tmpFenceChild=/tmp/fence/den-capability-child-$$
    jq '.filesystem.denyWrite |= map(select(startswith("/tmp/fence") | not))' "$policy" > "$policy.tmpfs"
    chmod 0400 "$policy.tmpfs"
    "$fence" --settings "$policy.tmpfs" --expose-host-path "$ca" -c \
      "printf inner > /tmp/fence/den-capability-$$; printf inner > /tmp/fence/den-capability-child-$$"
    test "$(cat "$tmpFenceProbe")" = unchanged
    test ! -e "$tmpFenceChild"
    touch "$out"
  '';
  darwinCapabilities = ''
    set -eu
    fence=${fence}/bin/fence

    : "''${DEN_NATIVE_HOST_ROOT:?native host root is required}"
    root=$DEN_NATIVE_HOST_ROOT/fence-capabilities
    mkdir -p "$root"
    chmod 0700 "$root"
    cd "$root"

    "$fence" --version | grep -F 'Version: 0.1.58'
    "$fence" --help > help.txt
    grep -F -- '--settings string' help.txt
    grep -F -- '-c, --c string' help.txt
    grep -F -- '--expose-host-path stringArray' help.txt
    printf '{}\n' | "$fence" --claude-pre-tool-use >hook.out 2>hook.err
    test ! -s hook.err

    home=$root/home
    worktree=$root/worktree
    state=$root/state
    scratch=$root/scratch
    policyDir=$worktree/.den-policy
    mkdir -p "$home/.npm/_logs" "$home/.fence" "$worktree" "$state" "$scratch" "$policyDir"
    chmod 0700 "$home/.npm/_logs" "$home/.fence" "$worktree" "$state" "$scratch" "$policyDir"
    printf secret > "$home/secret"
    printf unchanged > "$home/.npm/_logs/implicit"
    printf unchanged > "$home/.fence/debug"
    tmpFenceProbe=/tmp/fence/den-capability-$$
    privateFenceProbe=/private/tmp/fence/den-capability-private-$$
    mkdir -p /tmp/fence /private/tmp/fence
    printf unchanged > "$tmpFenceProbe"
    printf unchanged > "$privateFenceProbe"
    cleanup() {
      rm -f "$tmpFenceProbe" "$privateFenceProbe"
    }
    trap cleanup EXIT
    policy=$policyDir/fence.json

    jq -Rn '[inputs]' < ${closure}/store-paths > closure.json
    # shellcheck disable=SC2094
    jq -n \
      --argjson closure "$(cat closure.json)" \
      --arg home "$home" --arg worktree "$worktree" --arg state "$state" \
      --arg scratch "$scratch" --arg policy "$policy" --arg policyDir "$policyDir" \
      '{
        allowPty: true,
        network: {
          allowedDomains: [], deniedDomains: [], allowUnixSockets: ["/tmp/nonexistent-den-capability.sock"],
          allowLocalOutbound: true, allowLocalOutboundPorts: [65534]
        },
        filesystem: {
          defaultDenyRead: true, strictDenyRead: true, allowGitConfig: true,
          allowRead: ($closure + ["/System/Library", "/usr/lib", "/usr/share/icu", "/private/etc", "/private/var/db/timezone", $worktree, $state, $scratch, $policy]),
          allowExecute: $closure,
          allowWrite: [$worktree, $state, $scratch],
          denyRead: [$home + "/secret"],
          denyWrite: ["~/.npm/_logs", "~/.fence/debug", "/tmp/fence", "/tmp/fence/**", "/private/tmp/fence", "/private/tmp/fence/**", $policy, $policyDir]
        },
        command: { deny: ["printf denied"], useDefaults: true, acceptSharedBinaryCannotRuntimeDeny: ["chroot"], runtimeExecPolicy: "argv" }
      }' > "$policy"
    chmod 0400 "$policy"

    request='{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"printf allowed","cwd":"/tmp"},"cwd":"/tmp"}'
    printf '%s\n' "$request" | FENCE_SANDBOX=1 DEN_FENCE_TMPDIR="$scratch" TMPDIR="$scratch" \
      "$fence" --claude-pre-tool-use --settings "$policy" > helper-allow.out 2> helper-allow.err
    test ! -s helper-allow.out
    test ! -s helper-allow.err
    request='{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"printf denied","cwd":"/tmp"},"cwd":"/tmp"}'
    printf '%s\n' "$request" | FENCE_SANDBOX=1 DEN_FENCE_TMPDIR="$scratch" TMPDIR="$scratch" \
      "$fence" --claude-pre-tool-use --settings "$policy" > helper-deny.out 2> helper-deny.err
    printf '%s\n' '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny"}}' > helper-deny.expected
    cmp helper-deny.expected helper-deny.out
    test ! -s helper-deny.err
    test -z "$(find "$scratch" -mindepth 1 -print -quit)"

    "$fence" config show --settings "$policy" > parsed.json
    jq -e '
      .filesystem.defaultDenyRead == true and
      .filesystem.strictDenyRead == true and
      .command.runtimeExecPolicy == "argv" and
      .network.allowUnixSockets == ["/tmp/nonexistent-den-capability.sock"] and
      .network.allowLocalOutboundPorts == [65534]
    ' parsed.json
    chmod 0600 "$policy"
    jq 'del(.network.allowLocalOutbound, .network.allowLocalOutboundPorts, .command.runtimeExecPolicy)' "$policy" > "$policy.next"
    mv "$policy.next" "$policy"
    chmod 0400 "$policy"

    export HOME="$home" WORKTREE="$worktree" STATE="$state" SCRATCH="$scratch"
    export POLICY="$policy" TMP_FENCE_PROBE="$tmpFenceProbe" PRIVATE_FENCE_PROBE="$privateFenceProbe"
    export TMPDIR="$scratch" DEN_FENCE_TMPDIR="$scratch"
    # shellcheck disable=SC2016
    "$fence" --settings "$policy" -- ${pkgs.bash}/bin/bash -c '
      set -eu
      test "$TMPDIR" = "$SCRATCH"
      test "$DEN_FENCE_TMPDIR" = "$SCRATCH"
      printf outer > "$TMPDIR/outer"
      printf work > "$WORKTREE/write"
      printf state > "$STATE/write"
      if cat "$HOME/secret" >/dev/null 2>&1; then exit 20; fi
      if cat ${pkgs.hello}/bin/hello >/dev/null 2>&1; then exit 21; fi
      if printf changed > "$HOME/.npm/_logs/implicit" 2>/dev/null; then exit 22; fi
      if printf changed > "$HOME/.fence/debug" 2>/dev/null; then exit 23; fi
      if printf changed > "$TMP_FENCE_PROBE" 2>/dev/null; then exit 24; fi
      if printf changed > "$PRIVATE_FENCE_PROBE" 2>/dev/null; then exit 25; fi
      if printf changed >> "$POLICY" 2>/dev/null; then exit 26; fi
      if printf changed > "$(dirname "$POLICY")/other" 2>/dev/null; then exit 27; fi
    '
    # shellcheck disable=SC2016
    "$fence" --settings "$policy" -c 'test "$TMPDIR" = "$SCRATCH"; test "$DEN_FENCE_TMPDIR" = "$SCRATCH"; printf wrapper > "$TMPDIR/wrapper"'
    test "$(cat "$scratch/outer")" = outer
    test "$(cat "$scratch/wrapper")" = wrapper
    test "$(cat "$home/.npm/_logs/implicit")" = unchanged
    test "$(cat "$home/.fence/debug")" = unchanged
    test "$(cat "$tmpFenceProbe")" = unchanged
    test "$(cat "$privateFenceProbe")" = unchanged
    test "$(${pkgs.coreutils}/bin/stat -c %a "$policy")" = 400
    printf 'complete\n' > "$DEN_NATIVE_HOST_ROOT/fence-capabilities.complete"
  '';
in
assert fence == fenceInfo.package;
assert fenceInfo.version == "0.1.58";
assert fenceInfo.sourceHash == "sha256-ACe3N4bXYJW6QDQHtRChFWOTXTZTbEUbZ4d8cuFRqMY=";
assert fenceInfo.patchHash == "4be4f0266a0a79da10002893752ea8185915f6ecfb146513946bde8a96e41e2a";
assert fenceInfo.readOnlyMaskPatchHash == "440a5162ff4a3d2c8a3566d0ae018c166c03f86be54f74d5a9cfece65dc4def0";
assert fenceInfo.capabilities.claudePreToolUse;
assert fenceInfo.capabilities.denFenceTmpdir;
assert fenceInfo.capabilities.strictDenyRead;
assert if pkgs.stdenv.isDarwin then fenceInfo.capabilities.allowUnixSockets else fenceInfo.capabilities.linuxReadOnlyDenyReadMasks && fenceInfo.capabilities.argvRuntimePolicy;
if pkgs.stdenv.isLinux then
  pkgs.runCommand "fence-capabilities"
    {
      nativeBuildInputs = [ pkgs.jq ];
    }
    linuxCapabilities
else
  pkgs.writeShellApplication {
    name = "fence-capabilities";
    runtimeInputs = [ pkgs.coreutils pkgs.gnugrep pkgs.jq ];
    derivationArgs = {
      passthru.denHostFixturePlatform = "darwin";
    };
    text = darwinCapabilities;
  }
