{ inputs, pkgs }:

let
  lib = pkgs.lib;
  defaults = {
    agentDir = null;
    sessionDir = null;
    extraPkgs = [ ];
    resources = { extensions = [ ]; packages = [ ]; skills = [ ]; promptTemplates = [ ]; themes = [ ]; };
    docker = { };
    podman = { };
  };
  mkPi = import ../lib/mk-pi.nix { inherit inputs pkgs; };
  pi = mkPi defaults;
  adapterFor = isDarwin: (import ../lib/mk-pi.nix {
    inherit inputs isDarwin pkgs;
    mkAgentSandbox = value: value;
  }) defaults;
  linuxAdapter = adapterFor false;
  darwinAdapter = adapterFor true;
  fenceInfo = import ../lib/fence.nix { inherit pkgs; };
in
assert lib.assertMsg (pi.name == "pi") "Pi adapter output name is wrong";
assert fenceInfo.version == "0.1.58";
assert fenceInfo.sourceHash == "sha256-ACe3N4bXYJW6QDQHtRChFWOTXTZTbEUbZ4d8cuFRqMY=";
assert fenceInfo.patchHash == "4be4f0266a0a79da10002893752ea8185915f6ecfb146513946bde8a96e41e2a";
assert fenceInfo.capabilities.claudePreToolUse;
assert fenceInfo.capabilities.denFenceTmpdir;
assert fenceInfo.capabilities.strictDenyRead;
assert linuxAdapter.adapter.agent.securityAdapter == null;
assert !(lib.any (argument: lib.hasInfix "den-pi-security" (toString argument))
  (linuxAdapter.adapter.agent.mandatoryArgs ++ linuxAdapter.adapter.agent.resourceArgs));
assert darwinAdapter.adapter.agent.securityAdapter.kind == "pi-extension";
assert darwinAdapter.adapter.agent.securityAdapter.arguments == [ "--extension" darwinAdapter.adapter.agent.securityAdapter.path ];
assert builtins.head darwinAdapter.adapter.agent.securityAdapter.arguments == "--extension";
assert !(builtins.elem darwinAdapter.adapter.agent.securityAdapter.path darwinAdapter.adapter.agent.resourceArgs);
pkgs.runCommand "pi-adapter"
  {
    nativeBuildInputs = [ pkgs.jq ];
    inherit (pi) denManifest;
    darwinAgent = darwinAdapter.adapter.agent.executable;
    linuxAgent = linuxAdapter.adapter.agent.executable;
  }
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
    ' "$denManifest"
    grep -F 'revalidate_security_input' "$darwinAgent"
    ! grep -F 'revalidate_security_input' "$linuxAgent"
    ${if pkgs.stdenv.isDarwin then
      ''${pkgs.jq}/bin/jq -e '.agent.securityAdapter.kind == "pi-extension" and .agent.securityAdapter.arguments[0] == "--extension"' "$denManifest"''
    else
      ''${pkgs.jq}/bin/jq -e '.agent.securityAdapter == null and ([.agent.mandatoryArgs[], .agent.resourceArgs[]] | all(contains("den-pi-security") | not))' "$denManifest"''}
    test -x ${pi}/bin/pi
    touch "$out"
  ''
