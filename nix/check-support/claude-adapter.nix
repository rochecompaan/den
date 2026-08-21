{ pkgs, fence }:

let
  mkClaude = isDarwin: import ../lib/mk-claude.nix {
    inherit pkgs fence isDarwin;
    mkAgentSandbox = value: value;
  };
  adapter = (mkClaude false) { };
  darwinAdapter = (mkClaude true) { };
in
pkgs.runCommand "claude-adapter"
  {
    nativeBuildInputs = [ pkgs.jq ];
    adapter = builtins.toJSON adapter;
    darwinSettings = darwinAdapter.agent.darwinSettings;
  }
  ''
    set -eu
    printf '%s\n' "$adapter" > adapter.json
    jq -e '
      .agent.name == "claude" and
      (.agent.executable | endswith("/bin/claude")) and
      .agent.mandatoryArgs == ["--dangerously-skip-permissions"] and
      .agent.reservedFlags == ["--settings", "--permission-mode", "--dangerously-skip-permissions"] and
      .agent.configEnvironment == "CLAUDE_CONFIG_DIR" and
      (.agent | has("skills") | not) and
      (.agent | has("plugins") | not) and
      (.agent | has("mcp") | not) and
      (.agent | has("contextMode") | not) and
      (.agent | has("codeGraph") | not) and
      (.agent | has("seedDirectory") | not) and
      (.agent | has("resourcePackage") | not) and
      (.agent | has("hooks") | not)
    ' adapter.json
    test -n "$darwinSettings"
    test -e "$darwinSettings"
    jq -e --arg fence "${fence}/bin/fence" '
      .hooks.PreToolUse == [{
        matcher: "Bash",
        hooks: [{
          type: "command",
          command: ($fence + " --claude-pre-tool-use --settings \"$DEN_FENCE_POLICY_FILE\"")
        }]
      }]
    ' "$darwinSettings"
    touch "$out"
  ''
