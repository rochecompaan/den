{ ... }:

{
  perSystem = { pkgs, ... }:
    let
      den-launcher = import ../../nix/packages/den-launcher.nix { inherit pkgs; };
      git-transport = import ../../nix/check-support/git-transport.nix { inherit pkgs; };
      piDarwinSecurityExtension = pkgs.writeText "pi-darwin-security-extension-test" "fixture\n";
      piDarwinManifestExtension = pkgs.writeText "pi-darwin-manifest-extension-test" "fixture\n";
      script = pkgs.unixtools.script;
      scriptInvocation = command:
        if pkgs.stdenv.hostPlatform.isDarwin then
          "${script}/bin/script -q /dev/null ./process-harness ${command}"
        else
          "${script}/bin/script -qfec './process-harness ${command}' /dev/null";
    in
    {
      checks.launcher-unit = pkgs.runCommand "launcher-unit"
        {
          src = ../..;
          nativeBuildInputs = [ pkgs.go script pkgs.procps pkgs.python3 pkgs.jq ];
        }
        ''
          export HOME="$TMPDIR"
          export CGO_ENABLED=0
          cp -R "$src" source
          chmod -R u+w source
          cd source
          ln -s ${den-launcher.goModules} vendor
          go test -mod=vendor ./internal/... ./cmd/... -count=1
          ${pkgs.python3}/bin/python3 tests/check-derivation-impure-host-deps.py \
            "$PWD/scripts/check-derivation-impure-host-deps.py"
          ${pkgs.bash}/bin/bash tests/check-native-driver.sh "$PWD/scripts/check-native.sh"
          ${pkgs.bash}/bin/bash tests/native-runner.sh \
            "$PWD/nix/check-support/native-runner.sh"
          ${pkgs.bash}/bin/bash tests/native-resolver-lifecycle.sh \
            "$PWD/nix/check-support/native-resolver-lifecycle.sh"
          ${pkgs.bash}/bin/bash tests/claude-startup-runtime-manifest.sh \
            "$PWD/nix/check-support/claude-startup-runtime-manifest.sh"
          ${pkgs.bash}/bin/bash tests/pi-darwin-startup.sh \
            "$PWD/nix/check-support/pi-darwin-startup.sh" \
            ${piDarwinSecurityExtension} ${piDarwinManifestExtension}

          test -x ${den-launcher}/bin/den-launcher
          test ! -e ${den-launcher}/bin/den
          test -e ${git-transport}
          go build -o process-harness ./cmd/process-harness
          ${scriptInvocation "pty"} > pty.out
          tr -d '\r' < pty.out | grep -qx 'pty-ok'
          export DEN_PROCESS_PID_FILE="$TMPDIR/process.pid"
          export DEN_PROCESS_SIGNAL_FILE="$TMPDIR/process.signals"
          export DEN_PROCESS_READY_FILE="$TMPDIR/process.ready"
          harness_pid=
          group_id=
          cleanup_job_control() {
            if test -n "$group_id"; then
              kill -TERM "-$group_id" 2>/dev/null || true
            fi
            if test -n "$harness_pid"; then
              kill -TERM "$harness_pid" 2>/dev/null || true
              wait "$harness_pid" 2>/dev/null || true
            fi
          }
          wait_for_process_signal() {
            expected=$1
            for _ in $(seq 1 50); do
              if test -f "$DEN_PROCESS_SIGNAL_FILE" &&
                grep -Fq "$expected" "$DEN_PROCESS_SIGNAL_FILE"; then
                return 0
              fi
              sleep 0.1
            done
            printf 'timed out waiting for process signal %s\n' "$expected" >&2
            return 1
          }
          ${scriptInvocation "job-control"} &
          harness_pid=$!
          trap cleanup_job_control EXIT
          for _ in $(seq 1 50); do test -e "$DEN_PROCESS_PID_FILE" && break; sleep 0.1; done
          test -s "$DEN_PROCESS_PID_FILE"
          for _ in $(seq 1 50); do test -e "$DEN_PROCESS_READY_FILE" && break; sleep 0.1; done
          test -s "$DEN_PROCESS_READY_FILE"
          group_id=$(cat "$DEN_PROCESS_PID_FILE")
          kill -WINCH "-$group_id"
          wait_for_process_signal W
          kill -TSTP "-$group_id"
          wait_for_process_signal T
          kill -CONT "-$group_id"
          wait "$harness_pid"
          grep -Fq C "$DEN_PROCESS_SIGNAL_FILE"
          harness_pid=
          trap - EXIT
          touch "$out"
        '';
    };
}
