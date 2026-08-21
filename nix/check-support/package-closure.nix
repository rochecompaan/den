{ inputs, pkgs, claude }:

let
  fence = import ../lib/fence.nix { inherit pkgs; };
  repowolfClient = import ../packages/repowolf-client.nix { inherit inputs pkgs; };
  launcher = import ../packages/den-launcher.nix { inherit pkgs; };
  productionAdapter = (import ../lib/mk-claude.nix {
    inherit pkgs;
    fence = fence.package;
    isDarwin = pkgs.stdenv.isDarwin;
    mkAgentSandbox = value: value;
  }) { };
  darwinAdapter = (import ../lib/mk-claude.nix {
    inherit pkgs;
    fence = fence.package;
    isDarwin = true;
    mkAgentSandbox = value: value;
  }) { };
  darwinSettings = darwinAdapter.adapter.agent.darwinSettings;
in
assert fence.version == "0.1.58";
assert builtins.all (value: value) (builtins.attrValues fence.capabilities);
assert !(productionAdapter.adapter ? resourceBundles);
assert !(productionAdapter.adapter.agent ? skills);
assert !(productionAdapter.adapter.agent ? plugins);
assert !(productionAdapter.adapter.agent ? mcp);
assert !(productionAdapter.adapter.agent ? contextMode);
assert !(productionAdapter.adapter.agent ? codeGraph);
assert !(productionAdapter.adapter.agent ? seedDirectory);
pkgs.runCommand "package-closure"
  {
    nativeBuildInputs = [ pkgs.jq ];
    manifest = claude.denManifest;
  }
  ''
    set -eu
    test -x ${claude}/bin/claude
    test ! -e ${claude}/bin/den
    test "$(find ${claude}/bin -mindepth 1 -maxdepth 1 -printf '%f\n')" = claude

    jq -e \
      --arg fence "${fence.package}/bin/fence" \
      --arg repowolf "${repowolfClient}" \
      --arg policy "${../../policy/fence.json}" '
        .version == 1 and
        .fenceExecutable == $fence and
        .repoWolfClientDir == $repowolf and
        .basePolicy == $policy and
        (.pathEntries[0] == ($repowolf + "/bin"))
      ' "$manifest"

    closure=$(jq -r .closurePathsFile "$manifest")
    for required in \
      ${fence.package} ${repowolfClient} ${pkgs.gitMinimal} ${pkgs.bash} \
      ${pkgs.coreutils} ${launcher} ${pkgs.claude-code}; do
      grep -Fqx "$required" "$closure"
    done
    ${pkgs.lib.optionalString pkgs.stdenv.isLinux ''grep -Fqx "${pkgs.acl}" "$closure"''}

    ${fence.package}/bin/fence --version > fence-version
    grep -Fq 'Version: 0.1.58' fence-version
    test -r ${../../policy/fence.json}
    jq -e '.allowPty == true and .filesystem.strictDenyRead == true' ${../../policy/fence.json}

    test -x ${repowolfClient}/bin/repowolf-client
    test "$(readlink ${repowolfClient}/bin/gh)" = repowolf-client
    test "$(readlink ${repowolfClient}/bin/repowolf-git-ssh)" = repowolf-client
    test ! -e ${repowolfClient}/bin/repowolf

    # Exercise the store-managed macOS settings constructor on every host; on
    # Darwin the same artifact is also a direct production closure root.
    test -r ${darwinSettings}
    jq -e --arg fence "${fence.package}/bin/fence" '
      .hooks.PreToolUse == [{
        matcher: "Bash",
        hooks: [{
          type: "command",
          command: ($fence + " --claude-pre-tool-use --settings \"$DEN_FENCE_POLICY_FILE\"")
        }]
      }]
    ' ${darwinSettings}
    ${pkgs.lib.optionalString pkgs.stdenv.isDarwin ''grep -Fqx "${darwinSettings}" "$closure"''}

    # Inspect only store-path identities, never file contents, for forbidden
    # production closure classes. This keeps credential/path data out of logs.
    while IFS= read -r store_path; do
      base=''${store_path##*/}
      case "$base" in
        *github-cli*|*repowolf-server*|*repowolf-daemon*|*den-credentials*|*private-key*|\
        *user-skill*|*user-plugin*|*offline-fixture*|*context-mode*|*codegraph*|\
        *claude-resource*|*resource-bundle*|*marketplace-data*|*mcp-server*)
          echo 'forbidden production closure class detected' >&2
          exit 1
          ;;
      esac
    done < "$closure"

    # The package and immutable adapter have no resource-seeding environment.
    if grep -Fq CLAUDE_CODE_PLUGIN_SEED_DIR ${claude}/bin/claude "$manifest" ${darwinSettings}; then
      echo 'plugin seed environment leaked into generated artifacts' >&2
      exit 1
    fi

    touch "$out"
  ''
