{ inputs, ... }:

{
  perSystem = { pkgs, ... }: {
    checks.claude-startup = import ../../nix/check-support/claude-startup.nix {
      inherit inputs pkgs;
    };
  };
}
