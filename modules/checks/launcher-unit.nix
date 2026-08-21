{ ... }:

{
  perSystem = { pkgs, ... }:
    let
      den-launcher = import ../../nix/packages/den-launcher.nix { inherit pkgs; };
      git-transport = import ../../nix/check-support/git-transport.nix { inherit pkgs; };
      process-fixture = import ../../nix/check-support/process-fixture.nix { inherit pkgs; };
    in
    {
      checks.launcher-unit = pkgs.runCommand "launcher-unit"
        {
          src = ../..;
          nativeBuildInputs = [ pkgs.go pkgs.util-linux pkgs.procps ];
        }
        ''
          export HOME="$TMPDIR"
          export CGO_ENABLED=0
          cp -R "$src" source
          chmod -R u+w source
          cd source
          go test ./internal/... ./cmd/... -count=1

          test -x ${den-launcher}/bin/den-launcher
          test ! -e ${den-launcher}/bin/den
          test -e ${git-transport}
          test -x ${process-fixture}/bin/den-process-fixture
          ${pkgs.util-linux}/bin/script -qfec '${process-fixture}/bin/den-process-fixture pty' /dev/null > pty.out
          tr -d '\r' < pty.out | grep -qx 'pty-ok'
          export DEN_PROCESS_PID_FILE="$TMPDIR/process.pid"
          export DEN_PROCESS_SIGNAL_FILE="$TMPDIR/process.signals"
          ${pkgs.util-linux}/bin/script -qfec '${process-fixture}/bin/den-process-fixture job-control' /dev/null &
          fixture_pid=$!
          for _ in $(seq 1 50); do test -e "$DEN_PROCESS_PID_FILE" && break; sleep 0.1; done
          test -s "$DEN_PROCESS_PID_FILE"
          child_pid=$(cat "$DEN_PROCESS_PID_FILE")
          kill -WINCH "$child_pid"
          sleep 1
          grep -q W "$DEN_PROCESS_SIGNAL_FILE"
          kill -TSTP "$child_pid"
          sleep 1
          grep -q T "$DEN_PROCESS_SIGNAL_FILE"
          kill -CONT "$child_pid"
          wait "$fixture_pid"
          grep -q C "$DEN_PROCESS_SIGNAL_FILE"
          touch "$out"
        '';
    };
}
