{ inputs, pkgs }:

let
  lib = pkgs.lib;
  mkPi = import ../lib/mk-pi.nix { inherit inputs pkgs; };
  pi = mkPi {
    agentDir = null;
    sessionDir = null;
    extraPkgs = [ ];
    resources = { extensions = [ ]; packages = [ ]; skills = [ ]; promptTemplates = [ ]; themes = [ ]; };
    docker = { };
    podman = { };
  };
in
assert lib.assertMsg (pi.name == "pi") "Pi adapter output name is wrong";
pkgs.runCommand "pi-adapter"
  { nativeBuildInputs = [ pkgs.jq ]; manifest = pi.denManifest; }
  ''
    set -eu
    ${pkgs.jq}/bin/jq -e '
      .agent.name == "pi" and
      .agent.commandName == "pi" and
      .agent.argumentPolicy == "pi-0.84.4" and
      .agent.environment.scrub == ["PI_CODING_AGENT_DIR", "PI_CODING_AGENT_SESSION_DIR", "PI_PACKAGE_DIR", "PI_OFFLINE"] and
      .agent.environment.set.PI_OFFLINE == "1" and
      .agent.packageDirectory.name == "PI_PACKAGE_DIR" and
      .agent.reservedFlags == ["--session-dir", "--session", "--fork", "--export", "--extension", "-e", "--skill", "--prompt-template", "--theme"] and
      .agent.reservedCommands == ["install", "remove", "uninstall", "update", "list", "config"] and
      ((.stateBindings | length) == 2) and
      ((.protectedPathPatterns | index("~/.pi/agent")) != null) and
      ((.protectedPathPatterns | index("~/.agents")) != null) and
      ((.protectedPathPatterns | index("~/.agents/skills")) != null)
    ' "$manifest"
    test -x ${pi}/bin/pi
    touch "$out"
  ''
