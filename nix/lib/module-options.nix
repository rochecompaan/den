{ lib, pkgs }:

let
  inherit (lib) mkOption types;
  containerOptions = name: package: composePackage: {
    enable = mkOption {
      type = types.bool;
      default = false;
      description = "Whether to allow Claude to use the ${name} daemon.";
    };
    package = mkOption {
      type = types.package;
      default = package;
      description = "The ${name} client package available inside the Claude sandbox.";
    };
    composePackage = mkOption {
      type = types.package;
      default = composePackage;
      description = "The Compose client package available inside the Claude sandbox.";
    };
    socketPath = mkOption {
      type = types.nullOr (types.strMatching "^/.*");
      default = null;
      description = "The ${name} socket path, or null for runtime discovery.";
    };
    hostPorts = mkOption {
      type = types.listOf (types.ints.between 1 65535);
      default = [ ];
      description = "Host localhost ports available to ${name}. On macOS, Fence permits all localhost ports when this list is non-empty because it cannot enforce exact port restrictions.";
    };
  };
in
{
  options.programs.den.claude = {
    enable = mkOption {
      type = types.bool;
      default = false;
      description = "Whether to add Den's sandboxed Claude package.";
    };
    configDir = mkOption {
      type = types.nullOr (types.strMatching "^/.*");
      default = null;
      description = "The Claude configuration directory, or null to use runtime discovery.";
    };
    extraPkgs = mkOption {
      type = types.listOf types.package;
      default = [ ];
      description = "Packages available only inside the Claude sandbox.";
    };
    docker = containerOptions "Docker" pkgs.docker-client pkgs.docker-compose;
    podman = containerOptions "Podman" pkgs.podman pkgs.podman-compose;
  };

  assertions = claude: [
    {
      assertion = claude.docker.enable || claude.docker.hostPorts == [ ];
      message = "programs.den.claude.docker.hostPorts requires programs.den.claude.docker.enable = true";
    }
    {
      assertion = claude.podman.enable || claude.podman.hostPorts == [ ];
      message = "programs.den.claude.podman.hostPorts requires programs.den.claude.podman.enable = true";
    }
    {
      assertion = builtins.length claude.docker.hostPorts == builtins.length (lib.unique claude.docker.hostPorts);
      message = "programs.den.claude.docker.hostPorts must contain unique ports";
    }
    {
      assertion = builtins.length claude.podman.hostPorts == builtins.length (lib.unique claude.podman.hostPorts);
      message = "programs.den.claude.podman.hostPorts must contain unique ports";
    }
  ];
}
