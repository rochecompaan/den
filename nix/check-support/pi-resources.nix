{ inputs, pkgs }:

let
  lib = pkgs.lib;
  mkPi = import ../lib/mk-pi.nix;
  options = import ../lib/pi-options.nix { inherit pkgs; };
  resourcePackage = pkgs.runCommand "pi-configured-resource-package" { } ''
    mkdir -p "$out/extensions" "$out/skills/package" "$out/prompts" "$out/themes" "$out/node_modules/runtime"
    printf '{"name":"resource-package","pi":{"extensions":["extensions/package.ts"]},"dependencies":{"runtime":"1"}}\n' > "$out/package.json"
    printf '{"name":"runtime","version":"1"}\n' > "$out/node_modules/runtime/package.json"
    printf 'export default function configuredPackageExtension() {}\n' > "$out/extensions/package.ts"
    printf '%s\n' '---' 'name: package' '---' 'package skill' > "$out/skills/package/SKILL.md"
    printf 'package prompt\n' > "$out/prompts/package.md"
    printf '{"name":"package","colors":{}}\n' > "$out/themes/package.json"
  '';
  resources = {
    extensions = [ ./fixtures/pi/resources/extensions/first.ts ./fixtures/pi/resources/extensions/second.ts ];
    packages = [ resourcePackage ];
    skills = [ ./fixtures/pi/resources/skills/first ./fixtures/pi/resources/skills/second ];
    promptTemplates = [ ./fixtures/pi/resources/prompts/first.md ./fixtures/pi/resources/prompts/second.md ];
    themes = [ ./fixtures/pi/resources/themes/first.json ./fixtures/pi/resources/themes/second.json ];
  };
  configured = options {
    agentDir = null;
    sessionDir = null;
    extraPkgs = [ ];
    inherit resources;
    docker = { };
    podman = { };
  };
  pi = mkPi { inherit inputs pkgs; } {
    agentDir = null;
    sessionDir = null;
    extraPkgs = [ ];
    inherit resources;
    docker = { };
    podman = { };
  };
  invalid = value: !(builtins.tryEval value).success;
in
assert lib.assertMsg (invalid (options { unknown = true; })) "Pi options accepted an unknown key";
assert lib.assertMsg (invalid (options { agentDir = "relative"; })) "Pi options accepted a relative agent directory";
assert lib.assertMsg (invalid (options { sessionDir = "relative"; })) "Pi options accepted a relative session directory";
assert lib.assertMsg (invalid (options { resources.extensions = [ "/tmp/mutable.ts" ]; }).resources) "Pi options accepted a mutable resource string";
assert lib.assertMsg (configured.resources.extensions == resources.extensions) "Pi options changed extension order";
pi.resourceDiagnostics
