{ inputs, pkgs, claude }:

let
  fence = import ../lib/fence.nix { inherit pkgs; };
  repowolfClient = import ../packages/repowolf-client.nix { inherit inputs pkgs; };
  launcher = import ../packages/den-launcher.nix { inherit pkgs; };
  packageClosure = pkgs.closureInfo { rootPaths = [ claude ]; };
  fenceCapabilityCheck = import ./fence-capabilities.nix {
    inherit pkgs;
    fence = fence.package;
  };
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
    nativeBuildInputs = [ pkgs.jq pkgs.coreutils pkgs.gnugrep ];
    manifest = claude.denManifest;
    inherit fenceCapabilityCheck packageClosure;
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

    test -e "$fenceCapabilityCheck"

    fail_forbidden() {
      echo 'forbidden production closure content detected' >&2
      exit 1
    }
    while IFS= read -r store_path; do
      base=''${store_path##*/}
      case "$base" in
        *github-cli*|*repowolf-server*|*repowolf-daemon*|*den-credentials*|*private-key*|\
        *user-skill*|*user-plugin*|*offline-fixture*|*context-mode*|*codegraph*|\
        *claude-resource*|*resource-bundle*|*marketplace-data*|*mcp-server*)
          fail_forbidden ;;
      esac
      while IFS= read -r item; do
        case "$item" in
          ${repowolfClient}/bin/gh) ;;
          */bin/gh|*/gh-*|*/bin/den|*/bin/repowolf|*/bin/repowolf-server) fail_forbidden ;;
          */.git-credentials|*/credentials|*/credentials.json|*/id_rsa|*/id_ed25519|\
          */private-key|*/private_key|*/user-skill|*/user-plugin|*/mcp-server|\
          */context-mode|*/codegraph|*/marketplace-data|*/resource-bundle) fail_forbidden ;;
        esac
      done < <(${pkgs.coreutils}/bin/find "$store_path" \( -type f -o -type l \) -print 2>/dev/null)
      if ${pkgs.gnugrep}/bin/grep -R -I -Fq CLAUDE_CODE_PLUGIN_SEED_DIR "$store_path" 2>/dev/null; then
        fail_forbidden
      fi
    done < ${packageClosure}/store-paths

    touch "$out"
  ''
