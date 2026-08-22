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
  };
  deps = if dependencies == null then productionDependencies else dependencies;
  requiredDependencies = [ "fence" "repoWolfClient" "launcher" "git" "bash" "coreutils" ]
    ++ lib.optional pkgs.stdenv.isLinux "acl";
  adapterRuntimePackages = adapter.runtimePackages or [ ];
  adapterClosureOnlyPackages = adapter.closureOnlyPackages or [ ];
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
  manifest = pkgs.writeText "claude-manifest.json" (builtins.toJSON {
    version = 1;
    platform = if pkgs.stdenv.isDarwin then "darwin" else "linux";
    fenceExecutable = "${deps.fence}/bin/fence";
    repoWolfClientDir = "${deps.repoWolfClient}";
    basePolicy = "${../../policy/fence.json}";
    closurePathsFile = "${closure}/store-paths";
    scratchRoot = if pkgs.stdenv.isDarwin then "/private/tmp" else "/tmp";
    aclProbe = if pkgs.stdenv.isDarwin then [ "/bin/ls" "-lde" ] else [ "${deps.acl}/bin/getfacl" ];
    protectedPathPatterns = import ./protected-paths.nix;
    inherit pathEntries;
    explicitConfigDir = options.configDir;
    agent = adapter.agent;
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
assert lib.assertMsg (builtins.isList adapterRuntimePackages && lib.all lib.isDerivation adapterRuntimePackages)
  "adapter.runtimePackages must be a list of packages";
assert lib.assertMsg (builtins.isList adapterClosureOnlyPackages && lib.all lib.isDerivation adapterClosureOnlyPackages)
  "adapter.closureOnlyPackages must be a list of packages";
assert lib.assertMsg (lib.all (name: builtins.hasAttr name deps && lib.isDerivation deps.${name}) requiredDependencies)
  "mkAgentSandbox dependencies are incomplete";
pkgs.runCommand "claude"
  {
    meta.mainProgram = "claude";
    passthru = {
      denManifest = manifest;
      denOptions = options;
    };
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
    cat > "$out/bin/claude" <<'EOF'
    #!${deps.bash}/bin/bash
    exec ${deps.launcher}/bin/den-launcher --manifest ${manifest} -- "$@"
    EOF
    chmod 0555 "$out/bin/claude"
  ''
