{ ... }:
{
  perSystem = { pkgs, ... }:
    let
      pluginInjection = import ../../nix/check-support/claude-plugin-injection.nix {
        inherit pkgs;
      };
    in
    {
      checks.claude-plugin-injection =
        if pkgs.stdenv.isDarwin then
          pluginInjection
        else
          pkgs.runCommand "claude-plugin-injection-check"
            {
              nativeBuildInputs = [ pluginInjection ];
            }
            ''
              claude-plugin-injection
              touch "$out"
            '';
    };
}
