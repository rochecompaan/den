{ pkgs, fence }:

let
  closure = pkgs.closureInfo {
    rootPaths = [ fence pkgs.bash pkgs.coreutils pkgs.gnugrep pkgs.jq ];
  };
in
pkgs.runCommand "fence-capabilities"
  {
    nativeBuildInputs = [ pkgs.jq ];
  }
  (if pkgs.stdenv.isLinux then ''
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
    mkdir -m 0700 -p "$home/.npm/_logs" "$home/.fence" "$worktree" "$state" "$scratch" "$policyDir"
    printf secret > "$home/secret"
    printf unchanged > "$home/.npm/_logs/implicit"
    printf unchanged > "$home/.fence/debug"
    ca=/tmp/den-fence-capability-ca-$$
    trap 'rm -f "$ca"' EXIT
    printf ca-read-only > "$ca"
    chmod 0400 "$ca"
    policy=$policyDir/fence.json

    jq -Rn '[inputs]' < ${closure}/store-paths > closure.json
    jq -n \
      --argjson closure "$(cat closure.json)" \
      --arg home "$home" --arg worktree "$worktree" --arg state "$state" \
      --arg scratch "$scratch" --arg policy "$policy" --arg policyDir "$policyDir" --arg ca "$ca" \
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
          denyRead: [$home + "/secret"],
          denyWrite: ["~/.npm/_logs", "~/.fence/debug", "/tmp/fence", "/private/tmp/fence", $policy, $policyDir]
        },
        command: { deny: [], useDefaults: true, acceptSharedBinaryCannotRuntimeDeny: ["chroot"], runtimeExecPolicy: "argv" }
      }' > "$policy"
    chmod 0400 "$policy"

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

    export HOME="$home" WORKTREE="$worktree" STATE="$state" SCRATCH="$scratch"
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
      if cat ${pkgs.hello}/bin/hello >/dev/null 2>&1; then exit 21; fi
      if printf changed > "$HOME/.npm/_logs/implicit" 2>/dev/null; then exit 22; fi
      if printf changed > "$HOME/.fence/debug" 2>/dev/null; then exit 23; fi
      if printf changed >> "$POLICY" 2>/dev/null; then exit 24; fi
      if printf changed > "$(dirname "$POLICY")/other" 2>/dev/null; then exit 25; fi
      if printf changed > "$CA" 2>/dev/null; then exit 26; fi
    '
    "$fence" --settings "$policy" --expose-host-path "$ca" -c 'test "$TMPDIR" = "$SCRATCH"; test "$DEN_FENCE_TMPDIR" = "$SCRATCH"; printf nested > "$TMPDIR/nested"'
    test "$(cat "$scratch/outer")" = outer
    test "$(cat "$scratch/nested")" = nested
    test "$(cat "$home/.npm/_logs/implicit")" = unchanged
    test "$(cat "$home/.fence/debug")" = unchanged
    test "$(cat "$ca")" = ca-read-only
    test "$(stat -c %a "$policy")" = 400
    touch "$out"
  '' else ''
    set -eu
    ${fence}/bin/fence --version | grep -F 'Version: 0.1.58'
    ${fence}/bin/fence --help > help.txt
    grep -F -- '--settings string' help.txt
    grep -F -- '-c, --c string' help.txt
    grep -F -- '--expose-host-path stringArray' help.txt
    printf '{}\n' | ${fence}/bin/fence --claude-pre-tool-use >hook.out 2>hook.err
    test ! -s hook.err
    cat > policy.json <<'EOF'
    {
      "network": {
        "allowedDomains": [],
        "deniedDomains": [],
        "allowUnixSockets": ["/tmp/den-capability.sock"],
        "allowLocalOutbound": true,
        "allowLocalOutboundPorts": [65534]
      },
      "filesystem": { "defaultDenyRead": true, "strictDenyRead": true },
      "command": { "runtimeExecPolicy": "argv" }
    }
    EOF
    ${fence}/bin/fence config show --settings policy.json > parsed.json
    ${pkgs.jq}/bin/jq -e '
      .filesystem.defaultDenyRead == true and
      .filesystem.strictDenyRead == true and
      .command.runtimeExecPolicy == "argv" and
      .network.allowUnixSockets == ["/tmp/den-capability.sock"] and
      .network.allowLocalOutboundPorts == [65534]
    ' parsed.json
    touch "$out"
  '')
