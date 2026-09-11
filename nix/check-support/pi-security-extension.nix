{ pkgs }:

let
  piPackage = import ../packages/pi-coding-agent.nix { inherit pkgs; };
  fenceInfo = import ../lib/fence.nix { inherit pkgs; };
  securityHelper = import ./pi-security-helper.nix { inherit pkgs; };
  renderExtension = fence: name: pkgs.writeText name
    (builtins.replaceStrings [ "@fence@" ] [ fence ] (builtins.readFile ../pi/den-pi-security.ts));
  securityExtension = renderExtension "${securityHelper}" "den-pi-security.ts";
  missingFenceExtension = renderExtension "/nix/store/00000000000000000000000000000000-missing-fence" "den-pi-security-missing-fence.ts";
  replacementExtension = ./fixtures/pi/replace-shell-tools.ts;
  extensionCheck = pkgs.runCommand "pi-security-extension-behavior"
    {
      nativeBuildInputs = [ pkgs.coreutils ];
      node = "${piPackage.nodejs}/bin/node";
      resourceLoader = "${piPackage.packageRoot}/dist/core/resource-loader.js";
      extensionRunner = "${piPackage.packageRoot}/dist/core/extensions/runner.js";
      inherit securityExtension missingFenceExtension replacementExtension;
    }
    ''
      set -euo pipefail
      mkdir -p "$TMPDIR/home" "$TMPDIR/agent" "$TMPDIR/workspace" "$TMPDIR/session"
      export HOME="$TMPDIR/home"
      export PI_CODING_AGENT_DIR="$TMPDIR/agent"
      export DEN_FENCE_POLICY_FILE="$TMPDIR/policy.json"
      export DEN_PI_DARWIN_HELPER_REQUEST="$TMPDIR/request.json"
      export DEN_PI_DARWIN_HELPER_LISTENER_REPORT="$TMPDIR/helper-listener.report"
      export DEN_REPLACEMENT_BASH_MARKER="$TMPDIR/replacement-bash"
      export DEN_REPLACEMENT_USER_BASH_MARKER="$TMPDIR/replacement-user-bash"
      printf '{}\n' > "$DEN_FENCE_POLICY_FILE"
      cat > check.mjs <<'EOF'
      import { readFileSync } from "node:fs";
      const { DefaultResourceLoader } = await import(process.env.resourceLoader);
      const { ExtensionRunner } = await import(process.env.extensionRunner);
      const assert = (condition, message) => { if (!condition) throw new Error(message); };
      const load = async (paths) => {
        const loader = new DefaultResourceLoader({
          cwd: process.env.workspace,
          agentDir: process.env.agentDir,
          additionalExtensionPaths: paths,
          noExtensions: true,
        });
        await loader.reload();
        const loaded = loader.getExtensions();
        assert(loaded.extensions.length === paths.length, "not all extension sources loaded: " + JSON.stringify(loaded.errors));
        return new ExtensionRunner(loaded.extensions, loaded.runtime, process.env.workspace, {}, {});
      };
      const context = (cwd) => ({
        cwd,
        model: undefined,
        thinkingLevel: undefined,
        sessionManager: { getSessionId: () => "session", getSessionFile: () => undefined },
      });
      const expectReject = async (operation, name) => {
        let rejected = false;
        try { await operation(); } catch { rejected = true; }
        assert(rejected, name + " did not fail closed");
      };

      process.env.FENCE_SANDBOX = "1";
      process.env.DEN_PI_DARWIN_HELPER_MODE = "allow";
      const runner = await load([process.env.securityExtension, process.env.replacementExtension]);
      const bash = runner.getToolDefinition("bash");
      assert(bash, "security extension did not own bash");
      const bashCwd = process.env.workspace + "/bash-cwd";
      await import("node:fs").then(({ mkdirSync }) => mkdirSync(bashCwd));
      await bash.execute("tool-call", { command: "printf secure > bash-ran" }, undefined, undefined, context(bashCwd));
      assert(readFileSync(bashCwd + "/bash-ran", "utf8") === "secure", "secured bash did not execute in ctx.cwd");
      let request = JSON.parse(readFileSync(process.env.DEN_PI_DARWIN_HELPER_REQUEST, "utf8"));
      assert(request.hook_event_name === "PreToolUse", "wrong hook event");
      assert(request.tool_name === "Bash", "wrong tool name");
      assert(request.tool_input.command === "printf secure > bash-ran", "wrong command request");
      assert(request.tool_input.cwd === bashCwd && request.cwd === bashCwd, "current directory missing from request");
      assert(!readFileSync(process.env.securityExtension, "utf8").includes(" -c "), "security extension starts a nested Fence manager");

      for (const mode of ["deny", "rewrite", "malformed", "failed"]) {
        process.env.DEN_PI_DARWIN_HELPER_MODE = mode;
        await expectReject(() => bash.execute("tool-call", { command: mode }, undefined, undefined, context(bashCwd)), mode);
      }
      delete process.env.FENCE_SANDBOX;
      await expectReject(() => bash.execute("tool-call", { command: "missing-sandbox" }, undefined, undefined, context(bashCwd)), "missing FENCE_SANDBOX");
      process.env.FENCE_SANDBOX = "1";
      process.env.DEN_PI_DARWIN_HELPER_MODE = "allow";

      const user = await runner.emitUserBash({ type: "user_bash", command: "printf user-secure", cwd: process.env.workspace, excludeFromContext: false });
      assert(user?.operations, "security extension did not own user_bash");
      let userOutput = "";
      const userResult = await user.operations.exec("printf user-secure", process.env.workspace, { onData: (data) => { userOutput += data.toString(); } });
      assert(userResult.exitCode === 0 && userOutput === "user-secure", "secured user_bash did not delegate unchanged");
      const { existsSync } = await import("node:fs");
      assert(!existsSync(process.env.DEN_REPLACEMENT_BASH_MARKER), "later bash replacement won");
      assert(!existsSync(process.env.DEN_REPLACEMENT_USER_BASH_MARKER), "later user_bash replacement won");

      const { appendFileSync } = await import("node:fs");
      appendFileSync(process.env.DEN_FENCE_POLICY_FILE, "changed\n");
      await expectReject(() => bash.execute("tool-call", { command: "changed-policy" }, undefined, undefined, context(bashCwd)), "policy identity change");
      await expectReject(() => load([process.env.missingFenceExtension]), "spawn failure");
      EOF
      workspace="$TMPDIR/workspace" agentDir="$TMPDIR/agent" \
        resourceLoader="$resourceLoader" extensionRunner="$extensionRunner" \
        securityExtension="$securityExtension" missingFenceExtension="$missingFenceExtension" \
        replacementExtension="$replacementExtension" "$node" check.mjs
      test "$(<"$DEN_PI_DARWIN_HELPER_LISTENER_REPORT")" = no-listener
      touch "$out"
    '';
in
assert fenceInfo.version == "0.1.58";
assert fenceInfo.sourceHash == "sha256-ACe3N4bXYJW6QDQHtRChFWOTXTZTbEUbZ4d8cuFRqMY=";
assert fenceInfo.patchHash == "4be4f0266a0a79da10002893752ea8185915f6ecfb146513946bde8a96e41e2a";
assert fenceInfo.capabilities.claudePreToolUse;
assert fenceInfo.capabilities.denFenceTmpdir;
assert fenceInfo.capabilities.strictDenyRead;
assert if pkgs.stdenv.isDarwin then fenceInfo.capabilities.allowUnixSockets else fenceInfo.capabilities.argvRuntimePolicy;
pkgs.runCommand "pi-security-extension" { inherit extensionCheck; } ''
  test -e "$extensionCheck"
  touch "$out"
''
