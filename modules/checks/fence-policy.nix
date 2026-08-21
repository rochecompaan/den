{ ... }:

{
  perSystem = { pkgs, ... }:
    let
      fence = import ../../nix/lib/fence.nix { inherit pkgs; };
      fence-capabilities = import ../../nix/check-support/fence-capabilities.nix {
        inherit pkgs;
        fence = fence.package;
      };
    in
    {
      checks.fence-capabilities = fence-capabilities;
      checks.fence-policy = pkgs.runCommand "fence-policy"
        {
          src = ../..;
          nativeBuildInputs = [ pkgs.go pkgs.jq ];
        }
        ''
          set -eu
          export HOME="$TMPDIR/home"
          export CGO_ENABLED=0
          mkdir -p "$HOME"
          cp -R "$src" source
          chmod -R u+w source
          cd source

          jq -e '
            .allowPty == true and
            .filesystem.defaultDenyRead == true and
            .filesystem.strictDenyRead == true and
            .filesystem.allowGitConfig == true and
            (.filesystem.allowRead | length) == 0 and
            (.filesystem.allowExecute | length) == 0 and
            (.filesystem.allowWrite | length) == 0 and
            .command.runtimeExecPolicy == "argv"
          ' policy/fence.json
          if grep -R -F 'rw1_' policy; then
            echo 'base policy contains a RepoWolf token prefix' >&2
            exit 1
          fi
          go test ./internal/policy -count=1
          touch "$out"
        '';
    };
}
