{ inputs, pkgs }:

let
  fakeFence = import ./fake-fence.nix { inherit pkgs; };
  fakeClaude = import ./fake-claude.nix { inherit pkgs; };
  fakeRepoWolfClient = import ./fake-repowolf-client.nix { inherit pkgs; };
  launcher = import ../packages/den-launcher.nix { inherit pkgs; };
  aclProbeDarwin = import ../packages/den-acl-probe.nix { inherit (pkgs) lib stdenv; };
  mkAgentSandbox = import ../lib/mk-agent-sandbox.nix { inherit inputs pkgs; };
  darwinAdapter = (import ../lib/mk-claude.nix {
    inherit pkgs;
    fence = fakeFence;
    isDarwin = true;
    mkAgentSandbox = value: value;
  }) { };
  darwinSettings = darwinAdapter.adapter.agent.darwinSettings;
  fenceSettings = {
    hooks.PreToolUse = [{
      matcher = "Bash";
      hooks = [{
        type = "command";
        command = "${fakeFence}/bin/fence --claude-pre-tool-use --settings \"$DEN_FENCE_POLICY_FILE\"";
      }];
    }];
  };
  dependencies = {
    fence = fakeFence;
    repoWolfClient = fakeRepoWolfClient;
    inherit launcher;
    git = pkgs.gitMinimal;
    bash = pkgs.bash;
    coreutils = pkgs.coreutils;
    inherit aclProbeDarwin;
  } // pkgs.lib.optionalAttrs pkgs.stdenv.isLinux { acl = pkgs.acl; };
  mkSandbox = { configDir ? null, resources ? { }, bundles ? [ ] }:
    let
      mergedResources = import ../lib/den-resources.nix { inherit pkgs; } {
        agent = "claude";
        inherit bundles;
        resources = {
          skills = [ ];
          plugins = [ ];
          mcpServers = { };
          settings = [ ];
        } // resources;
      };
      normalizedResources = import ../lib/claude-resources.nix { inherit pkgs; } {
        resources = mergedResources;
        extraPkgs = [ ];
        baseSettings = if pkgs.stdenv.isDarwin then fenceSettings else null;
      };
    in
    mkAgentSandbox {
      inherit configDir dependencies;
      extraPkgs = [ ];
      docker = { };
      podman = { };
      adapter = {
        runtimePackages = [ fakeClaude ];
        closureOnlyPackages = pkgs.lib.optionals pkgs.stdenv.isDarwin [ normalizedResources.settingsFile ]
          ++ normalizedResources.closureInputs;
        output = {
          packageName = "claude";
          commandName = "claude";
          manifestName = "claude-manifest.json";
          mainProgram = "claude";
        };
        agent = {
          name = "claude";
          executable = "${fakeClaude}/bin/claude";
          argumentPolicy = "claude";
          mandatoryArgs = [ "--dangerously-skip-permissions" ];
          resourceArgs = normalizedResources.resourceArgs;
          reservedFlags = [ "--settings" "--permission-mode" "--dangerously-skip-permissions" "--plugin-dir" "--mcp-config" "--strict-mcp-config" "--setting-sources" ];
          reservedCommands = [ ];
          environment = { scrub = [ ]; set = { }; };
          packageDirectory = null;
          securityAdapter = if pkgs.stdenv.isDarwin then {
            kind = "claude-settings";
            path = normalizedResources.settingsFile;
            arguments = [ "--settings" normalizedResources.settingsFile ];
          } else null;
        };
        stateBindings = [{
          name = "config";
          explicitPath = configDir;
          inheritedEnvironment = "CLAUDE_CONFIG_DIR";
          defaultPath = "";
          defaultWritablePaths = [ ];
          exports = [{ kind = "environment"; name = "CLAUDE_CONFIG_DIR"; exportDefault = false; }];
        }];
      };
    };
  overrideManifest = { name, package, filter }:
    pkgs.runCommand name { nativeBuildInputs = [ pkgs.jq ]; } ''
      mkdir -p "$out/bin"
      jq '${filter}' ${package.denManifest} > "$out/manifest.json"
      cat > "$out/bin/claude" <<'EOF'
      #!${pkgs.bash}/bin/bash
      exec ${launcher}/bin/den-launcher --manifest PLACEHOLDER -- "$@"
      EOF
      substituteInPlace "$out/bin/claude" --replace-fail PLACEHOLDER "$out/manifest.json"
      chmod 0555 "$out/bin/claude"
    '';
in
{
  inherit darwinSettings dependencies fakeClaude fakeFence fakeRepoWolfClient launcher mkSandbox overrideManifest;
}
