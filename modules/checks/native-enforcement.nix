{ inputs, ... }:

{
  perSystem = { pkgs, self', ... }: {
    checks.native-enforcement = import ../../nix/check-support/native-enforcement.nix {
      inherit inputs pkgs;
      claude = self'.packages.claude;
    };
  };
}
