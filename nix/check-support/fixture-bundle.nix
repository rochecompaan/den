{ pkgs }:

let
  skill = pkgs.runCommand "fixture-bundle-skill" { } ''
    mkdir -p "$out/fixture-bundle-skill"
    {
      printf -- '---\n'
      printf 'name: fixture-bundle-skill\n'
      printf 'description: Use when the user says amber-lynx\n'
      printf -- '---\n'
      printf 'Reply with the word fixture.\n'
    } > "$out/fixture-bundle-skill/SKILL.md"
  '';
  plugin = pkgs.runCommand "fixture-bundle-plugin" { } ''
    mkdir -p "$out/.claude-plugin"
    printf '%s\n' '{"name":"fixture-bundle-plugin","version":"1.0.0"}' > "$out/.claude-plugin/plugin.json"
  '';
  mcpServer = pkgs.writeShellScriptBin "fixture-bundle-mcp" "exit 0";
  piExtension = pkgs.writeTextDir "index.ts" "export default function fixture() {}";
  settingsFragment = { env.DEN_FIXTURE_BUNDLE = "1"; };
in
pkgs.runCommand "fixture-den-bundle"
  {
    passthru = {
      denResources = {
        pi = {
          extensions = [ piExtension ];
          skills = [ skill ];
        };
        claude = {
          skills = [ skill ];
          plugins = [ plugin ];
          mcpServers = {
            fixture = {
              command = "${mcpServer}/bin/fixture-bundle-mcp";
              args = [ ];
            };
          };
          settings = [ settingsFragment ];
        };
      };
      fixtureParts = { inherit skill plugin mcpServer piExtension settingsFragment; };
    };
  } "mkdir $out"
