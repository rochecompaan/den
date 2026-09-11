{ inputs, ... }:
{
  perSystem = { pkgs, ... }:
    let
      mkAgentSandbox = import ../../nix/lib/mk-agent-sandbox.nix {
        inherit inputs pkgs;
      };
      mkPi = import ../../nix/lib/mk-pi.nix {
        inherit inputs pkgs mkAgentSandbox;
      };
    in
    {
      lib.mkPi = mkPi;
    };
}
