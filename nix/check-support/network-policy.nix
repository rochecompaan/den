{ pkgs, den }:

let
  lib = pkgs.lib;
  fixtures = import ./network-policy-fixtures.nix { inherit pkgs den; };
  constructors = {
    claude = den.lib.${pkgs.system}.mkClaude;
    pi = den.lib.${pkgs.system}.mkPi;
  };
  fails = value: !(builtins.tryEval (builtins.deepSeq value true)).success;
  invalidNetworks = [
    { extraAllowedDomains = [ "API.example.com" ]; }
    { extraAllowedDomains = [ "https://api.example.com" ]; }
    { extraAllowedDomains = [ 42 ]; }
    { unexpected = true; }
  ];
  unchangedWhenEmpty = agent:
    (constructors.${agent} { network.extraAllowedDomains = [ ]; }).outPath
      == fixtures.defaults.${agent}.outPath;
  rejectsInvalid = agent: lib.all (network: fails (constructors.${agent} { inherit network; }).outPath) invalidNetworks;
in
assert lib.all unchangedWhenEmpty [ "claude" "pi" ];
assert lib.all rejectsInvalid [ "claude" "pi" ];
pkgs.runCommand "network-policy"
  {
    nativeBuildInputs = [ pkgs.jq ];
    bundledPolicy = ../../policy/fence.json;
    claudeDefaultManifest = fixtures.defaults.claude.denManifest;
    piDefaultManifest = fixtures.defaults.pi.denManifest;
    claudeCustomManifest = fixtures.custom.claude.denManifest;
    piCustomManifest = fixtures.custom.pi.denManifest;
  }
  ''
    set -eu
    for agent in claude pi; do
      case "$agent" in
        claude) defaultManifest="$claudeDefaultManifest"; customManifest="$claudeCustomManifest"; other=pi ;;
        pi) defaultManifest="$piDefaultManifest"; customManifest="$piCustomManifest"; other=claude ;;
      esac
      defaultPolicy=$(jq -r .basePolicy "$defaultManifest")
      customPolicy=$(jq -r .basePolicy "$customManifest")
      test "$defaultPolicy" = "$bundledPolicy"
      jq -e --slurpfile bundled "$bundledPolicy" \
        --arg own "$agent.project.test" --arg other "$other.project.test" '
        .network.allowedDomains == ($bundled[0].network.allowedDomains + [
          "api.project.test", "*.services.project.test",
          "github.com", "*.github.com", "githubusercontent.com", "*.githubusercontent.com",
          "gitlab.com", "*.gitlab.com", "bitbucket.org", "*.bitbucket.org",
          "metadata.google.internal", "statsig.anthropic.com", $own
        ]) and
        ([.network.allowedDomains[] | select(. == "api.project.test")] | length) == 1 and
        ([.network.allowedDomains[] | select(. == "registry.npmjs.org")] | length) == 1 and
        (.network.allowedDomains | index($own)) != null and
        (.network.allowedDomains | index($other)) == null
      ' "$customPolicy"
      jq -e --slurpfile original "$defaultPolicy" '
        del(.network.allowedDomains) == ($original[0] | del(.network.allowedDomains))
      ' "$customPolicy"
    done
    touch "$out"
  ''
