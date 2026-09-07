{ inputs, pkgs }:

let
  mkPi = import ../lib/mk-pi.nix;
  pi = import ../packages/pi-coding-agent.nix { inherit pkgs; };
  resourceFixture = pkgs.runCommand "den-native-pi-resource-package" { } ''
    mkdir -p "$out"
    cat > "$out/package.json" <<'JSON'
    {"name":"fixture-package","pi":{"prompts":["package-prompt.md"]}}
    JSON
    printf '%s\n' 'Immutable package prompt fixture.' > "$out/package-prompt.md"
  '';
  sandbox = mkPi { inherit inputs pkgs; } {
    agentDir = null;
    sessionDir = null;
    extraPkgs = [ ];
    resources = {
      extensions = [
        ./fixtures/pi/native/report-extension.ts
        ./fixtures/pi/native/provider-extension.ts
        ./fixtures/pi/native/switch-extension.ts
      ];
      packages = [ resourceFixture ];
      skills = [ ./fixtures/pi/native/skill ];
      promptTemplates = [ ./fixtures/pi/native/prompt.md ];
      themes = [ ./fixtures/pi/native/theme.json ];
    };
    docker = { };
    podman = { };
  };
in
{
  inherit pi resourceFixture sandbox;
  manifest = sandbox.denManifest;
  packageRoot = pi.packageRoot;
}
