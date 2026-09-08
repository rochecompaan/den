{ inputs, pkgs, mkAgentSandbox ? import ./mk-agent-sandbox.nix { inherit inputs pkgs; }, isDarwin ? pkgs.stdenv.isDarwin }:

args@{ agentDir ? null, sessionDir ? null, extraPkgs ? [ ], resources ? { }, docker ? { }, podman ? { }, ... }:
let
  lib = pkgs.lib;
  options = import ./pi-options.nix { inherit pkgs; } args;
  pi = import ../packages/pi-coding-agent.nix { inherit pkgs; };
  fenceInfo = import ./fence.nix { inherit pkgs; };
  fence = fenceInfo.package;
  normalizedResources = import ./pi-resources.nix { inherit pkgs; } {
    inherit (options) resources extraPkgs;
  };
  securityExtension = pkgs.writeText "den-pi-security.ts"
    (builtins.replaceStrings [ "@fence@" ] [ "${fence}" ] (builtins.readFile ../pi/den-pi-security.ts));
  piAgentSource = builtins.replaceStrings [ "@pi@" ] [ "${pi}" ] (builtins.readFile ../pi/den-pi-agent.sh);
  darwinInputValidation = lib.optionalString isDarwin ''
    validate_security_input() {
      path=$1
      if [ ! -f "$path" ] || [ -L "$path" ] || [ "$(${pkgs.coreutils}/bin/readlink -f "$path")" != "$path" ]; then
        echo "Pi command security input is invalid" >&2
        exit 1
      fi
      ${pkgs.coreutils}/bin/stat -c '%d:%i:%s:%Y' "$path"
    }
    : "''${DEN_FENCE_POLICY_FILE:?Pi command security policy is unavailable}"
    extension_identity=$(validate_security_input ${securityExtension})
    fence_identity=$(validate_security_input ${fence}/bin/fence)
    policy_identity=$(validate_security_input "$DEN_FENCE_POLICY_FILE")
    revalidate_security_input() {
      expected=$1
      path=$2
      if [ "$expected" != "$(validate_security_input "$path")" ]; then
        echo "Pi command security input changed" >&2
        exit 1
      fi
    }
    revalidate_security_input "$extension_identity" ${securityExtension}
    revalidate_security_input "$fence_identity" ${fence}/bin/fence
    revalidate_security_input "$policy_identity" "$DEN_FENCE_POLICY_FILE"
  '';
  piAgent = pkgs.writeShellScript "den-pi-agent" (darwinInputValidation + piAgentSource);
in
assert lib.assertMsg (pi.version == "0.84.4") "Den requires Pi 0.84.4; refusing unknown version ${pi.version}";
assert lib.assertMsg (pi.actualPatchHash == builtins.convertHash { hash = pi.patchHash; toHashFormat = "base16"; })
  "Pi hardening patch hash drifted";
assert lib.assertMsg (fenceInfo.version == "0.1.58" &&
  fenceInfo.sourceHash == "sha256-ACe3N4bXYJW6QDQHtRChFWOTXTZTbEUbZ4d8cuFRqMY=" &&
  fenceInfo.patchHash == "4be4f0266a0a79da10002893752ea8185915f6ecfb146513946bde8a96e41e2a")
  "Pi requires Den's pinned Fence 0.1.58";
assert lib.assertMsg (fenceInfo.capabilities.claudePreToolUse &&
  fenceInfo.capabilities.denFenceTmpdir && fenceInfo.capabilities.strictDenyRead &&
  (if isDarwin then fenceInfo.capabilities.allowUnixSockets else fenceInfo.capabilities.linuxReadOnlyDenyReadMasks && fenceInfo.capabilities.argvRuntimePolicy))
  "Fence lacks mandatory Pi security capabilities";
mkAgentSandbox {
  inherit (options) extraPkgs docker podman;
  configDir = null;
  adapter = {
    output = {
      packageName = "pi";
      commandName = "pi";
      manifestName = "pi-manifest.json";
      mainProgram = "pi";
    };
    runtimePackages = [ ];
    closureOnlyPackages = [ pi piAgent ] ++ lib.optional isDarwin securityExtension ++ normalizedResources.closureInputs;
    protectedPathPatterns = [ "~/.pi/agent" "~/.agents" "~/.agents/skills" ];
    passthru = { resourceDiagnostics = normalizedResources.diagnosticsCheck; };
    agent = {
      name = "pi";
      executable = "${piAgent}";
      argumentPolicy = "pi-0.84.4";
      mandatoryArgs = [ ];
      resourceArgs = normalizedResources.resourceArgs;
      reservedFlags = [ "--session-dir" "--session" "--fork" "--export" "--extension" "-e" "--skill" "--prompt-template" "--theme" ];
      reservedCommands = [ "install" "remove" "uninstall" "update" "list" "config" ];
      environment = {
        scrub = [ "PI_CODING_AGENT_DIR" "PI_CODING_AGENT_SESSION_DIR" "PI_PACKAGE_DIR" "PI_OFFLINE" ];
        set = { PI_OFFLINE = "1"; };
      };
      packageDirectory = { name = "PI_PACKAGE_DIR"; value = pi.packageRoot; };
      securityAdapter = if isDarwin then {
        kind = "pi-extension";
        path = securityExtension;
        arguments = [ "--extension" securityExtension ];
      } else null;
    };
    stateBindings = [
      {
        name = "agent";
        explicitPath = options.agentDir;
        inheritedEnvironment = "PI_CODING_AGENT_DIR";
        defaultPath = ".local/state/den/pi/agent";
        defaultWritablePaths = [ ];
        exports = [{ kind = "environment"; name = "PI_CODING_AGENT_DIR"; exportDefault = true; }];
      }
      {
        name = "session";
        explicitPath = options.sessionDir;
        inheritedEnvironment = "PI_CODING_AGENT_SESSION_DIR";
        defaultPath = ".local/state/den/pi/sessions";
        defaultWritablePaths = [ ];
        exports = [
          { kind = "environment"; name = "PI_CODING_AGENT_SESSION_DIR"; exportDefault = true; }
          { kind = "argument"; name = "--session-dir"; exportDefault = true; }
        ];
      }
    ];
  };
}
