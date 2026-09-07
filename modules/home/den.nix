{ self, ... }:
{
  flake.homeModules.den = { config, lib, pkgs, ... }:
    let
      moduleOptions = import ../../nix/lib/module-options.nix { inherit lib pkgs; };
      claude = config.programs.den.claude;
      pi = config.programs.den.pi;
    in
    {
      options = moduleOptions.options;
      config = lib.mkMerge [
        { assertions = moduleOptions.assertions claude ++ moduleOptions.piAssertions pi; }
        (lib.mkIf claude.enable {
          home.packages = [
            (self.lib.${pkgs.system}.mkClaude (builtins.removeAttrs claude [ "enable" ]))
          ];
        })
        (lib.mkIf pi.enable {
          home.packages = [
            (self.lib.${pkgs.system}.mkPi (builtins.removeAttrs pi [ "enable" ]))
          ];
        })
      ];
    };
}
