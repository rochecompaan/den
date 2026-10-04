{ inputs, pkgs }:
let
  lib = pkgs.lib;
  mkPi = import ../lib/mk-pi.nix { inherit inputs pkgs; };
  settings = pkgs.writeText "managed-settings.json" ''{"theme":"default"}\n'';
  pathSource = ./pi-module-api.nix;
  pi = mkPi {
    stateFiles.agent = {
      "settings.json" = settings;
      "profiles/pi-subagents/openai.json" = pathSource;
    };
  };
  fails = value: !(builtins.tryEval (builtins.deepSeq value value)).success;
in
assert fails ((mkPi { stateFiles.agent."../escape" = settings; }).outPath);
assert fails ((mkPi { stateFiles.agent."settings.json" = "/tmp/settings.json"; }).outPath);
assert fails ((mkPi { stateFiles.agent."settings.json" = "${settings}/../escape"; }).outPath);
pkgs.runCommand "pi-managed-state"
  {
    nativeBuildInputs = [ pkgs.coreutils pkgs.jq ];
    manifest = pi.denManifest;
    inherit pi settings pathSource;
  }
  ''
    set -euo pipefail
    jq -e --arg settings "$settings" --arg pathSource "$pathSource" '
      .stateBindings[0].managedFiles == [
        {destination:"profiles/pi-subagents/openai.json", source:$pathSource},
        {destination:"settings.json", source:$settings}
      ] and
      .stateBindings[1].managedFiles == []
    ' "$manifest"
    closure=$(jq -r .closurePathsFile "$manifest")
    grep -Fqx "$settings" "$closure"
    grep -Fqx "$pathSource" "$closure"

    mkdir -p "$TMPDIR/home" "$TMPDIR/workspace"
    export HOME="$TMPDIR/home"
    export PI_CODING_AGENT_DIR="$TMPDIR/pi-agent"
    export PI_CODING_AGENT_SESSION_DIR="$TMPDIR/pi-sessions"
    export REPOWOLF_ENDPOINT=https://broker.example.test/
    export REPOWOLF_TOKEN=rw1_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
    printf certificate > "$TMPDIR/ca.pem"
    chmod 0400 "$TMPDIR/ca.pem"
    export REPOWOLF_CA_FILE="$TMPDIR/ca.pem"
    cd "$TMPDIR/workspace"

    run_wrapper() {
      log=$1
      if ${pkgs.coreutils}/bin/timeout 20 ${pi}/bin/pi --mode rpc < /dev/null > "$log" 2>&1; then
        status=0
      else
        status=$?
      fi
      if [ "$status" -eq 124 ]; then
        cat "$log" >&2
        echo 'Pi wrapper timed out' >&2
        exit 1
      fi
      printf '%s' "$status" > "$log.status"
    }

    run_wrapper "$TMPDIR/fresh.log"
    test -L "$PI_CODING_AGENT_DIR/settings.json"
    test "$(readlink "$PI_CODING_AGENT_DIR/settings.json")" = "$settings"
    test -L "$PI_CODING_AGENT_DIR/profiles/pi-subagents/openai.json"
    test "$(readlink "$PI_CODING_AGENT_DIR/profiles/pi-subagents/openai.json")" = "$pathSource"

    printf auth > "$TMPDIR/expected-auth.json"
    printf custom > "$TMPDIR/expected-custom.json"
    cp "$TMPDIR/expected-auth.json" "$PI_CODING_AGENT_DIR/auth.json"
    cp "$TMPDIR/expected-custom.json" "$PI_CODING_AGENT_DIR/profiles/pi-subagents/custom.json"
    rm "$PI_CODING_AGENT_DIR/settings.json"
    printf replacement > "$PI_CODING_AGENT_DIR/settings.json"
    run_wrapper "$TMPDIR/replacement.log"
    test -L "$PI_CODING_AGENT_DIR/settings.json"
    test "$(readlink "$PI_CODING_AGENT_DIR/settings.json")" = "$settings"
    cmp "$TMPDIR/expected-auth.json" "$PI_CODING_AGENT_DIR/auth.json"
    cmp "$TMPDIR/expected-custom.json" "$PI_CODING_AGENT_DIR/profiles/pi-subagents/custom.json"

    mkdir "$TMPDIR/outside" "$TMPDIR/expected-outside"
    printf outside > "$TMPDIR/expected-outside/sentinel"
    cp "$TMPDIR/expected-outside/sentinel" "$TMPDIR/outside/sentinel"
    rm -rf "$PI_CODING_AGENT_DIR/profiles"
    ln -s "$TMPDIR/outside" "$PI_CODING_AGENT_DIR/profiles"
    run_wrapper "$TMPDIR/rejected.log"
    status=$(cat "$TMPDIR/rejected.log.status")
    test "$status" -ne 0
    grep -F 'managed state "profiles/pi-subagents/openai.json": parent is a symbolic link' "$TMPDIR/rejected.log"
    diff -r "$TMPDIR/expected-outside" "$TMPDIR/outside"
    touch "$out"
  ''
