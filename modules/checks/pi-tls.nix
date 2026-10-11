{ inputs, ... }:
{
  perSystem = { pkgs, ... }: {
    checks.pi-tls = import ../../nix/check-support/pi-tls.nix { inherit inputs pkgs; };
  };
}
