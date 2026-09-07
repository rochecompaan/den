{ inputs, pkgs }:

let
  mkPi = import ../lib/mk-pi.nix;
  pi = import ../packages/pi-coding-agent.nix { inherit pkgs; };
  resourceFixture = pkgs.runCommand "den-native-pi-resource-package" { } ''
    mkdir -p "$out/extensions" "$out/skills/fixture-package-skill" "$out/prompts" "$out/themes"
    cat > "$out/package.json" <<'JSON'
    {"name":"fixture-package","pi":{"extensions":["./extensions/fixture-package-extension.ts"],"skills":["./skills"],"prompts":["./prompts"],"themes":["./themes"]}}
    JSON
    cat > "$out/extensions/fixture-package-extension.ts" <<'TS'
    import { appendFileSync } from "node:fs";
    import { join } from "node:path";
    export default function () {
      appendFileSync(join(process.env.PI_CODING_AGENT_DIR!, "pi-resources.report"), "package:fixture-package\n");
    }
    TS
    printf '%b\n' '---\nname: fixture-package-skill\ndescription: Immutable package skill fixture\n---\nPackage skill.' > "$out/skills/fixture-package-skill/SKILL.md"
    printf '%s\n' 'Immutable package prompt fixture.' > "$out/prompts/package-prompt.md"
    sed 's/fixture-theme/fixture-package-theme/' ${./fixtures/pi/native/theme.json} > "$out/themes/package-theme.json"
    mkdir -p "$out/skills/native-collision"
    sed 's/fixture-skill/native-collision/' ${./fixtures/pi/native/skill/SKILL.md} > "$out/skills/native-collision/SKILL.md"
    printf '%s\n' 'Package collision winner.' > "$out/prompts/native-collision.md"
    sed 's/fixture-theme/native-collision/' ${./fixtures/pi/native/theme.json} > "$out/themes/native-collision.json"
    cat > "$out/extensions/z-collision.ts" <<'TS'
    export default function (pi: any) {
      pi.registerTool({ name: "native-collision", label: "Collision", description: "Package loser", parameters: { type: "object", properties: {} }, execute: async () => ({ content: [] }) });
    }
    TS
  '';
  directCollisions = pkgs.runCommand "den-native-pi-direct-collisions" { outputs = [ "out" "prompts" "themes" ]; } ''
    mkdir -p "$out/native-collision" "$prompts" "$themes"
    sed 's/fixture-skill/native-collision/' ${./fixtures/pi/native/skill/SKILL.md} > "$out/native-collision/SKILL.md"
    printf '%s\n' 'Direct collision loser.' > "$prompts/native-collision.md"
    sed 's/fixture-theme/native-collision/' ${./fixtures/pi/native/theme.json} > "$themes/native-collision.json"
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
      skills = [ ./fixtures/pi/native/skill directCollisions ];
      promptTemplates = [ ./fixtures/pi/native/prompt.md directCollisions.prompts ];
      themes = [ ./fixtures/pi/native/theme.json directCollisions.themes ];
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
