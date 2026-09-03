{ ... }:

{
  perSystem = { pkgs, ... }:
    let
      den-launcher = import ../../nix/packages/den-launcher.nix { inherit pkgs; };
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
          ln -s ${den-launcher.goModules} vendor

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
          go test -mod=vendor ./internal/policy -count=1

          denSource=$PWD
          cp -R ${fence.package.src} fence-source
          chmod -R u+w fence-source
          ln -s ${fence.package.goModules} fence-source/vendor
          cp nix/check-support/fence-domain-policy_test.go.fixture \
            fence-source/internal/proxy/den_policy_test.go
          cd fence-source
          DEN_POLICY_PATH="$denSource/policy/fence.json" \
            go test -mod=vendor ./internal/proxy -run '^TestDenPolicyDomainDecisions$' -count=1 -v
          touch "$out"
        '';
    };
}
