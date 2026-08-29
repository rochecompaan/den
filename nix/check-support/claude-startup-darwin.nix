{ inputs, pkgs }:

let
  fakes = import ./fakes.nix { inherit inputs pkgs; };
  aclProbeDarwin = import ../packages/den-acl-probe.nix { inherit (pkgs) lib stdenv; };
  inheritedSandbox = fakes.mkSandbox { };
  explicitSandbox = fakes.mkSandbox {
    configDir = "/private/tmp/den-claude-startup-runtime-placeholder";
  };
in
pkgs.writeShellApplication {
  name = "claude-startup";
  runtimeInputs = [ pkgs.coreutils pkgs.gitMinimal pkgs.gnugrep pkgs.jq ];
  derivationArgs = {
    passthru.denHostFixturePlatform = "darwin";
  };
  text = ''
    export DEN_CLAUDE_STARTUP_INHERITED_MANIFEST=${inheritedSandbox.denManifest}
    export DEN_CLAUDE_STARTUP_EXPLICIT_MANIFEST=${explicitSandbox.denManifest}
    export DEN_CLAUDE_STARTUP_LAUNCHER=${fakes.launcher}/bin/den-launcher
    export DEN_CLAUDE_STARTUP_ACL_PROBE=${aclProbeDarwin}/bin/den-acl-probe
    ${builtins.readFile ./claude-startup-runtime-manifest.sh}
    ${builtins.readFile ./claude-startup-darwin.sh}
  '';
}
