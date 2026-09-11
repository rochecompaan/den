{ ... }:
{
  perSystem = { pkgs, ... }: {
    checks.pi-security-extension = import ../../nix/check-support/pi-security-extension.nix { inherit pkgs; };
  };
}
