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
      piDarwinStartup = if pkgs.stdenv.isDarwin then
        import ../../nix/check-support/pi-darwin-startup.nix { inherit inputs pkgs piFixture; }
      else null;
    in
    {
      checks.native-enforcement = import ../../nix/check-support/native-enforcement.nix {
        inherit inputs pkgs claudeStartup piFixture piDarwinStartup;
        claude = self'.packages.claude;
      };
    };
}
