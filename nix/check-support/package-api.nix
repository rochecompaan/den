{ inputs, pkgs, den, darwinPackages }:

let
  mkClaude = den.lib.mkClaude;
  claude = den.packages.claude;
  fence = (import ../lib/fence.nix { inherit pkgs; }).package;
  repowolf = import ../packages/repowolf-client.nix { inherit inputs pkgs; };
  launcher = import ../packages/den-launcher.nix { inherit pkgs; };
  aclProbeDarwin = import ../packages/den-acl-probe.nix { inherit (pkgs) lib stdenv; };
  default = den.packages.default;
  protectedPathPatterns = builtins.toJSON (import ../lib/protected-paths.nix);
  expectedPlatform = if pkgs.stdenv.isDarwin then "darwin" else "linux";
  expectedScratchRoot = if pkgs.stdenv.isDarwin then "/private/tmp" else "/tmp";
  expectedACLProbe = builtins.toJSON (
    if pkgs.stdenv.isDarwin then [ "${aclProbeDarwin}/bin/den-acl-probe" ] else [ "${pkgs.acl}/bin/getfacl" ]
  );
  aclClosureRoot = if pkgs.stdenv.isDarwin then toString aclProbeDarwin else toString pkgs.acl;
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
    aclProbeDarwin = fakeDependency;
  };
  mkAdapter = isDarwin: import ../lib/mk-claude.nix {
    fence = fakeDependency;
    mkAgentSandbox = value: value;
    inherit isDarwin pkgs;
  };
  adapter = (mkAdapter false) { };
  darwinAdapter = (mkAdapter true) { };
  fakeClaude = (pkgs.writeShellScriptBin "claude" ''
    mkdir -p "$CLAUDE_CAPTURE_DIR"
    printf '%s' "$NODE_EXTRA_CA_CERTS" > "$CLAUDE_CAPTURE_DIR/node-extra-ca-certs"
    printf '%s\0' "$@" > "$CLAUDE_CAPTURE_DIR/args"
    exit 73
  '').overrideAttrs (_: { version = "2.1.158"; });
  fakeAdapter = (import ../lib/mk-claude.nix {
    fence = fakeDependency;
    mkAgentSandbox = value: value;
    isDarwin = false;
    pkgs = pkgs // { claude-code = fakeClaude; };
  }) { };
  darwinFixture = mkAgentSandbox {
    adapter = darwinAdapter.adapter;
    configDir = null;
    extraPkgs = [ ];
    docker = { };
    podman = { };
    inherit dependencies;
  };
  missingPrograms = pkgs.runCommand "missing-mandatory-programs" { } ''
    mkdir -p "$out/bin"
    touch "$out/bin/claude" "$out/bin/docker"
  '';
  extraDocker = pkgs.writeShellScriptBin "docker" "exit 0";
  invalidAgent = mkAgentSandbox {
    adapter = adapter.adapter // {
      runtimePackages = [ missingPrograms ];
      agent = adapter.adapter.agent // { executable = "${missingPrograms}/bin/claude"; };
    };
    configDir = null;
    extraPkgs = [ ];
    docker = { };
    podman = { };
    inherit dependencies;
  };
  invalidDocker = mkAgentSandbox {
    adapter = adapter.adapter;
    configDir = null;
    extraPkgs = [ extraDocker ];
    docker = {
      enable = true;
      package = missingPrograms;
      composePackage = fakeDockerCompose;
    };
    podman = { };
    inherit dependencies;
  };
  expectedBuildFailures = [
    (pkgs.testers.testBuildFailure invalidAgent)
    (pkgs.testers.testBuildFailure invalidDocker)
  ];
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
assert builtins.length adapter.adapter.closureOnlyPackages == 1;
assert toString (builtins.head adapter.adapter.closureOnlyPackages) == adapter.adapter.agent.executable;
assert builtins.length darwinAdapter.adapter.closureOnlyPackages == 2;
assert builtins.elem darwinAdapter.adapter.agent.executable (map toString darwinAdapter.adapter.closureOnlyPackages);
assert builtins.elem (toString darwinAdapter.adapter.agent.darwinSettings) (map toString darwinAdapter.adapter.closureOnlyPackages);
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
    nativeBuildInputs = [ pkgs.coreutils pkgs.jq ] ++ expectedBuildFailures;
    defaultManifest = claude.denManifest;
    customManifest = custom.denManifest;
    darwinFixtureManifest = darwinFixture.denManifest;
    defaultExecutable = adapter.adapter.agent.executable;
    darwinExecutable = darwinAdapter.adapter.agent.executable;
    fakeExecutable = fakeAdapter.adapter.agent.executable;
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
      --arg platform "${expectedPlatform}" \
      --arg scratchRoot "${expectedScratchRoot}" \
      --argjson aclProbe '${expectedACLProbe}' \
      --argjson protected "$expectedProtected" \
      '
        .version == 1 and
        .platform == $platform and
        .scratchRoot == $scratchRoot and
        .fenceExecutable == $fence and
        .repoWolfClientDir == $repowolf and
        .aclProbe == $aclProbe and
        .protectedPathPatterns == $protected and
        .docker == {enable: false, socketPath: null, hostPorts: [], clientPrograms: []} and
        .podman == {enable: false, socketPath: null, hostPorts: [], clientPrograms: []}
      ' "$defaultManifest"

    defaultClosure=$(jq -r .closurePathsFile "$defaultManifest")
    test "$(jq -r .agent.executable "$defaultManifest")" = "$defaultExecutable"
    test "$(jq -r .agent.executable "$defaultManifest")" != "${pkgs.claude-code}/bin/claude"
    for root in ${fence} ${repowolf} ${launcher} ${pkgs.gitMinimal} ${pkgs.bash} ${pkgs.coreutils} ${aclClosureRoot} ${pkgs.claude-code} "$defaultExecutable"; do
      grep -Fqx "$root" "$defaultClosure"
    done

    jq -e \
      --arg repowolf "${repowolf}/bin" \
      --arg repoWolfClient "${repowolf}" \
      --arg fence "${fence}/bin" \
      --arg fenceExecutable "${fence}/bin/fence" \
      --arg platform "${expectedPlatform}" \
      --arg scratchRoot "${expectedScratchRoot}" \
      --argjson aclProbe '${expectedACLProbe}' \
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
        .platform == $platform and
        .scratchRoot == $scratchRoot and
        .fenceExecutable == $fenceExecutable and
        .repoWolfClientDir == $repoWolfClient and
        .aclProbe == $aclProbe and
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
    test "$(jq -r .agent.executable "$customManifest")" = "$defaultExecutable"
    test "$(jq -r .agent.executable "$customManifest")" != "${pkgs.claude-code}/bin/claude"
    for root in ${fence} ${repowolf} ${launcher} ${pkgs.gitMinimal} ${pkgs.bash} ${pkgs.coreutils} ${aclClosureRoot} ${pkgs.claude-code} "$defaultExecutable" ${fakeGh} ${fakeDocker} ${fakeDockerCompose} ${fakePodman} ${fakePodmanCompose}; do
      grep -Fqx "$root" "$closure"
    done

    darwinFixtureClosure=$(jq -r .closurePathsFile "$darwinFixtureManifest")
    test "$(jq -r .agent.executable "$darwinFixtureManifest")" = "$darwinExecutable"
    test "$(jq -r .agent.executable "$darwinFixtureManifest")" != "${pkgs.claude-code}/bin/claude"
    grep -Fqx "$darwinExecutable" "$darwinFixtureClosure"
    grep -Fqx "${pkgs.claude-code}" "$darwinFixtureClosure"
    grep -Fqx "$darwinSettings" "$darwinFixtureClosure"

    capture="$TMPDIR/claude-capture"
    set +e
    CLAUDE_CAPTURE_DIR="$capture" \
      NODE_EXTRA_CA_CERTS="/hostile/inherited-ca.pem" \
      REPOWOLF_CA_FILE="/canonical/launcher-prepared-ca.pem" \
      "$fakeExecutable" "space argument" 'semi;$(not-executed)'
    status=$?
    set -e
    test "$status" = 73
    test "$(cat "$capture/node-extra-ca-certs")" = "/canonical/launcher-prepared-ca.pem"
    printf '%s\0' "space argument" 'semi;$(not-executed)' > expected-args
    cmp expected-args "$capture/args"
    touch "$out"
  ''
