{ pkgs, den }:

let
  constructors = {
    claude = den.lib.${pkgs.system}.mkClaude;
    pi = den.lib.${pkgs.system}.mkPi;
  };
  commonDomains = [
    "api.project.test" "*.services.project.test" "registry.npmjs.org" "api.project.test"
    "github.com" "*.github.com" "githubusercontent.com" "*.githubusercontent.com"
    "gitlab.com" "*.gitlab.com" "bitbucket.org" "*.bitbucket.org"
    "metadata.google.internal" "statsig.anthropic.com"
  ];
  extraDomains = {
    claude = commonDomains ++ [ "claude.project.test" ];
    pi = commonDomains ++ [ "pi.project.test" ];
  };
in
{
  inherit extraDomains;
  defaults = pkgs.lib.mapAttrs (_: constructor: constructor { }) constructors;
  custom = pkgs.lib.mapAttrs (agent: constructor: constructor {
    network.extraAllowedDomains = extraDomains.${agent};
  }) constructors;
}
