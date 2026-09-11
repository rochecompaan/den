{ inputs, ... }:
{
  perSystem = { pkgs, ... }: {
    checks.pi-resources = import ../../nix/check-support/pi-resources.nix { inherit inputs pkgs; };
  };
}
