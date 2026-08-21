{ inputs, pkgs, den, darwinPackages }:

let
  mkClaude = den.lib.mkClaude;
  claude = den.packages.claude;
  fence = (import ../lib/fence.nix { inherit pkgs; }).package;
  repowolf = import ../packages/repowolf-client.nix { inherit inputs pkgs; };
  launcher = import ../packages/den-launcher.nix { inherit pkgs; };
  default = den.packages.default;
  protectedPathPatterns = builtins.toJSON (import ../lib/protected-paths.nix);
  fakeGh = pkgs.writeShellScriptBin "gh" "exit 0";
  fakeDocker = pkgs.writeShellScriptBin "docker" "exit 0";
  fakeDockerCompose = pkgs.writeShellScriptBin "docker-compose" "exit 0";
  fakePodman = pkgs.writeShellScriptBin "podman" "exit 0";
  fakePodmanCompose = pkgs.writeShellScriptBin "podman-compose" "exit 0";
  custom = mkClaude {
    configDir = "/tmp/den-claude-config";
    extraPkgs = [ fakeGh ];
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
  defaults = claude.denOptions;
  fails = value: !(builtins.tryEval (builtins.deepSeq value value)).success;
  mkAgentSandbox = import ../lib/mk-agent-sandbox.nix { inherit inputs pkgs; };
  fakeDependency = pkgs.writeShellScriptBin "package-api-dependency" "exit 0";
  dependencies = {
    fence = fakeDependency;
    repoWolfClient = fakeDependency;
    launcher = fakeDependency;
    git = fakeDependency;
    bash = fakeDependency;
    coreutils = fakeDependency;
    acl = fakeDependency;
  };
  mkAdapter = isDarwin: import ../lib/mk-claude.nix {
    fence = fakeDependency;
    mkAgentSandbox = value: value;
    inherit isDarwin pkgs;
  };
  adapter = (mkAdapter false) { };
  darwinAdapter = (mkAdapter true) { };
  darwinFixture = mkAgentSandbox {
    adapter = darwinAdapter.adapter;
    configDir = null;
    extraPkgs = [ ];
    docker = { };
    podman = { };
    inherit dependencies;
  };
in
assert builtins.attrNames den.packages == [ "claude" "default" ];
assert builtins.attrNames den.lib == [ "mkClaude" ];
assert default.outPath == claude.outPath;
assert (mkClaude { }).outPath == claude.outPath;
assert darwinPackages.x86_64.default.outPath == darwinPackages.x86_64.claude.outPath;
assert darwinPackages.aarch64.default.outPath == darwinPackages.aarch64.claude.outPath;
assert !(den.packages ? den);
assert !(den.packages ? claudeResources);
assert !(den.lib ? mkAgentSandbox);
assert !(den.lib ? mkClaudeResourceBundle);
assert !(adapter.adapter ? resourceBundles);
assert !(adapter.adapter ? claudeResources);
assert !(adapter.adapter.agent ? resourceBundles);
assert !(adapter.adapter.agent ? claudeResources);
assert builtins.length darwinAdapter.adapter.closureOnlyPackages == 1;
assert toString (builtins.head darwinAdapter.adapter.closureOnlyPackages) == toString darwinAdapter.adapter.agent.darwinSettings;
assert defaults.configDir == null;
assert defaults.extraPkgs == [ ];
assert defaults.docker.enable == false;
assert defaults.docker.socketPath == null;
assert defaults.docker.hostPorts == [ ];
assert defaults.podman.enable == false;
assert defaults.podman.socketPath == null;
assert defaults.podman.hostPorts == [ ];
assert custom.denOptions.configDir == "/tmp/den-claude-config";
assert custom.denOptions.docker.socketPath == "/tmp/docker.sock";
assert custom.denOptions.docker.hostPorts == [ 2375 2376 ];
assert custom.denOptions.podman.socketPath == "/tmp/podman.sock";
assert custom.denOptions.podman.hostPorts == [ 8080 ];
assert fails ((mkClaude { configDir = "relative"; }).outPath);
assert fails ((mkClaude { configDir = ./.; }).outPath);
assert fails ((mkClaude { extraPkgs = [ "not-a-package" ]; }).outPath);
assert fails ((mkClaude { docker.socketPath = "relative.sock"; }).outPath);
assert fails ((mkClaude { podman.socketPath = "relative.sock"; }).outPath);
assert fails ((mkClaude { docker = { enable = true; hostPorts = [ 0 ]; }; }).outPath);
assert fails ((mkClaude { docker = { enable = true; hostPorts = [ 65536 ]; }; }).outPath);
assert fails ((mkClaude { docker = { enable = true; hostPorts = [ 2375 2375 ]; }; }).outPath);
assert fails ((mkClaude { podman.hostPorts = [ 8080 ]; }).outPath);
assert fails ((mkClaude { dependencies = dependencies; }).outPath);
assert fails ((mkClaude { docker.unexpected = true; }).outPath);
assert fails ((mkClaude { podman.unexpected = true; }).outPath);
assert fails ((mkClaude { resourceBundles = [ ]; }).outPath);
assert fails ((mkClaude { claudeResources = [ ]; }).outPath);
pkgs.runCommand "package-api"
  {
    nativeBuildInputs = [ pkgs.coreutils pkgs.jq ];
    defaultManifest = claude.denManifest;
    customManifest = custom.denManifest;
    darwinFixtureManifest = darwinFixture.denManifest;
    darwinSettings = toString darwinAdapter.adapter.agent.darwinSettings;
  }
  ''
    test "${default.outPath}" = "${claude.outPath}"
    test "${(mkClaude { }).outPath}" = "${claude.outPath}"
    test "${claude.meta.mainProgram}" = claude
    test ! -e ${claude}/bin/den
    test "$(find ${claude}/bin -mindepth 1 -maxdepth 1 -printf '%f\n')" = claude
    grep -Fqx 'exec ${launcher}/bin/den-launcher --manifest ${claude.denManifest} -- "$@"' ${claude}/bin/claude

    expectedProtected='${protectedPathPatterns}'
    jq -e \
      --arg fence "${fence}/bin/fence" \
      --arg repowolf "${repowolf}" \
      --arg acl "${pkgs.acl}/bin/getfacl" \
      --argjson protected "$expectedProtected" \
      '
        .version == 1 and
        .platform == "linux" and
        .scratchRoot == "/tmp" and
        .fenceExecutable == $fence and
        .repoWolfClientDir == $repowolf and
        .aclProbe == [$acl] and
        .protectedPathPatterns == $protected and
        .docker == {enable: false, socketPath: null, hostPorts: [], clientPrograms: []} and
        .podman == {enable: false, socketPath: null, hostPorts: [], clientPrograms: []}
      ' "$defaultManifest"

    defaultClosure=$(jq -r .closurePathsFile "$defaultManifest")
    for root in ${fence} ${repowolf} ${launcher} ${pkgs.gitMinimal} ${pkgs.bash} ${pkgs.coreutils} ${pkgs.acl} ${pkgs.claude-code}; do
      grep -Fqx "$root" "$defaultClosure"
    done

    jq -e \
      --arg repowolf "${repowolf}/bin" \
      --arg repoWolfClient "${repowolf}" \
      --arg fence "${fence}/bin" \
      --arg fenceExecutable "${fence}/bin/fence" \
      --arg acl "${pkgs.acl}/bin/getfacl" \
      --argjson protected "$expectedProtected" \
      --arg git "${pkgs.gitMinimal}/bin" \
      --arg bash "${pkgs.bash}/bin" \
      --arg coreutils "${pkgs.coreutils}/bin" \
      --arg launcher "${launcher}/bin" \
      --arg claude "${pkgs.claude-code}/bin" \
      --arg fakeGh "${fakeGh}/bin" \
      --arg docker "${fakeDocker}/bin/docker" \
      --arg dockerCompose "${fakeDockerCompose}/bin/docker-compose" \
      --arg podman "${fakePodman}/bin/podman" \
      --arg podmanCompose "${fakePodmanCompose}/bin/podman-compose" \
      '
        .version == 1 and
        .platform == "linux" and
        .scratchRoot == "/tmp" and
        .fenceExecutable == $fenceExecutable and
        .repoWolfClientDir == $repoWolfClient and
        .aclProbe == [$acl] and
        .protectedPathPatterns == $protected and
        .explicitConfigDir == "/tmp/den-claude-config" and
        .docker == {
          enable: true,
          socketPath: "/tmp/docker.sock",
          hostPorts: [2375, 2376],
          clientPrograms: [$docker, $dockerCompose]
        } and
        .podman == {
          enable: true,
          socketPath: "/tmp/podman.sock",
          hostPorts: [8080],
          clientPrograms: [$podman, $podmanCompose]
        } and
        .pathEntries == [
          $repowolf, $fence, $git, $bash, $coreutils, $launcher, $claude,
          "${fakeDocker}/bin", "${fakeDockerCompose}/bin", "${fakePodman}/bin", "${fakePodmanCompose}/bin", $fakeGh
        ]
      ' "$customManifest"

    closure=$(jq -r .closurePathsFile "$customManifest")
    for root in ${fence} ${repowolf} ${launcher} ${pkgs.gitMinimal} ${pkgs.bash} ${pkgs.coreutils} ${pkgs.acl} ${pkgs.claude-code} ${fakeGh} ${fakeDocker} ${fakeDockerCompose} ${fakePodman} ${fakePodmanCompose}; do
      grep -Fqx "$root" "$closure"
    done

    darwinFixtureClosure=$(jq -r .closurePathsFile "$darwinFixtureManifest")
    grep -Fqx "$darwinSettings" "$darwinFixtureClosure"
    touch "$out"
  ''
