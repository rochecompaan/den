{ pkgs, fence, mkAgentSandbox, isDarwin ? pkgs.stdenv.isDarwin }:

args@{ configDir ? null, extraPkgs ? [ ], docker ? { }, podman ? { }, ... }:

let
  lib = pkgs.lib;
  options = import ./options.nix { inherit pkgs; } args;
  claude = pkgs.claude-code;
  claudeExecutable = pkgs.writeShellScript "den-claude-agent" ''
    export NODE_EXTRA_CA_CERTS="$REPOWOLF_CA_FILE"
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
  mandatoryArgs = [ "--dangerously-skip-permissions" ]
    ++ lib.optionals isDarwin [ "--settings" settings ];
in
assert lib.assertMsg (claude.version == "2.1.158")
  "Den requires Claude Code 2.1.158; refusing unknown version ${claude.version}";
mkAgentSandbox {
  inherit (options) configDir extraPkgs docker podman;
  adapter = {
    runtimePackages = [ claude ];
    closureOnlyPackages = [ claudeExecutable ]
      ++ lib.optionals isDarwin [ settings ];
    agent = {
      name = "claude";
      executable = "${claudeExecutable}";
      inherit mandatoryArgs;
      reservedFlags = [ "--settings" "--permission-mode" "--dangerously-skip-permissions" ];
      configEnvironment = "CLAUDE_CONFIG_DIR";
      darwinSettings = lib.optionalString isDarwin settings;
    };
  };
}
