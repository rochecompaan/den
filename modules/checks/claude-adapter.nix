{ ... }:
{
  perSystem = { pkgs, ... }:
    let
      fence = (import ../../nix/lib/fence.nix { inherit pkgs; }).package;
    in
    {
      checks = {
        claude-adapter = import ../../nix/check-support/claude-adapter.nix {
          inherit pkgs fence;
        };
        claude-settings-merge = import ../../nix/check-support/claude-settings-merge.nix {
          inherit pkgs;
        };
      };
    };
}
