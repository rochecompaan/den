{ inputs, ... }:
{
  perSystem = { pkgs, ... }: {
    checks.pi-adapter = import ../../nix/check-support/pi-adapter.nix { inherit inputs pkgs; };
  };
}
