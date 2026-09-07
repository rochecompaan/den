{ inputs, pkgs }:

{ adapter, configDir, extraPkgs, docker, podman, dependencies ? null }:
let
  lib = pkgs.lib;
  options = import ./options.nix { inherit pkgs; } {
    inherit configDir extraPkgs docker podman;
  };
  productionDependencies = let
    fence = import ./fence.nix { inherit pkgs; };
  in {
    fence = fence.package;
    repoWolfClient = import ../packages/repowolf-client.nix { inherit inputs pkgs; };
    launcher = import ../packages/den-launcher.nix { inherit pkgs; };
    git = pkgs.gitMinimal;
    bash = pkgs.bash;
    coreutils = pkgs.coreutils;
  } // lib.optionalAttrs pkgs.stdenv.isLinux {
    acl = pkgs.acl;
  } // lib.optionalAttrs pkgs.stdenv.isDarwin {
    aclProbeDarwin = import ../packages/den-acl-probe.nix { inherit (pkgs) lib stdenv; };
  };
  deps = if dependencies == null then productionDependencies else dependencies;
  requiredDependencies = [ "fence" "repoWolfClient" "launcher" "git" "bash" "coreutils" ]
    ++ lib.optional pkgs.stdenv.isLinux "acl"
    ++ lib.optional pkgs.stdenv.isDarwin "aclProbeDarwin";
  adapterRuntimePackages = adapter.runtimePackages;
  adapterClosureOnlyPackages = adapter.closureOnlyPackages;
  output = adapter.output;
  agent = builtins.intersectAttrs {
    name = null;
    executable = null;
    argumentPolicy = null;
    mandatoryArgs = null;
    resourceArgs = null;
    reservedFlags = null;
    reservedCommands = null;
    environment = null;
    packageDirectory = null;
    securityAdapter = null;
  } adapter.agent // { commandName = output.commandName; };
  stateBindings = adapter.stateBindings;
  safeBasename = name: builtins.isString name && name != "" && name != "." && name != ".."
    && !(lib.hasInfix "/" name) && !(lib.hasInfix "\n" name) && !(lib.hasInfix "\r" name);
  dockerPackages = lib.optionals options.docker.enable [
    options.docker.package
    options.docker.composePackage
  ];
  podmanPackages = lib.optionals options.podman.enable [
    options.podman.package
    options.podman.composePackage
  ];
  clientPrograms = config: packages: names:
    if config.enable then lib.zipListsWith (package: name: "${package}/bin/${name}") packages names else [ ];
  dockerClientPrograms = clientPrograms options.docker dockerPackages [ "docker" "docker-compose" ];
  podmanClientPrograms = clientPrograms options.podman podmanPackages [ "podman" "podman-compose" ];
  requiredPrograms = [ adapter.agent.executable ] ++ dockerClientPrograms ++ podmanClientPrograms;
  closureRoots = (map (name: deps.${name}) requiredDependencies)
    ++ adapterRuntimePackages ++ adapterClosureOnlyPackages
    ++ dockerPackages ++ podmanPackages ++ options.extraPkgs;
  closure = pkgs.closureInfo {
    rootPaths = closureRoots;
  };
  pathEntries = map (package: "${package}/bin") [
    deps.repoWolfClient
    deps.fence
    deps.git
    deps.bash
    deps.coreutils
    deps.launcher
  ] ++ map (package: "${package}/bin") adapterRuntimePackages
    ++ map (package: "${package}/bin") dockerPackages
    ++ map (package: "${package}/bin") podmanPackages
    ++ map (package: "${package}/bin") options.extraPkgs;
  manifest = pkgs.writeText output.manifestName (builtins.toJSON {
    version = 2;
    platform = if pkgs.stdenv.isDarwin then "darwin" else "linux";
    fenceExecutable = "${deps.fence}/bin/fence";
    repoWolfClientDir = "${deps.repoWolfClient}";
    basePolicy = "${../../policy/fence.json}";
    closurePathsFile = "${closure}/store-paths";
    scratchRoot = if pkgs.stdenv.isDarwin then "/private/tmp" else "/tmp";
    aclProbe = if pkgs.stdenv.isDarwin then [ "${deps.aclProbeDarwin}/bin/den-acl-probe" ] else [ "${deps.acl}/bin/getfacl" ];
    protectedPathPatterns = lib.unique ((import ./protected-paths.nix) ++ (adapter.protectedPathPatterns or [ ]));
    inherit pathEntries;
    inherit stateBindings;
    agent = agent;
    docker = {
      inherit (options.docker) enable socketPath hostPorts;
      clientPrograms = dockerClientPrograms;
    };
    podman = {
      inherit (options.podman) enable socketPath hostPorts;
      clientPrograms = podmanClientPrograms;
    };
  });
in
assert lib.assertMsg (builtins.isAttrs adapter && adapter ? agent) "adapter must provide an agent";
assert lib.assertMsg (builtins.isAttrs output) "adapter must provide output";
assert lib.assertMsg (safeBasename output.packageName) "output.packageName must be a safe basename";
assert lib.assertMsg (safeBasename output.commandName) "output.commandName must be a safe basename";
assert lib.assertMsg (safeBasename output.manifestName && lib.hasSuffix ".json" output.manifestName)
  "output.manifestName must be a safe .json basename";
assert lib.assertMsg (safeBasename output.mainProgram) "output.mainProgram must be a safe basename";
assert lib.assertMsg (builtins.isList adapterRuntimePackages && lib.all lib.isDerivation adapterRuntimePackages)
  "adapter.runtimePackages must be a list of packages";
assert lib.assertMsg (builtins.isList adapterClosureOnlyPackages && lib.all lib.isDerivation adapterClosureOnlyPackages)
  "adapter.closureOnlyPackages must be a list of packages";
assert lib.assertMsg (lib.all (name: builtins.hasAttr name deps && lib.isDerivation deps.${name}) requiredDependencies)
  "mkAgentSandbox dependencies are incomplete";
pkgs.runCommand output.packageName
  {
    commandName = output.commandName;
    meta.mainProgram = output.mainProgram;
    passthru = {
      denManifest = manifest;
      denOptions = options;
    } // (adapter.passthru or { });
  }
  ''
    for program in ${lib.escapeShellArgs requiredPrograms}; do
      case "$program" in
        /nix/store/*) ;;
        *) echo "mandatory program path is not an absolute store path" >&2; exit 1 ;;
      esac
      if [ ! -f "$program" ] || [ ! -x "$program" ]; then
        echo "mandatory program is not an executable regular file" >&2
        exit 1
      fi
    done
    mkdir -p "$out/bin"
    cat > "$out/bin/$commandName" <<'EOF'
    #!${deps.bash}/bin/bash
    exec ${deps.launcher}/bin/den-launcher --manifest ${manifest} -- "$@"
    EOF
    chmod 0555 "$out/bin/$commandName"
  ''
