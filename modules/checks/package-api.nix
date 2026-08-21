{ inputs, ... }:
{
  perSystem = { pkgs, self', ... }: {
    checks.package-api = import ../../nix/check-support/package-api.nix {
      den = self';
      inherit inputs pkgs;
    };
  };
}
