{ ... }:

{
  perSystem = { pkgs, ... }: {
    checks.network-options = import ../../nix/check-support/network-options.nix { inherit pkgs; };
  };
}
