{ pkgs }:

{ resources, extraPkgs ? [ ], baseSettings ? null }:
let
  lib = pkgs.lib;

  # --- settings fragments ---
  loadFragment = fragment:
    if builtins.isAttrs fragment && !lib.isDerivation fragment
    then fragment
    else builtins.fromJSON (builtins.readFile "${fragment}");
  validateFragment = fragment:
    let text = builtins.toJSON fragment; in
    assert lib.assertMsg (builtins.isAttrs fragment)
      "Claude settings fragment must be a JSON object";
    assert lib.assertMsg (!(fragment ? disableAllHooks))
      "Claude settings fragment must not set disableAllHooks";
    assert lib.assertMsg (!(fragment ? apiKeyHelper))
      "Claude settings fragment must not set apiKeyHelper";
    assert lib.assertMsg
      (!(lib.any (name: lib.hasPrefix "ANTHROPIC_" name) (builtins.attrNames (fragment.env or { }))))
      "Claude settings fragment must not override ANTHROPIC_* environment";
    assert lib.assertMsg
      (!(lib.hasInfix "--claude-pre-tool-use" text) && !(lib.hasInfix "DEN_FENCE_POLICY_FILE" text))
      "Claude settings fragment must not reference the Den fence hook";
    fragment;
  fragments = map (fragment: validateFragment (loadFragment fragment)) resources.settings;

  mergeHooks = left: right:
    left // lib.mapAttrs (name: value: (left.${name} or [ ]) ++ value) right;
  mergeFragment = left: right:
    lib.recursiveUpdate left (builtins.removeAttrs right [ "hooks" ])
    // lib.optionalAttrs (left ? hooks || right ? hooks) {
      hooks = mergeHooks (left.hooks or { }) (right.hooks or { });
    };
  userSettings = lib.foldl' mergeFragment { } fragments;
  mergedSettings =
    if baseSettings == null then userSettings else mergeFragment userSettings baseSettings;
  hasSettings = baseSettings != null || fragments != [ ];
  settingsFile =
    if hasSettings
    then pkgs.writeText "den-claude-settings.json" (builtins.toJSON mergedSettings)
    else null;

  # --- skills -> den-skills plugin ---
  skillsPlugin = pkgs.runCommand "den-skills"
    { nativeBuildInputs = [ pkgs.coreutils pkgs.findutils ]; }
    ''
      set -eu
      mkdir -p "$out/.claude-plugin" "$out/skills"
      printf '%s\n' '{"name":"den-skills","description":"Den injected skills","version":"1.0.0"}' \
        > "$out/.claude-plugin/plugin.json"
      for entry in ${lib.escapeShellArgs (map toString resources.skills)}; do
        found=false
        while IFS= read -r skillFile; do
          found=true
          skillDirectory=$(dirname "$skillFile")
          skillName=$(basename "$skillDirectory")
          if [ -e "$out/skills/$skillName" ]; then
            echo "Claude skill name collides: $skillName" >&2
            exit 1
          fi
          ln -s "$skillDirectory" "$out/skills/$skillName"
        done < <(find -L "$entry" -type f -name SKILL.md)
        if [ "$found" != true ]; then
          echo "Claude skill resource has no SKILL.md: $entry" >&2
          exit 1
        fi
      done
    '';

  # --- mcp servers ---
  serverNamePattern = "^[A-Za-z][A-Za-z0-9_-]*$";
  allowedServerKeys = [ "command" "args" "env" ];
  validateServer = name: server:
    assert lib.assertMsg (builtins.match serverNamePattern name != null)
      "Claude MCP server has an invalid name: ${name}";
    assert lib.assertMsg
      (lib.all (key: builtins.elem key allowedServerKeys) (builtins.attrNames server))
      "Claude MCP server ${name} has an unknown option";
    assert lib.assertMsg (server ? command && builtins.isString server.command)
      "Claude MCP server ${name} needs a command string";
    assert lib.assertMsg (lib.hasPrefix builtins.storeDir server.command)
      "Claude MCP server ${name} command must be a store path";
    assert lib.assertMsg (builtins.hasContext server.command)
      "Claude MCP server ${name} command must reference its package (write \"\${pkg}/bin/...\")";
    server;
  mcpServers = lib.mapAttrs validateServer resources.mcpServers;
  hasMcp = mcpServers != { };
  mcpConfigFile = pkgs.writeText "den-mcp.json" (builtins.toJSON { inherit mcpServers; });

  # --- flags ---
  hasSkills = resources.skills != [ ];
  pluginArgs = lib.concatMap (entry: [ "--plugin-dir" "${entry}" ]) resources.plugins
    ++ lib.optionals hasSkills [ "--plugin-dir" "${skillsPlugin}" ];
  mcpArgs = lib.optionals hasMcp [ "--mcp-config" "${mcpConfigFile}" ];
  linuxSettingsArgs = lib.optionals (baseSettings == null && settingsFile != null)
    [ "--settings" "${settingsFile}" ];
  resourceArgs = pluginArgs ++ mcpArgs ++ linuxSettingsArgs;

  # --- diagnostics ---
  pluginEntries = lib.imap0 (index: entry: { inherit index entry; }) resources.plugins;
  diagnosticsCheck = pkgs.runCommand "claude-resource-validation"
    { nativeBuildInputs = [ pkgs.coreutils pkgs.jq ]; }
    ''
      set -euo pipefail
      declare -A pluginNames
      pluginNames[den-skills]=reserved
      check_plugin() {
        entry=$1
        manifest="$entry/.claude-plugin/plugin.json"
        if [ ! -f "$manifest" ]; then
          echo "Claude plugin has no .claude-plugin/plugin.json: $entry" >&2
          exit 1
        fi
        name=$(${pkgs.jq}/bin/jq -er .name "$manifest") || {
          echo "Claude plugin manifest has no name: $entry" >&2
          exit 1
        }
        if [ -n "''${pluginNames[$name]+x}" ]; then
          echo "Claude plugin name collides: $name" >&2
          exit 1
        fi
        pluginNames[$name]=1
      }
      ${lib.concatMapStringsSep "\n" (item: "check_plugin ${lib.escapeShellArg "${item.entry}"}") pluginEntries}
      ${lib.concatMapStringsSep "\n" (server: ''
        if [ ! -x ${lib.escapeShellArg server.command} ]; then
          echo "Claude MCP server command is not executable: ${server.command}" >&2
          exit 1
        fi
      '') (builtins.attrValues mcpServers)}
      echo 'Claude resource validation passed.' > "$out"
    '';

  resourceClosure = pkgs.linkFarm "claude-configured-resources"
    (lib.imap0 (index: path: { name = toString index; path = path; })
      (resources.plugins
        ++ lib.optional hasSkills skillsPlugin
        ++ lib.optional hasMcp mcpConfigFile
        ++ lib.optional (settingsFile != null) settingsFile));
in
{
  inherit resourceArgs settingsFile diagnosticsCheck;
  # Exposed for the build-failure check; consumers use the documented keys above.
  skillsPlugin = if hasSkills then skillsPlugin else null;
  closureInputs = [ diagnosticsCheck resourceClosure ];
}
