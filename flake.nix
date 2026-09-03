{
  description = "Den Claude sandbox";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/c94da05fe469a845461ae503894fad568abeb2a6";

    flake-parts.url = "github:hercules-ci/flake-parts";

    import-tree.url = "github:vic/import-tree";

    repowolf.url = "github:rochecompaan/repowolf";

    home-manager.url = "github:nix-community/home-manager";
    home-manager.inputs.nixpkgs.follows = "nixpkgs";

    devenv.url = "github:cachix/devenv";
    devenv.inputs.nixpkgs.follows = "nixpkgs";
  };

  outputs = inputs:
    inputs.flake-parts.lib.mkFlake { inherit inputs; }
      (inputs.import-tree ./modules);
}
