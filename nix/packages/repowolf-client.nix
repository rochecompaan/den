{ inputs, pkgs }:

(pkgs.callPackage "${inputs.repowolf}/nix/package-client.nix" { }).overrideAttrs (old: {
  __darwinAllowLocalNetworking = pkgs.stdenv.isDarwin;
  meta = old.meta // {
    platforms = pkgs.lib.platforms.unix;
  };
})
