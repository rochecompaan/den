{ lib, flake-parts-lib, inputs, ... }:
let
  inherit (lib) mkOption types;
in
{
  imports = [
    (flake-parts-lib.mkTransposedPerSystemModule {
      name = "lib";
      file = ./claude.nix;
      option = mkOption {
        type = types.lazyAttrsOf types.raw;
        default = { };
        description = "Per-system Den library helpers.";
      };
    })
  ];

  perSystem = { pkgs, ... }:
    let
      fence = (import ../../nix/lib/fence.nix { inherit pkgs; }).package;
      mkAgentSandbox = import ../../nix/lib/mk-agent-sandbox.nix {
        inherit inputs pkgs;
      };
      mkClaude = import ../../nix/lib/mk-claude.nix {
        inherit pkgs fence mkAgentSandbox;
      };
    in
    {
      lib.mkClaude = mkClaude;
    };
}
