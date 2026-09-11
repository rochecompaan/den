{ inputs, pkgs, den }:

let
  lib = pkgs.lib;
  fakeExtra = pkgs.writeShellScriptBin "pi-module-extra" "exit 0";
  fakeDocker = pkgs.writeShellScriptBin "docker" "exit 0";
  fakeDockerCompose = pkgs.writeShellScriptBin "docker-compose" "exit 0";
  fakePodman = pkgs.writeShellScriptBin "podman" "exit 0";
  fakePodmanCompose = pkgs.writeShellScriptBin "podman-compose" "exit 0";
  resourcePackage = pkgs.runCommand "pi-module-resource-package" { } ''
    mkdir -p "$out/prompts"
    printf '{"name":"pi-module-resource","pi":{"prompts":["prompts/one.md"]}}\n' > "$out/package.json"
    printf 'one\n' > "$out/prompts/one.md"
  '';
  resources = {
    extensions = [ ./fixtures/pi/resources/extensions/second.ts ./fixtures/pi/resources/extensions/first.ts ];
    packages = [ resourcePackage ];
    skills = [ ./fixtures/pi/resources/skills/second ./fixtures/pi/resources/skills/first ];
    promptTemplates = [ ./fixtures/pi/resources/prompts/second.md ./fixtures/pi/resources/prompts/first.md ];
    themes = [ ./fixtures/pi/resources/themes/second.json ./fixtures/pi/resources/themes/first.json ];
  };
  piOptions = {
    enable = true;
    agentDir = "/tmp/den-pi-agent";
    sessionDir = "/tmp/den-pi-sessions";
    extraPkgs = [ fakeExtra ];
    inherit resources;
    docker = {
      enable = true;
      package = fakeDocker;
      composePackage = fakeDockerCompose;
      socketPath = "/tmp/pi-docker.sock";
      hostPorts = [ 2376 2375 ];
    };
    podman = {
      enable = true;
      package = fakePodman;
      composePackage = fakePodmanCompose;
      socketPath = "/tmp/pi-podman.sock";
      hostPorts = [ 8081 8080 ];
    };
  };
  claudeOptions = { programs.den.claude.enable = true; };
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
  configured = { programs.den.pi = piOptions; };
  both = lib.recursiveUpdate claudeOptions configured;
  piDisabledWithClaude = lib.recursiveUpdate claudeOptions { programs.den.pi.enable = false; };
  homeDisabled = mkHome den.homeModules.den [ ];
  devenvDisabled = mkDevenv den.devenvModules.den [ ];
  homeEnabled = mkHome den.homeModules.den [ configured ];
  devenvEnabled = mkDevenv den.devenvModules.den [ configured ];
  homeBoth = mkHome den.homeModules.den [ both ];
  devenvBoth = mkDevenv den.devenvModules.den [ both ];
  homeClaude = mkHome den.homeModules.den [ claudeOptions ];
  devenvClaude = mkDevenv den.devenvModules.den [ claudeOptions ];
  homePiDisabled = mkHome den.homeModules.den [ piDisabledWithClaude ];
  devenvPiDisabled = mkDevenv den.devenvModules.den [ piDisabledWithClaude ];
  expectedPi = den.lib.${pkgs.system}.mkPi (builtins.removeAttrs piOptions [ "enable" ]);
  expectedClaude = den.lib.${pkgs.system}.mkClaude { };
  paths = packages: map (package: package.outPath) packages;
  count = outPath: packages: builtins.length (builtins.filter (package: package.outPath == outPath) packages);
  fakeDen = den // {
    lib = den.lib // {
      ${pkgs.system} = den.lib.${pkgs.system} // { mkPi = _: throw "disabled Pi module called mkPi"; };
    };
  };
  homeDisabledLazy = mkHome (homeModule fakeDen) [ ];
  devenvDisabledLazy = mkDevenv (devenvModule fakeDen) [ ];
in
assert (builtins.tryEval (builtins.deepSeq (paths homeDisabledLazy.config.home.packages) true)).success;
assert (builtins.tryEval (builtins.deepSeq (paths devenvDisabledLazy.packages) true)).success;
assert !homeDisabled.config.programs.den.pi.enable;
assert !devenvDisabled.programs.den.pi.enable;
assert homeDisabled.config.programs.den.pi.agentDir == null;
assert devenvDisabled.programs.den.pi.sessionDir == null;
assert homeDisabled.config.programs.den.pi.extraPkgs == [ ];
assert devenvDisabled.programs.den.pi.resources == {
  extensions = [ ]; packages = [ ]; skills = [ ]; promptTemplates = [ ]; themes = [ ];
};
assert !homeDisabled.config.programs.den.pi.docker.enable;
assert !devenvDisabled.programs.den.pi.podman.enable;
assert homeEnabled.config.programs.den.pi.resources == resources;
assert devenvEnabled.programs.den.pi.resources == resources;
assert homeEnabled.config.programs.den.pi.docker.hostPorts == [ 2376 2375 ];
assert devenvEnabled.programs.den.pi.podman.hostPorts == [ 8081 8080 ];
assert count expectedPi.outPath homeEnabled.config.home.packages == 1;
assert count expectedPi.outPath devenvEnabled.packages == 1;
assert count expectedPi.outPath homeBoth.config.home.packages == 1;
assert count expectedClaude.outPath homeBoth.config.home.packages == 1;
assert count expectedPi.outPath devenvBoth.packages == 1;
assert count expectedClaude.outPath devenvBoth.packages == 1;
assert paths homePiDisabled.config.home.packages == paths homeClaude.config.home.packages;
assert paths devenvPiDisabled.packages == paths devenvClaude.packages;
pkgs.runCommand "pi-module-api" { } ''
  test -x ${expectedPi}/bin/pi
  test -x ${expectedClaude}/bin/claude
  test ! -e ${expectedPi}/bin/claude
  test ! -e ${expectedClaude}/bin/pi
  touch "$out"
''
