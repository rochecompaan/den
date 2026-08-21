{ inputs, self, ... }:
{
  perSystem = { pkgs, self', ... }: {
    checks.package-api = import ../../nix/check-support/package-api.nix {
      den = self';
      darwinPackages = {
        aarch64 = self.packages.aarch64-darwin;
        x86_64 = self.packages.x86_64-darwin;
      };
      inherit inputs pkgs;
    };
  };
}
