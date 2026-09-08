{ inputs, pkgs, piFixture ? import ./pi-native-fixture.nix { inherit inputs pkgs; } }:

assert pkgs.stdenv.isDarwin;
pkgs.writeShellApplication {
  name = "pi-darwin-startup";
  runtimeInputs = [ pkgs.coreutils pkgs.jq ];
  derivationArgs = {
    passthru.denHostFixturePlatform = "darwin";
  };
  text = ''
    export DEN_NATIVE_PI_STARTUP_PI=${piFixture.pi}/bin/pi
    export DEN_NATIVE_PI_STARTUP_SANDBOX=${piFixture.sandbox}/bin/pi
    export DEN_NATIVE_PI_STARTUP_MANIFEST=${piFixture.manifest}
    export DEN_NATIVE_PI_STARTUP_LAUNCHER=${piFixture.launcher}/bin/den-launcher
    export DEN_NATIVE_PI_STARTUP_FENCE=${piFixture.fence}/bin/fence
    ${builtins.readFile ./pi-darwin-startup.sh}
  '';
}
