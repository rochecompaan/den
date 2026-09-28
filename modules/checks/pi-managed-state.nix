{ inputs, ... }:
{
  perSystem = { pkgs, ... }: {
    checks.pi-managed-state = import ../../nix/check-support/pi-managed-state.nix {
      inherit inputs pkgs;
    };
  };
}
