{ inputs, pkgs }:

if pkgs.stdenv.isLinux then
  import ./claude-startup-linux.nix { inherit inputs pkgs; }
else
  import ./claude-startup-darwin.nix { inherit inputs pkgs; }
