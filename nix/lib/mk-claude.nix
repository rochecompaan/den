{ pkgs, fence, mkAgentSandbox, isDarwin ? pkgs.stdenv.isDarwin }:

{ configDir ? null, extraPkgs ? [ ], docker ? { }, podman ? { } }:

let
  lib = pkgs.lib;
  claude = pkgs.claude-code;
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
  inherit configDir extraPkgs docker podman;
  adapter = {
    runtimePackages = [ claude ];
    agent = {
      name = "claude";
      executable = "${claude}/bin/claude";
      inherit mandatoryArgs;
      reservedFlags = [ "--settings" "--permission-mode" "--dangerously-skip-permissions" ];
      configEnvironment = "CLAUDE_CONFIG_DIR";
      darwinSettings = lib.optionalString isDarwin settings;
    };
  };
}
