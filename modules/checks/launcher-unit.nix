{ ... }:

{
  perSystem = { pkgs, ... }:
    let
      den-launcher = import ../../nix/packages/den-launcher.nix { inherit pkgs; };
      git-transport = import ../../nix/check-support/git-transport.nix { inherit pkgs; };
    in
    {
      checks.launcher-unit = pkgs.runCommand "launcher-unit"
        {
          src = ../..;
          nativeBuildInputs = [ pkgs.go pkgs.util-linux pkgs.procps pkgs.python3 pkgs.jq ];
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

          test -x ${den-launcher}/bin/den-launcher
          test ! -e ${den-launcher}/bin/den
          test -e ${git-transport}
          go build -o process-harness ./cmd/process-harness
          ${pkgs.util-linux}/bin/script -qfec './process-harness pty' /dev/null > pty.out
          tr -d '\r' < pty.out | grep -qx 'pty-ok'
          export DEN_PROCESS_PID_FILE="$TMPDIR/process.pid"
          export DEN_PROCESS_SIGNAL_FILE="$TMPDIR/process.signals"
          export DEN_PROCESS_READY_FILE="$TMPDIR/process.ready"
          ${pkgs.util-linux}/bin/script -qfec './process-harness job-control' /dev/null &
          harness_pid=$!
          for _ in $(seq 1 50); do test -e "$DEN_PROCESS_PID_FILE" && break; sleep 0.1; done
          test -s "$DEN_PROCESS_PID_FILE"
          for _ in $(seq 1 50); do test -e "$DEN_PROCESS_READY_FILE" && break; sleep 0.1; done
          test -s "$DEN_PROCESS_READY_FILE"
          group_id=$(cat "$DEN_PROCESS_PID_FILE")
          kill -WINCH "-$group_id"
          sleep 1
          grep -q W "$DEN_PROCESS_SIGNAL_FILE"
          kill -TSTP "-$group_id"
          sleep 1
          kill -CONT "-$group_id"
          wait "$harness_pid"
          grep -q TC "$DEN_PROCESS_SIGNAL_FILE"
          touch "$out"
        '';
    };
}
