{ inputs, pkgs, den }:

let
  lib = pkgs.lib;
  mkClaude = den.lib.${pkgs.system}.mkClaude;
  moduleOptions = {
    programs.den.claude = {
      enable = true;
      configDir = "/tmp/den-claude-config";
      extraPkgs = [ fakeExtra ];
      docker = {
        enable = true;
        package = fakeDocker;
        composePackage = fakeDockerCompose;
        socketPath = "/tmp/docker.sock";
        hostPorts = [ 2375 2376 ];
      };
      podman = {
        enable = true;
        package = fakePodman;
        composePackage = fakePodmanCompose;
        socketPath = "/tmp/podman.sock";
        hostPorts = [ 8080 ];
      };
    };
  };
  fakeExtra = pkgs.writeShellScriptBin "module-api-extra" "exit 0";
  fakeDocker = pkgs.writeShellScriptBin "module-api-docker" "exit 0";
  fakeDockerCompose = pkgs.writeShellScriptBin "module-api-docker-compose" "exit 0";
  fakePodman = pkgs.writeShellScriptBin "module-api-podman" "exit 0";
  fakePodmanCompose = pkgs.writeShellScriptBin "module-api-podman-compose" "exit 0";
  defaultModuleOptions = { programs.den.claude.enable = true; };
  fakeDen = den // {
    lib = den.lib // {
      ${pkgs.system} = den.lib.${pkgs.system} // {
        mkClaude = _: throw "disabled module must not call mkClaude";
      };
    };
  };
  homeModule = self: (import ../../modules/home/den.nix { inherit self; }).flake.homeModules.den;
  devenvModule = self: (import ../../modules/devenv/den.nix { inherit self; }).flake.devenvModules.den;
  mkHome = module: modules: inputs.home-manager.lib.homeManagerConfiguration {
    inherit pkgs;
    modules = [ module {
      home.username = "den";
      home.homeDirectory = "/home/den";
      home.stateVersion = "24.11";
    } ] ++ modules;
  };
  mkDevenv = module: modules: inputs.devenv.lib.mkConfig {
    inherit pkgs;
    inputs = { };
    modules = [ module ] ++ modules;
  };
  homeDisabled = mkHome den.homeModules.den [ ];
  devenvDisabled = mkDevenv den.devenvModules.den [ ];
  homeDisabledLazy = mkHome (homeModule fakeDen) [ ];
  devenvDisabledLazy = mkDevenv (devenvModule fakeDen) [ ];
  homeDefaultEnabled = mkHome den.homeModules.den [ defaultModuleOptions ];
  devenvDefaultEnabled = mkDevenv den.devenvModules.den [ defaultModuleOptions ];
  homeEnabled = mkHome den.homeModules.den [ moduleOptions ];
  devenvEnabled = mkDevenv den.devenvModules.den [ moduleOptions ];
  expectedDefault = mkClaude { };
  expected = mkClaude (builtins.removeAttrs moduleOptions.programs.den.claude [ "enable" ]);
  homeDefaultPackages = homeDefaultEnabled.config.home.packages;
  devenvDefaultPackages = devenvDefaultEnabled.packages;
  homePackages = homeEnabled.config.home.packages;
  devenvPackages = devenvEnabled.packages;
  packageOutPaths = packages: builtins.map (package: package.outPath) packages;
  succeeds = value: (builtins.tryEval (builtins.deepSeq value value)).success;
  hasOutPath = outPath: packages: lib.any (package: package.outPath == outPath) packages;
  countOutPath = outPath: packages: builtins.length (builtins.filter (package: package.outPath == outPath) packages);
  fails = value: !(builtins.tryEval value).success;
  invalidHome = value: (mkHome den.homeModules.den [ { programs.den.claude = value; } ]).config.programs.den.claude;
  invalidDevenv = value: (mkDevenv den.devenvModules.den [ { programs.den.claude = value; } ]).programs.den.claude;
  invalidHomeAssertions = value: (mkHome den.homeModules.den [ { programs.den.claude = value; } ]).config.assertions;
  invalidDevenvAssertions = value: (mkDevenv den.devenvModules.den [ { programs.den.claude = value; } ]).shell.drvPath;
in
assert succeeds (packageOutPaths homeDisabledLazy.config.home.packages);
assert succeeds (packageOutPaths devenvDisabledLazy.packages);
assert !hasOutPath expectedDefault.outPath homeDisabled.config.home.packages;
assert !hasOutPath expectedDefault.outPath devenvDisabled.packages;
assert countOutPath expectedDefault.outPath homeDefaultPackages == 1;
assert countOutPath expectedDefault.outPath devenvDefaultPackages == 1;
assert homeDisabled.config.programs.den.claude.configDir == null;
assert devenvDisabled.programs.den.claude.configDir == null;
assert homeDisabled.config.programs.den.claude.extraPkgs == [ ];
assert devenvDisabled.programs.den.claude.extraPkgs == [ ];
assert !homeDisabled.config.programs.den.claude.docker.enable;
assert !devenvDisabled.programs.den.claude.docker.enable;
assert homeDisabled.config.programs.den.claude.docker.package.outPath == pkgs.docker-client.outPath;
assert devenvDisabled.programs.den.claude.docker.composePackage.outPath == pkgs.docker-compose.outPath;
assert homeDisabled.config.programs.den.claude.docker.socketPath == null;
assert devenvDisabled.programs.den.claude.docker.hostPorts == [ ];
assert !homeDisabled.config.programs.den.claude.podman.enable;
assert !devenvDisabled.programs.den.claude.podman.enable;
assert homeDisabled.config.programs.den.claude.podman.package.outPath == pkgs.podman.outPath;
assert devenvDisabled.programs.den.claude.podman.composePackage.outPath == pkgs.podman-compose.outPath;
assert homeDisabled.config.programs.den.claude.podman.socketPath == null;
assert devenvDisabled.programs.den.claude.podman.hostPorts == [ ];
assert homeEnabled.config.programs.den.claude.configDir == "/tmp/den-claude-config";
assert devenvEnabled.programs.den.claude.configDir == "/tmp/den-claude-config";
assert homeEnabled.config.programs.den.claude.extraPkgs == [ fakeExtra ];
assert devenvEnabled.programs.den.claude.extraPkgs == [ fakeExtra ];
assert homeEnabled.config.programs.den.claude.docker.enable;
assert devenvEnabled.programs.den.claude.docker.enable;
assert homeEnabled.config.programs.den.claude.docker.package.outPath == fakeDocker.outPath;
assert devenvEnabled.programs.den.claude.docker.composePackage.outPath == fakeDockerCompose.outPath;
assert homeEnabled.config.programs.den.claude.docker.socketPath == "/tmp/docker.sock";
assert devenvEnabled.programs.den.claude.docker.hostPorts == [ 2375 2376 ];
assert homeEnabled.config.programs.den.claude.podman.package.outPath == fakePodman.outPath;
assert devenvEnabled.programs.den.claude.podman.composePackage.outPath == fakePodmanCompose.outPath;
assert homeEnabled.config.programs.den.claude.podman.socketPath == "/tmp/podman.sock";
assert devenvEnabled.programs.den.claude.podman.hostPorts == [ 8080 ];
assert homeEnabled.config.programs.den.claude.docker.socketPath != null;
assert devenvEnabled.programs.den.claude.podman.socketPath != null;
assert countOutPath expected.outPath homePackages == 1;
assert countOutPath expected.outPath devenvPackages == 1;
assert !hasOutPath fakeExtra.outPath homePackages;
assert !hasOutPath fakeExtra.outPath devenvPackages;
assert fails (invalidHome { enable = false; configDir = "relative"; }).configDir;
assert fails (invalidDevenv { enable = false; configDir = "relative"; }).configDir;
assert fails (invalidHome { enable = false; docker.socketPath = "relative.sock"; }).docker.socketPath;
assert fails (invalidDevenv { enable = false; podman.socketPath = "relative.sock"; }).podman.socketPath;
assert fails (invalidHomeAssertions { enable = false; docker = { enable = true; hostPorts = [ 2375 2375 ]; }; });
assert fails (invalidDevenvAssertions { enable = false; podman = { hostPorts = [ 8080 ]; }; });
pkgs.runCommand "module-api" { } "touch $out"
