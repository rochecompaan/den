{ inputs, ... }:

{
  perSystem = { pkgs, ... }: {
    checks.pure-launcher = import ../../nix/check-support/pure-launcher.nix {
      inherit inputs pkgs;
    };
  };
}
