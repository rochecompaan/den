{ inputs, pkgs }:

let
  lib = pkgs.lib;
  mkPi = import ../lib/mk-pi.nix;
  options = import ../lib/pi-options.nix { inherit pkgs; };
  resourcePackage = name: resourceName: pkgs.runCommand name { } ''
    mkdir -p "$out/prompts" "$out/node_modules/runtime"
    printf '{"name":"%s","pi":{"prompts":["prompts/collision.md"]},"dependencies":{"runtime":"1"}}\n' ${lib.escapeShellArg resourceName} > "$out/package.json"
    printf '{"name":"runtime","version":"1"}\n' > "$out/node_modules/runtime/package.json"
    printf '%s prompt\n' ${lib.escapeShellArg resourceName} > "$out/prompts/collision.md"
  '';
  resourcePackageFirst = resourcePackage "pi-configured-resource-package-first" "first";
  resourcePackageSecond = resourcePackage "pi-configured-resource-package-second" "second";
  resources = {
    extensions = [ ./fixtures/pi/resources/extensions/first.ts ./fixtures/pi/resources/extensions/second.ts ];
    packages = [ resourcePackageFirst resourcePackageSecond ];
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
  storeStringPrompt = "${resourcePackageFirst}/prompts/collision.md";
  # Reachable only through a store-path string, so the closure check proves the
  # string context carries the package into the Fence closure.
  stringOnlyPackage = pkgs.runCommand "pi-string-only-resource" { } ''
    mkdir -p "$out/prompts"
    printf 'string prompt\n' > "$out/prompts/string.md"
  '';
  stringOnlyPi = mkPi { inherit inputs pkgs; } {
    resources.promptTemplates = [ "${stringOnlyPackage}/prompts/string.md" ];
  };
  piPackage = import ../packages/pi-coding-agent.nix { inherit pkgs; };
  resourceCollisions = pkgs.runCommand "pi-resource-collisions"
    {
      nativeBuildInputs = [ pkgs.coreutils ];
      manifest = pi.denManifest;
      node = "${piPackage.nodejs}/bin/node";
      resourceLoader = "${piPackage.packageRoot}/dist/core/resource-loader.js";
      parseArgs = "${piPackage.packageRoot}/dist/cli/args.js";
      promptFirst = "${resourcePackageFirst}/prompts/collision.md";
      promptSecond = "${resourcePackageSecond}/prompts/collision.md";
    }
    ''
      set -euo pipefail
      mkdir -p "$TMPDIR/home" "$TMPDIR/agent" "$TMPDIR/workspace"
      export HOME="$TMPDIR/home"
      export PI_RESOURCE_MANIFEST="$manifest"
      cat > collision-check.mjs <<'EOF'
      import { readFileSync, realpathSync } from "node:fs";

      const { parseArgs } = await import(process.env.parseArgs);
      const { DefaultResourceLoader } = await import(process.env.resourceLoader);
      const assert = (condition, message) => {
        if (!condition) throw new Error(message);
      };
      const canonical = (path) => realpathSync(path);
      const resourceArgs = JSON.parse(readFileSync(process.env.PI_RESOURCE_MANIFEST, "utf8")).agent.resourceArgs;
      const args = parseArgs(resourceArgs);
      const expected = {
        extensionFirst: canonical(args.extensions[0]),
        extensionSecond: canonical(args.extensions[1]),
        skillFirst: canonical(args.skills[0] + "/SKILL.md"),
        skillSecond: canonical(args.skills[1] + "/SKILL.md"),
        promptFirst: canonical(process.env.promptFirst),
        promptSecond: canonical(process.env.promptSecond),
        themeFirst: canonical(args.themes[0]),
        themeSecond: canonical(args.themes[1]),
      };
      const loader = new DefaultResourceLoader({
        cwd: process.env.workspace,
        agentDir: process.env.agentDir,
        additionalExtensionPaths: args.extensions,
        additionalSkillPaths: args.skills,
        additionalPromptTemplatePaths: args.promptTemplates,
        additionalThemePaths: args.themes,
      });
      await loader.reload();

      const extensionError = loader.getExtensions().errors.find((diagnostic) =>
        canonical(diagnostic.path) === expected.extensionSecond &&
        diagnostic.error === "Tool \"den-collision\" conflicts with " + expected.extensionFirst,
      );
      assert(extensionError, "extension collision did not retain the first winner and report the second loser");

      const assertCollision = (type, name, result, winner, loser) => {
        const diagnostic = result.diagnostics.find((entry) => entry.collision?.resourceType === type && entry.collision.name === name);
        assert(diagnostic, type + " collision diagnostic is absent");
        assert(canonical(diagnostic.collision.winnerPath) === winner, type + " winner is not the first configured resource: " + JSON.stringify(diagnostic));
        assert(canonical(diagnostic.collision.loserPath) === loser, type + " loser is not the second configured resource: " + JSON.stringify(diagnostic));
      };
      assertCollision("skill", "den-collision", loader.getSkills(), expected.skillFirst, expected.skillSecond);
      assertCollision("prompt", "collision", loader.getPrompts(), expected.promptFirst, expected.promptSecond);
      assertCollision("theme", "den-collision", loader.getThemes(), expected.themeFirst, expected.themeSecond);
      EOF
      workspace="$TMPDIR/workspace" agentDir="$TMPDIR/agent" \
        promptFirst="$promptFirst" promptSecond="$promptSecond" \
        parseArgs="$parseArgs" resourceLoader="$resourceLoader" \
        "$node" collision-check.mjs
      touch "$out"
    '';
in
assert lib.assertMsg (invalid (options { unknown = true; })) "Pi options accepted an unknown key";
assert lib.assertMsg (invalid (options { agentDir = "relative"; })) "Pi options accepted a relative agent directory";
assert lib.assertMsg (invalid (options { sessionDir = "relative"; })) "Pi options accepted a relative session directory";
assert lib.assertMsg (invalid (options { resources.extensions = [ "/tmp/mutable.ts" ]; }).resources) "Pi options accepted a mutable resource string";
assert lib.assertMsg ((options { resources.promptTemplates = [ storeStringPrompt ]; }).resources.promptTemplates == [ storeStringPrompt ]) "Pi options rejected a store path string";
assert lib.assertMsg (invalid (options { resources.promptTemplates = [ (builtins.unsafeDiscardStringContext storeStringPrompt) ]; }).resources) "Pi options accepted a store path string without context";
assert lib.assertMsg (invalid (options { resources.promptTemplates = [ "${resourcePackageFirst}/../escape.md" ]; }).resources) "Pi options accepted a store path string with a parent component";
assert lib.assertMsg (configured.resources.extensions == resources.extensions) "Pi options changed extension order";
pkgs.runCommand "pi-resources"
  {
    nativeBuildInputs = [ pkgs.jq ];
    resourceDiagnostics = pi.resourceDiagnostics;
    stringOnlyDiagnostics = stringOnlyPi.resourceDiagnostics;
    stringOnlyManifest = stringOnlyPi.denManifest;
    inherit resourceCollisions stringOnlyPackage;
  }
  ''
    test -e "$resourceDiagnostics"
    test -e "$resourceCollisions"
    test -e "$stringOnlyDiagnostics"
    grep -Fqx "$stringOnlyPackage" "$(jq -r .closurePathsFile "$stringOnlyManifest")"
    touch "$out"
  ''
