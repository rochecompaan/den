{ inputs, ... }:

{
  perSystem = { pkgs, self', ... }: {
    checks.package-closure = import ../../nix/check-support/package-closure.nix {
      inherit inputs pkgs;
      claude = self'.packages.claude;
    };
  };
}
