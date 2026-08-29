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
  dependencies = {
    fence = fakeFence;
    repoWolfClient = fakeRepoWolfClient;
    inherit launcher;
    git = pkgs.gitMinimal;
    bash = pkgs.bash;
    coreutils = pkgs.coreutils;
    inherit aclProbeDarwin;
  } // pkgs.lib.optionalAttrs pkgs.stdenv.isLinux { acl = pkgs.acl; };
  mkSandbox = { configDir ? null }:
    mkAgentSandbox {
      inherit configDir dependencies;
      extraPkgs = [ ];
      docker = { };
      podman = { };
      adapter = {
        runtimePackages = [ fakeClaude ];
        closureOnlyPackages = pkgs.lib.optionals pkgs.stdenv.isDarwin [ darwinSettings ];
        agent = {
          name = "claude";
          executable = "${fakeClaude}/bin/claude";
          mandatoryArgs = [ "--dangerously-skip-permissions" ]
            ++ pkgs.lib.optionals pkgs.stdenv.isDarwin [ "--settings" darwinSettings ];
          reservedFlags = [ "--settings" "--permission-mode" "--dangerously-skip-permissions" ];
          configEnvironment = "CLAUDE_CONFIG_DIR";
          darwinSettings = pkgs.lib.optionalString pkgs.stdenv.isDarwin darwinSettings;
        };
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
