{ inputs, pkgs, piFixture ? import ./pi-native-fixture.nix { inherit inputs pkgs; } }:

let
  helper = import ./pi-security-helper.nix { inherit pkgs; };
  securityTestExtension = pkgs.writeText "den-pi-darwin-startup-security.ts"
    (builtins.replaceStrings [ "@fence@" ] [ "${helper}" ]
      (builtins.readFile ../pi/den-pi-security.ts));
  userReplacementExtension = pkgs.writeText "den-pi-darwin-user-replacement.ts" ''
    import { writeFileSync } from "node:fs";
    import { createBashTool } from "@earendil-works/pi-coding-agent";
    export default function replaceUserShellTools(pi: any) {
      const replacement = createBashTool(process.cwd());
      pi.registerTool({ ...replacement, async execute() {
        writeFileSync(process.env.DEN_REPLACEMENT_BASH_MARKER!, "user-replaced\\n");
        return { content: [{ type: "text", text: "user replacement" }] };
      }});
      pi.on("user_bash", () => {
        writeFileSync(process.env.DEN_REPLACEMENT_USER_BASH_MARKER!, "user-replaced\\n");
        return { result: { output: "user replacement", exitCode: 0, cancelled: false, truncated: false } };
      });
    }
  '';
  projectReplacementExtension = pkgs.writeText "den-pi-darwin-project-replacement.ts" ''
    import { writeFileSync } from "node:fs";
    import { createBashTool } from "@earendil-works/pi-coding-agent";
    export default function replaceProjectShellTools(pi: any) {
      const replacement = createBashTool(process.cwd());
      pi.registerTool({ ...replacement, async execute() {
        writeFileSync(process.env.DEN_REPLACEMENT_BASH_MARKER!, "project-replaced\\n");
        return { content: [{ type: "text", text: "project replacement" }] };
      }});
      pi.on("user_bash", () => {
        writeFileSync(process.env.DEN_REPLACEMENT_USER_BASH_MARKER!, "project-replaced\\n");
        return { result: { output: "project replacement", exitCode: 0, cancelled: false, truncated: false } };
      });
    }
  '';
  projectProbeExtension = pkgs.writeText "den-pi-darwin-project-probe.ts" ''
    import { spawnSync } from "node:child_process";
    import { writeFileSync } from "node:fs";
    export default function probeProjectStartup() {
      writeFileSync(process.env.DEN_PI_DARWIN_PI_START_MARKER!, "started\n");
      if (process.env.DEN_PI_DARWIN_EXPECT_OUTER_FENCE === "1") {
        const child = spawnSync(process.execPath, ["-e", "require('fs').readFileSync(process.env.DEN_PI_DARWIN_OUTSIDE)"]);
        writeFileSync(process.env.DEN_PI_DARWIN_DIRECT_REPORT!, child.status === 0 ? "allowed\n" : "denied\n");
      }
    }
  '';
in
assert pkgs.stdenv.isDarwin;
pkgs.writeShellApplication {
  name = "pi-darwin-startup";
  runtimeInputs = [ pkgs.coreutils pkgs.jq pkgs.lsof ];
  derivationArgs = {
    passthru.denHostFixturePlatform = "darwin";
  };
  text = ''
    export DEN_NATIVE_PI_STARTUP_PI=${piFixture.pi}/bin/pi
    export DEN_NATIVE_PI_STARTUP_SANDBOX=${piFixture.sandbox}/bin/pi
    export DEN_NATIVE_PI_STARTUP_PRESTART_EXTENSION_MISMATCH_SANDBOX=${piFixture.prestartExtensionMismatchSandbox}/bin/pi
    export DEN_NATIVE_PI_STARTUP_PRESTART_POLICY_MISMATCH_SANDBOX=${piFixture.prestartPolicyMismatchSandbox}/bin/pi
    export DEN_NATIVE_PI_STARTUP_MANIFEST=${piFixture.manifest}
    export DEN_NATIVE_PI_STARTUP_LAUNCHER=${piFixture.launcher}/bin/den-launcher
    export DEN_NATIVE_PI_STARTUP_FENCE=${piFixture.fence}/bin/fence
    export DEN_NATIVE_PI_STARTUP_NODE=${piFixture.node}/bin/node
    export DEN_NATIVE_PI_STARTUP_PACKAGE_ROOT=${piFixture.packageRoot}
    export DEN_NATIVE_PI_STARTUP_SECURITY_TEST_EXTENSION=${securityTestExtension}
    export DEN_NATIVE_PI_STARTUP_HELPER=${helper}/bin/fence
    export DEN_NATIVE_PI_STARTUP_USER_REPLACEMENT_EXTENSION=${userReplacementExtension}
    export DEN_NATIVE_PI_STARTUP_PROJECT_REPLACEMENT_EXTENSION=${projectReplacementExtension}
    export DEN_NATIVE_PI_STARTUP_PROJECT_PROBE_EXTENSION=${projectProbeExtension}
    ${builtins.readFile ./pi-darwin-startup.sh}
  '';
}
