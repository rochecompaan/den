{ lib, pkgs }:

let
  inherit (lib) mkOption types;
  containerOptions = agent: name: package: composePackage: {
    enable = mkOption {
      type = types.bool;
      default = false;
      description = "Whether to allow ${agent} to use the ${name} daemon.";
    };
    package = mkOption {
      type = types.package;
      default = package;
      description = "The ${name} client package available inside the ${agent} sandbox.";
    };
    composePackage = mkOption {
      type = types.package;
      default = composePackage;
      description = "The Compose client package available inside the ${agent} sandbox.";
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
  containerAssertions = optionName: value: [
    {
      assertion = value.docker.enable || value.docker.hostPorts == [ ];
      message = "${optionName}.docker.hostPorts requires ${optionName}.docker.enable = true";
    }
    {
      assertion = value.podman.enable || value.podman.hostPorts == [ ];
      message = "${optionName}.podman.hostPorts requires ${optionName}.podman.enable = true";
    }
    {
      assertion = builtins.length value.docker.hostPorts == builtins.length (lib.unique value.docker.hostPorts);
      message = "${optionName}.docker.hostPorts must contain unique ports";
    }
    {
      assertion = builtins.length value.podman.hostPorts == builtins.length (lib.unique value.podman.hostPorts);
      message = "${optionName}.podman.hostPorts must contain unique ports";
    }
  ];
  resourceType = types.either types.path types.package;
in
{
  options.programs.den = {
    claude = {
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
      docker = containerOptions "Claude" "Docker" pkgs.docker-client pkgs.docker-compose;
      podman = containerOptions "Claude" "Podman" pkgs.podman pkgs.podman-compose;
    };

    pi = {
      enable = mkOption {
        type = types.bool;
        default = false;
        description = "Whether to add Den's sandboxed Pi package.";
      };
      agentDir = mkOption {
        type = types.nullOr (types.strMatching "^/.*");
        default = null;
        description = "The Pi agent directory, or null to use runtime discovery.";
      };
      sessionDir = mkOption {
        type = types.nullOr (types.strMatching "^/.*");
        default = null;
        description = "The Pi session directory, or null to use runtime discovery.";
      };
      extraPkgs = mkOption {
        type = types.listOf types.package;
        default = [ ];
        description = "Packages available only inside the Pi sandbox.";
      };
      resources = {
        extensions = mkOption { type = types.listOf resourceType; default = [ ]; description = "Immutable Pi extensions."; };
        packages = mkOption { type = types.listOf resourceType; default = [ ]; description = "Immutable Pi resource packages."; };
        skills = mkOption { type = types.listOf resourceType; default = [ ]; description = "Immutable Pi skills."; };
        promptTemplates = mkOption { type = types.listOf resourceType; default = [ ]; description = "Immutable Pi prompt templates."; };
        themes = mkOption { type = types.listOf resourceType; default = [ ]; description = "Immutable Pi themes."; };
      };
      docker = containerOptions "Pi" "Docker" pkgs.docker-client pkgs.docker-compose;
      podman = containerOptions "Pi" "Podman" pkgs.podman pkgs.podman-compose;
    };
  };

  assertions = claude: containerAssertions "programs.den.claude" claude;
  piAssertions = pi: containerAssertions "programs.den.pi" pi;
}
