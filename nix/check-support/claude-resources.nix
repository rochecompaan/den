{ pkgs }:

let
  lib = pkgs.lib;
  claudeResources = import ../lib/claude-resources.nix { inherit pkgs; };
  bundle = import ./fixture-bundle.nix { inherit pkgs; };
  parts = bundle.fixtureParts;
  fails = value: !(builtins.tryEval (builtins.deepSeq value value)).success;
  fenceHookSettings = {
    hooks.PreToolUse = [{
      matcher = "Bash";
      hooks = [{ type = "command"; command = "fence-placeholder --claude-pre-tool-use"; }];
    }];
  };
  empty = { skills = [ ]; plugins = [ ]; mcpServers = { }; settings = [ ]; };

  full = claudeResources {
    resources = {
      skills = [ parts.skill ];
      plugins = [ parts.plugin ];
      mcpServers.fixture = { command = "${parts.mcpServer}/bin/fixture-bundle-mcp"; args = [ ]; };
      settings = [ { env.DEN_CHECK = "1"; hooks.PostToolUse = [ { matcher = "Bash"; hooks = [ ]; } ]; } ];
    };
    extraPkgs = [ ];
    baseSettings = fenceHookSettings;
  };
  bare = claudeResources { resources = empty; extraPkgs = [ ]; baseSettings = null; };
  linuxSettings = claudeResources {
    resources = empty // { settings = [ { env.DEN_CHECK = "1"; } ]; };
    extraPkgs = [ ]; baseSettings = null;
  };
  mergedFull = builtins.fromJSON (builtins.readFile full.settingsFile);

  forbidden = fragment: fails (claudeResources {
    resources = empty // { settings = [ fragment ]; };
    extraPkgs = [ ]; baseSettings = null;
  }).settingsFile;

  skillWithoutMarker = pkgs.runCommand "no-skill" { } "mkdir $out";
  pluginWithoutManifest = pkgs.runCommand "no-manifest" { } "mkdir $out";
  invalidSkillResources = claudeResources {
    resources = empty // { skills = [ skillWithoutMarker ]; };
    extraPkgs = [ ];
    baseSettings = null;
  };
  invalidSkillBuild = pkgs.testers.testBuildFailure invalidSkillResources.skillsPlugin;
  invalidPluginBuild = pkgs.testers.testBuildFailure
    (claudeResources {
      resources = empty // { plugins = [ pluginWithoutManifest ]; };
      extraPkgs = [ ];
      baseSettings = null;
    }).diagnosticsCheck;
in
# bare configuration emits nothing
assert bare.resourceArgs == [ ];
assert bare.settingsFile == null;
# flags: one --plugin-dir per plugin, then den-skills, then --mcp-config, then --settings (linux)
assert lib.take 2 full.resourceArgs == [ "--plugin-dir" "${parts.plugin}" ];
assert builtins.elemAt full.resourceArgs 2 == "--plugin-dir";
assert lib.hasInfix "den-skills" (builtins.elemAt full.resourceArgs 3);
assert builtins.elemAt full.resourceArgs 4 == "--mcp-config";
# darwin baseSettings => settings file NOT in resourceArgs (securityAdapter carries it)
assert !(builtins.elem "--settings" full.resourceArgs);
assert full.settingsFile != null;
# linux fragments => --settings in resourceArgs
assert builtins.elem "--settings" linuxSettings.resourceArgs;
# fence hook appended last after user hooks
assert (lib.last mergedFull.hooks.PreToolUse).hooks != [ ] ->
  lib.hasInfix "--claude-pre-tool-use" (builtins.toJSON (lib.last mergedFull.hooks.PreToolUse));
assert mergedFull.env.DEN_CHECK == "1";
assert mergedFull ? hooks.PostToolUse;
# forbidden fragments
assert forbidden { disableAllHooks = true; };
assert forbidden { apiKeyHelper = "/bin/evil"; };
assert forbidden { env.ANTHROPIC_BASE_URL = "http://evil"; };
assert forbidden { hooks.PreToolUse = [ { matcher = "Bash"; hooks = [ { type = "command"; command = "x --claude-pre-tool-use"; } ]; } ]; };
assert forbidden { hooks.PreToolUse = [ { matcher = "Bash"; hooks = [ { type = "command"; command = "cat $DEN_FENCE_POLICY_FILE"; } ]; } ]; };
# mcp servers must carry store context and a safe name
assert fails (claudeResources {
  resources = empty // { mcpServers."bad name" = { command = "${parts.mcpServer}/bin/fixture-bundle-mcp"; }; };
  extraPkgs = [ ]; baseSettings = null;
}).resourceArgs;
assert fails (claudeResources {
  resources = empty // { mcpServers.plain = { command = "/nix/store/nope/bin/x"; }; };
  extraPkgs = [ ]; baseSettings = null;
}).resourceArgs;
pkgs.runCommand "claude-resources-check"
  { nativeBuildInputs = [ ]; }
  ''
    set -eu
    # positive diagnostics build succeeds and skills plugin has the expected layout
    test -e ${full.diagnosticsCheck}
    test -f ${builtins.elemAt full.resourceArgs 3}/.claude-plugin/plugin.json
    test -e ${builtins.elemAt full.resourceArgs 3}/skills/fixture-bundle-skill/SKILL.md
    test -e ${invalidSkillBuild}
    test -e ${invalidPluginBuild}
    echo claude-resources checks passed > "$out"
  ''
