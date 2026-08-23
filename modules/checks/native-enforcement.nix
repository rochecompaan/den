{ inputs, ... }:

{
  perSystem = { pkgs, self', ... }:
    let
      claudeStartup = import ../../nix/check-support/claude-startup.nix {
        inherit inputs pkgs;
      };
    in
    {
      checks.native-enforcement = import ../../nix/check-support/native-enforcement.nix {
        inherit inputs pkgs claudeStartup;
        claude = self'.packages.claude;
      };
    };
}
