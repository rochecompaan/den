{ inputs, pkgs, mkAgentSandbox ? import ./mk-agent-sandbox.nix { inherit inputs pkgs; } }:

args@{ agentDir ? null, sessionDir ? null, extraPkgs ? [ ], resources ? { }, docker ? { }, podman ? { }, ... }:
let
  lib = pkgs.lib;
  options = import ./pi-options.nix { inherit pkgs; } args;
  pi = import ../packages/pi-coding-agent.nix { inherit pkgs; };
  fence = (import ./fence.nix { inherit pkgs; }).package;
  normalizedResources = import ./pi-resources.nix { inherit pkgs; } {
    inherit (options) resources extraPkgs;
  };
  piAgent = pkgs.writeShellScript "den-pi-agent" (builtins.replaceStrings [ "@pi@" ] [ "${pi}" ] (builtins.readFile ../pi/den-pi-agent.sh));
in
assert lib.assertMsg (pi.version == "0.84.4") "Den requires Pi 0.84.4; refusing unknown version ${pi.version}";
assert lib.assertMsg (pi.actualPatchHash == builtins.convertHash { hash = pi.patchHash; toHashFormat = "base16"; })
  "Pi hardening patch hash drifted";
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
    closureOnlyPackages = [ pi piAgent ] ++ normalizedResources.closureInputs;
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
      securityAdapter = null;
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
