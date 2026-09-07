{ inputs, self, ... }:
{
  perSystem = { pkgs, ... }: {
    checks.pi-module-api = import ../../nix/check-support/pi-module-api.nix {
      den = self;
      inherit inputs pkgs;
    };
  };
}
