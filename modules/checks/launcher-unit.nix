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
          nativeBuildInputs = [ pkgs.go ];
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
          touch "$out"
        '';
    };
}
