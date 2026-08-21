{ ... }:
{
  perSystem = { pkgs, ... }:
    let
      fence = (import ../../nix/lib/fence.nix { inherit pkgs; }).package;
    in
    {
      checks.claude-adapter = import ../../nix/check-support/claude-adapter.nix {
        inherit pkgs fence;
      };
    };
}
