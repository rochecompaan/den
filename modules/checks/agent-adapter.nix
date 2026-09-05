{ inputs, ... }:
{
  perSystem = { pkgs, ... }: {
    checks.agent-adapter = import ../../nix/check-support/agent-adapter.nix {
      inherit inputs pkgs;
    };
  };
}
