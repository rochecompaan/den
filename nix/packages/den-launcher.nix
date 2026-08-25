{ pkgs }:

let
  vendorHash = import ../lib/den-go-vendor-hash.nix { lib = pkgs.lib; };
in
pkgs.buildGoModule {
  pname = "den-launcher";
  version = "0.1.0";
  src = ../..;

  inherit vendorHash;
  env.CGO_ENABLED = 0;
  subPackages = [ "cmd/den-launcher" ];
}
