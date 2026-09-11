{ ... }:
{
  perSystem = { pkgs, ... }: {
    checks.pi-package = import ../../nix/check-support/pi-package.nix { inherit pkgs; };
  };
}
