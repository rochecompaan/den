{ inputs, ... }:

{
  perSystem = { pkgs, self', ... }:
    let
      claudeStartup = import ../../nix/check-support/claude-startup.nix {
        inherit inputs pkgs;
      };
      piFixture = import ../../nix/check-support/pi-native-fixture.nix {
        inherit inputs pkgs;
      };
    in
    {
      checks.native-enforcement = import ../../nix/check-support/native-enforcement.nix {
        inherit inputs pkgs claudeStartup piFixture;
        claude = self'.packages.claude;
      };
    };
}
