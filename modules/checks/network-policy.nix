{ self, ... }:

{
  perSystem = { pkgs, ... }: {
    checks.network-options = import ../../nix/check-support/network-options.nix { inherit pkgs; };
    checks.network-policy = import ../../nix/check-support/network-policy.nix { inherit pkgs; den = self; };
  };
}
