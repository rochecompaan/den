{ inputs, pkgs }:

let
  mkPi = import ../lib/mk-pi.nix;
  fence = (import ../lib/fence.nix { inherit pkgs; }).package;
  extraPackage = pkgs.writeShellApplication {
    name = "den9-extra-present";
    text = ''
      printf 'extra-package\n'
    '';
  };
  forbiddenPiExtraPackage = pkgs.runCommand "den9-pi-shadow" { } ''
    mkdir -p "$out/bin"
    cat > "$out/bin/pi" <<'EOF'
    #!${pkgs.bash}/bin/bash
    printf 'shadow-pi\n'
    EOF
    chmod 0555 "$out/bin/pi"
  '';
  fenceInputRecorder = pkgs.writeShellScript "den-native-pi-fence-input-recorder" ''
    if [ -n "''${DEN_NATIVE_PI_FENCE_INPUT_REPORT-}" ]; then
      {
        printf 'HTTP_PROXY=%s\n' "''${HTTP_PROXY-}"
        printf 'HTTPS_PROXY=%s\n' "''${HTTPS_PROXY-}"
        printf 'ALL_PROXY=%s\n' "''${ALL_PROXY-}"
        printf 'NO_PROXY=%s\n' "''${NO_PROXY-}"
      } > "$DEN_NATIVE_PI_FENCE_INPUT_REPORT"
    fi
    if [ -n "''${DEN_NATIVE_PI_FENCE_POLICY_REPORT-}" ] && [ -n "''${DEN_FENCE_POLICY_FILE-}" ]; then
      ${pkgs.coreutils}/bin/cp -- "$DEN_FENCE_POLICY_FILE" "$DEN_NATIVE_PI_FENCE_POLICY_REPORT"
    fi
    exec ${fence}/bin/fence "$@"
  '';
  # Only this native fixture substitutes account-home discovery. The production
  # source and launcher derivation stay unchanged; drift fails the fixture build.
  launcher = (import ../packages/den-launcher.nix { inherit pkgs; }).overrideAttrs (old: {
    pname = "den-native-pi-launcher";
    nativeBuildInputs = (old.nativeBuildInputs or [ ]) ++ [ pkgs.python3 ];
    postPatch = (old.postPatch or "") + ''
      python3 - <<'PYTHON'
      from pathlib import Path
      path = Path("internal/launch/launch.go")
      source = path.read_text()
      original = "func invokingAccountHome() (string, error) {\n\taccount, err := user.Current()\n\tif err != nil || account == nil {\n\t\treturn \"\", err\n\t}\n\treturn account.HomeDir, nil\n}"
      replacement = "func invokingAccountHome() (string, error) {\n\thome := os.Getenv(\"DEN_NATIVE_INVOKING_HOME\")\n\tif !filepath.IsAbs(home) {\n\t\treturn \"\", fmt.Errorf(\"native fixture requires an absolute invoking home\")\n\t}\n\treturn home, nil\n}"
      account_import = '\t"os/user"\n'
      if source.count(original) != 1 or source.count(account_import) != 1:
          raise SystemExit("native fixture account-home substitution drifted")
      path.write_text(source.replace(original, replacement).replace(account_import, ""))
      PYTHON
    '';
  });
  mkFixtureSandbox = args: (import ../lib/mk-agent-sandbox.nix { inherit inputs pkgs; }) (args // {
    dependencies = {
      inherit launcher;
      inherit fence;
      repoWolfClient = import ../packages/repowolf-client.nix { inherit inputs pkgs; };
      git = pkgs.gitMinimal;
      bash = pkgs.bash;
      coreutils = pkgs.coreutils;
    } // pkgs.lib.optionalAttrs pkgs.stdenv.isLinux { acl = pkgs.acl; }
      // pkgs.lib.optionalAttrs pkgs.stdenv.isDarwin {
        aclProbeDarwin = import ../packages/den-acl-probe.nix { inherit (pkgs) lib stdenv; };
      };
  });
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
    mkdir -p "$out/collision-package/extensions"
    printf '%s\n' '{"name":"fixture-collision-package","pi":{"extensions":["./extensions/z-collision.ts"]}}' > "$out/collision-package/package.json"
    cat > "$out/collision-package/extensions/z-collision.ts" <<'TS'
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
  nativeResources = {
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
  prestartMismatchExtension = pkgs.writeText "den-pi-prestart-mismatch.ts" ''
    import { writeFileSync } from "node:fs";
    export default function () {
      writeFileSync(process.env.DEN_PI_DARWIN_PI_START_MARKER!, "started\n");
    }
  '';
  prestartExtensionMismatchTestInputs = {
    adapterExtension = prestartMismatchExtension;
  };
  prestartPolicyMismatchTestInputs = {
    policyIdentity = "fixture-policy-identity-mismatch";
  };
  prestartMismatchAdapter = (import ../lib/mk-pi.nix {
    inherit inputs pkgs;
    isDarwin = true;
    mkAgentSandbox = value: value;
    darwinSecurityTestInputs = prestartExtensionMismatchTestInputs;
  }) {
    agentDir = null;
    sessionDir = null;
    extraPkgs = [ ];
    resources = { };
    docker = { };
    podman = { };
  };
  forbiddenPiExtraPackageCheck = pkgs.testers.testBuildFailure ((import ../lib/pi-resources.nix { inherit pkgs; }) {
    resources = nativeResources;
    extraPkgs = [ forbiddenPiExtraPackage ];
  }).diagnosticsCheck;
  sandbox = mkPi { inherit inputs pkgs; mkAgentSandbox = mkFixtureSandbox; } {
    agentDir = null;
    sessionDir = null;
    extraPkgs = [ pkgs.curl pkgs.openssh extraPackage forbiddenPiExtraPackageCheck ];
    resources = nativeResources;
    docker = { };
    podman = { };
  };
  prestartExtensionMismatchSandbox = (import ../lib/mk-pi.nix {
    inherit inputs pkgs;
    mkAgentSandbox = mkFixtureSandbox;
    darwinSecurityTestInputs = prestartExtensionMismatchTestInputs;
  }) {
    agentDir = null;
    sessionDir = null;
    extraPkgs = [ pkgs.curl pkgs.openssh extraPackage forbiddenPiExtraPackageCheck ];
    resources = nativeResources;
    docker = { };
    podman = { };
  };
  prestartPolicyMismatchSandbox = (import ../lib/mk-pi.nix {
    inherit inputs pkgs;
    mkAgentSandbox = mkFixtureSandbox;
    darwinSecurityTestInputs = prestartPolicyMismatchTestInputs;
  }) {
    agentDir = null;
    sessionDir = null;
    extraPkgs = [ pkgs.curl pkgs.openssh extraPackage forbiddenPiExtraPackageCheck ];
    resources = nativeResources;
    docker = { };
    podman = { };
  };
in
assert prestartMismatchAdapter.adapter.agent.securityAdapter.path == prestartMismatchExtension;
{
  inherit pi resourceFixture sandbox prestartExtensionMismatchSandbox prestartPolicyMismatchSandbox launcher fence fenceInputRecorder forbiddenPiExtraPackageCheck;
  node = pi.nodejs;
  manifest = sandbox.denManifest;
  packageRoot = pi.packageRoot;
}
