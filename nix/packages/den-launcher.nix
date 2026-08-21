{ pkgs }:

pkgs.buildGoModule {
  pname = "den-launcher";
  version = "0.1.0";
  src = ../..;

  vendorHash = null;
  env.CGO_ENABLED = 0;
  subPackages = [ "cmd/den-launcher" ];
}
