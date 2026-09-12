{ pkgs }:

let
  lib = pkgs.lib;
  denResources = import ../lib/den-resources.nix { inherit pkgs; };
  bundle = import ./fixture-bundle.nix { inherit pkgs; };
  parts = bundle.fixtureParts;
  emptyClaude = { skills = [ ]; plugins = [ ]; mcpServers = { }; settings = [ ]; };
  emptyPi = { extensions = [ ]; packages = [ ]; skills = [ ]; promptTemplates = [ ]; themes = [ ]; };
  fails = value: !(builtins.tryEval (builtins.deepSeq value value)).success;

  claudeMerged = denResources { agent = "claude"; bundles = [ bundle ]; resources = emptyClaude // { skills = [ parts.skill ]; }; };
  piMerged = denResources { agent = "pi"; bundles = [ bundle ]; resources = emptyPi; };

  badBundleNoPassthru = pkgs.runCommand "bad-bundle" { } "mkdir $out";
  badBundleAgent = pkgs.runCommand "bad-agent" { passthru.denResources.codex = { }; } "mkdir $out";
  badBundleClass = pkgs.runCommand "bad-class" { passthru.denResources.claude.extensions = [ ]; } "mkdir $out";
  duplicateMcp = denResources {
    agent = "claude";
    bundles = [ bundle ];
    resources = emptyClaude // { mcpServers.fixture = { command = "${parts.mcpServer}/bin/fixture-bundle-mcp"; }; };
  };
in
# bundle classes land before direct entries
assert claudeMerged.skills == [ parts.skill parts.skill ];
assert claudeMerged.plugins == [ parts.plugin ];
assert claudeMerged.settings == [ parts.settingsFragment ];
assert builtins.attrNames claudeMerged.mcpServers == [ "fixture" ];
# missing agent key contributes nothing
assert piMerged.extensions == [ parts.piExtension ];
assert piMerged.packages == [ ];
# rejection table
assert fails (denResources { agent = "claude"; bundles = [ badBundleNoPassthru ]; resources = emptyClaude; });
assert fails (denResources { agent = "claude"; bundles = [ badBundleAgent ]; resources = emptyClaude; });
assert fails (denResources { agent = "claude"; bundles = [ badBundleClass ]; resources = emptyClaude; });
assert fails duplicateMcp;
pkgs.runCommand "den-resources-check" { } "echo den-resources eval checks passed > $out"
