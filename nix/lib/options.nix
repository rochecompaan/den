{ pkgs }:

raw:
let
  lib = pkgs.lib;
  defaults = {
    configDir = null;
    extraPkgs = [ ];
    docker = {
      enable = false;
      package = pkgs.docker-client;
      composePackage = pkgs.docker-compose;
      socketPath = null;
      hostPorts = [ ];
    };
    podman = {
      enable = false;
      package = pkgs.podman;
      composePackage = pkgs.podman-compose;
      socketPath = null;
      hostPorts = [ ];
    };
  };
  allowedRootOptions = [ "configDir" "extraPkgs" "docker" "podman" ];
  allowedContainerOptions = [ "enable" "package" "composePackage" "socketPath" "hostPorts" ];
  hasOnly = allowed: value: lib.all (name: builtins.elem name allowed) (builtins.attrNames value);
  isAbsoluteString = value: builtins.isString value && builtins.match "^/.*" value != null;
  isPackage = value: lib.isDerivation value;
  isPort = value: builtins.isInt value && value >= 1 && value <= 65535;
  docker = defaults.docker // (raw.docker or { });
  podman = defaults.podman // (raw.podman or { });
  validContainer = name: value:
    assert lib.assertMsg (hasOnly allowedContainerOptions value) "${name} has an unknown option";
    assert lib.assertMsg (builtins.isBool value.enable) "${name}.enable must be a Boolean";
    assert lib.assertMsg (isPackage value.package) "${name}.package must be a package";
    assert lib.assertMsg (isPackage value.composePackage) "${name}.composePackage must be a package";
    assert lib.assertMsg (value.socketPath == null || isAbsoluteString value.socketPath)
      "${name}.socketPath must be null or an absolute string";
    assert lib.assertMsg (builtins.isList value.hostPorts && lib.all isPort value.hostPorts)
      "${name}.hostPorts must contain unique ports from 1 through 65535";
    assert lib.assertMsg (builtins.length value.hostPorts == builtins.length (lib.unique value.hostPorts))
      "${name}.hostPorts must contain unique ports from 1 through 65535";
    assert lib.assertMsg (value.enable || value.hostPorts == [ ])
      "${name}.hostPorts requires ${name}.enable = true";
    value;
in
assert lib.assertMsg (builtins.isAttrs raw) "Claude options must be an attribute set";
assert lib.assertMsg (hasOnly allowedRootOptions raw) "Claude has an unknown option";
assert lib.assertMsg (!(raw ? configDir) || raw.configDir == null || isAbsoluteString raw.configDir)
  "configDir must be null or an absolute string";
assert lib.assertMsg (!(raw ? extraPkgs) || (builtins.isList raw.extraPkgs && lib.all isPackage raw.extraPkgs))
  "extraPkgs must be a list of packages";
assert lib.assertMsg (!(raw ? docker) || builtins.isAttrs raw.docker) "docker must be an attribute set";
assert lib.assertMsg (!(raw ? podman) || builtins.isAttrs raw.podman) "podman must be an attribute set";
{
  configDir = raw.configDir or defaults.configDir;
  extraPkgs = raw.extraPkgs or defaults.extraPkgs;
  docker = validContainer "docker" docker;
  podman = validContainer "podman" podman;
}
