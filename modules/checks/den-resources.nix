{ ... }:
{
  perSystem = { pkgs, ... }: {
    checks.den-resources = import ../../nix/check-support/den-resources.nix { inherit pkgs; };
  };
}
