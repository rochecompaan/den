{ inputs, pkgs }:

if pkgs.stdenv.isLinux then
  import ./pure-launcher-linux.nix { inherit inputs pkgs; }
else
  import ./pure-launcher-darwin.nix { inherit inputs pkgs; }
