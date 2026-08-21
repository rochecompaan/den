{ inputs, self, ... }:
{
  perSystem = { pkgs, self', ... }: {
    checks.module-api = import ../../nix/check-support/module-api.nix {
      den = self;
      inherit inputs pkgs;
    };
  };
}
