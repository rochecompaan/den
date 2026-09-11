{ pkgs, fence, mkAgentSandbox, isDarwin ? pkgs.stdenv.isDarwin }:

args@{ configDir ? null, extraPkgs ? [ ], docker ? { }, podman ? { }, ... }:

let
  lib = pkgs.lib;
  options = import ./options.nix { inherit pkgs; } args;
  claude = pkgs.claude-code;
  claudeExecutable = pkgs.writeShellScript "den-claude-agent" ''
    export NODE_EXTRA_CA_CERTS="$REPOWOLF_CA_FILE"
    export CLAUDE_CODE_TMPDIR="$DEN_FENCE_TMPDIR"
    export GIT_SSH_COMMAND="$REPOWOLF_CLIENT_DIR/bin/repowolf-git-ssh"
    exec ${claude}/bin/claude "$@"
  '';
  settings = pkgs.writeText "den-claude-settings.json" (builtins.toJSON {
    hooks.PreToolUse = [
      {
        matcher = "Bash";
        hooks = [
          {
            type = "command";
            command = "${fence}/bin/fence --claude-pre-tool-use --settings \"$DEN_FENCE_POLICY_FILE\"";
          }
        ];
      }
    ];
  });
  mandatoryArgs = [ "--dangerously-skip-permissions" ];
in
assert lib.assertMsg (claude.version == "2.1.158")
  "Den requires Claude Code 2.1.158; refusing unknown version ${claude.version}";
mkAgentSandbox {
  inherit (options) configDir extraPkgs docker podman;
  adapter = {
    output = {
      packageName = "claude";
      commandName = "claude";
      manifestName = "claude-manifest.json";
      mainProgram = "claude";
    };
    runtimePackages = [ claude ];
    closureOnlyPackages = [ claudeExecutable ]
      ++ lib.optionals isDarwin [ settings ];
    agent = {
      name = "claude";
      executable = "${claudeExecutable}";
      argumentPolicy = "claude";
      inherit mandatoryArgs;
      resourceArgs = [ ];
      reservedFlags = [ "--settings" "--permission-mode" "--dangerously-skip-permissions" ];
      reservedCommands = [ ];
      environment = { scrub = [ ]; set = { }; };
      packageDirectory = null;
      securityAdapter = if isDarwin then {
        kind = "claude-settings";
        path = settings;
        arguments = [ "--settings" settings ];
      } else null;
      # Compatibility adapter metadata for existing Nix fixture consumers.
      configEnvironment = "CLAUDE_CONFIG_DIR";
      darwinSettings = lib.optionalString isDarwin settings;
    };
    stateBindings = [{
      name = "config";
      explicitPath = options.configDir;
      inheritedEnvironment = "CLAUDE_CONFIG_DIR";
      defaultPath = "";
      defaultWritablePaths = [ ];
      exports = [{ kind = "environment"; name = "CLAUDE_CONFIG_DIR"; exportDefault = false; }];
    }];
  };
}
