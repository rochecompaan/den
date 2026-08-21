{ inputs, pkgs }:

(pkgs.callPackage "${inputs.repowolf}/nix/package-client.nix" { }).overrideAttrs (old: {
  version = "0.1.0";
  meta = old.meta // {
    platforms = pkgs.lib.platforms.unix;
  };
})
