{ ... }:
{
  perSystem = { pkgs, ... }: {
    checks.claude-resources = import ../../nix/check-support/claude-resources.nix { inherit pkgs; };
  };
}
