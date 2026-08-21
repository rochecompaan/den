{ self, ... }:
{
  flake.devenvModules.den = { config, lib, pkgs, ... }:
    let
      moduleOptions = import ../../nix/lib/module-options.nix { inherit lib pkgs; };
      claude = config.programs.den.claude;
    in
    {
      options = moduleOptions.options;
      config = lib.mkMerge [
        { assertions = moduleOptions.assertions claude; }
        (lib.mkIf claude.enable {
          packages = [
            (self.lib.${pkgs.system}.mkClaude (builtins.removeAttrs claude [ "enable" ]))
          ];
        })
      ];
    };
}
