{ inputs, pkgs, claude }:

let
  fence = import ../lib/fence.nix { inherit pkgs; };
  repowolfClient = import ../packages/repowolf-client.nix { inherit inputs pkgs; };
  launcher = import ../packages/den-launcher.nix { inherit pkgs; };
  aclProbeDarwin = if pkgs.stdenv.isDarwin
    then import ../packages/den-acl-probe.nix { inherit (pkgs) lib stdenv; }
    else null;
  expectedACLProbe = if pkgs.stdenv.isDarwin then "${aclProbeDarwin}/bin/den-acl-probe" else "";
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
    nativeBuildInputs = [ pkgs.jq pkgs.coreutils pkgs.findutils pkgs.gnugrep ];
    manifest = claude.denManifest;
    inherit fenceCapabilityCheck packageClosure;
  }
  ''
    set -eu
    test -x ${claude}/bin/claude
    test ! -e ${claude}/bin/den
    test "$(${pkgs.findutils}/bin/find ${claude}/bin -mindepth 1 -maxdepth 1 -printf '%f\n')" = claude

    jq -e \
      --arg fence "${fence.package}/bin/fence" \
      --arg repowolf "${repowolfClient}" \
      --arg aclProbe "${expectedACLProbe}" \
      --arg policy "${../../policy/fence.json}" '
        .version == 1 and
        .fenceExecutable == $fence and
        .repoWolfClientDir == $repowolf and
        .basePolicy == $policy and
        (if .platform == "darwin" then .aclProbe == [$aclProbe] else true) and
        (.pathEntries[0] == ($repowolf + "/bin"))
      ' "$manifest"

    closure=$(jq -r .closurePathsFile "$manifest")
    for required in \
      ${fence.package} ${repowolfClient} ${pkgs.gitMinimal} ${pkgs.bash} \
      ${pkgs.coreutils} ${launcher} ${pkgs.claude-code}; do
      grep -Fqx "$required" "$closure"
    done
    ${pkgs.lib.optionalString pkgs.stdenv.isLinux ''grep -Fqx "${pkgs.acl}" "$closure"''}
    ${pkgs.lib.optionalString pkgs.stdenv.isDarwin ''grep -Fqx "${aclProbeDarwin}" "$closure"''}

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
    store_class_forbidden() {
      case "$1" in
        *github-cli*|*repowolf-server*|*repowolf-daemon*|*den-credentials*|*private-key*|\
        *user-skill*|*user-plugin*|*offline-fixture*|*context-mode*|*codegraph*|\
        *claude-resource*|*resource-bundle*|*marketplace-data*|*mcp-server*) return 0 ;;
        *) return 1 ;;
      esac
    }
    artifact_forbidden() {
      case "$1" in
        ${repowolfClient}/bin/gh) return 1 ;;
        */bin/gh|*/gh-*|*/bin/den|*/bin/repowolf|*/bin/repowolf-server) return 0 ;;
        */.git-credentials|*/credentials|*/credentials.json|*/id_rsa|*/id_ed25519|\
        */private-key|*/private_key|*/user-skill|*/user-plugin|*/mcp-server|\
        */context-mode|*/codegraph|*/marketplace-data|*/resource-bundle) return 0 ;;
        *) return 1 ;;
      esac
    }
    contains_seed_variable() {
      test -f "$1" && test -r "$1" && \
        ${pkgs.gnugrep}/bin/grep -a -Fq CLAUDE_CODE_PLUGIN_SEED_DIR "$1" 2>/dev/null
    }

    negative="$TMPDIR/closure-negative-fixture"
    mkdir -p "$negative/bin"
    : > "$negative/bin/den"
    artifact_forbidden "$negative/bin/den" || fail_forbidden
    if artifact_forbidden ${repowolfClient}/bin/gh; then fail_forbidden; fi
    printf '\000CLAUDE_CODE_PLUGIN_SEED_DIR\000' > "$negative/binary-seed"
    contains_seed_variable "$negative/binary-seed" || fail_forbidden
    newline_root="$negative/newline-root"
    mkdir -p "$newline_root"
    newline_file="$newline_root/seed
fixture"
    printf '\000CLAUDE_CODE_PLUGIN_SEED_DIR\000' > "$newline_file"
    newline_artifacts="$negative/newline-artifacts"
    if ! ${pkgs.findutils}/bin/find "$newline_root" -type f -print0 > "$newline_artifacts"; then
      echo 'closure fixture traversal failed' >&2
      exit 1
    fi
    newline_detected=0
    while IFS= read -r -d "" item; do
      if contains_seed_variable "$item"; then newline_detected=1; fi
    done < "$newline_artifacts"
    test "$newline_detected" = 1 || fail_forbidden

    artifacts="$TMPDIR/package-closure-artifacts"
    : > "$artifacts"
    store_count=0
    while IFS= read -r store_path; do
      test -n "$store_path" || continue
      store_count=$((store_count + 1))
      base=''${store_path##*/}
      if store_class_forbidden "$base"; then fail_forbidden; fi
      if ! ${pkgs.findutils}/bin/find "$store_path" \( -type f -o -type l \) -print0 >> "$artifacts"; then
        echo 'production closure traversal failed' >&2
        exit 1
      fi
    done < ${packageClosure}/store-paths
    test "$store_count" -gt 0
    test -s "$artifacts"

    for generated in ${claude}/bin/claude "$manifest" ${darwinSettings}; do
      if contains_seed_variable "$generated"; then fail_forbidden; fi
    done

    artifact_count=0
    while IFS= read -r -d "" item; do
      artifact_count=$((artifact_count + 1))
      if artifact_forbidden "$item"; then fail_forbidden; fi
      case "$item" in
        ${pkgs.claude-code}/*) ;;
        *) if contains_seed_variable "$item"; then fail_forbidden; fi ;;
      esac
    done < "$artifacts"
    test "$artifact_count" -gt 0

    touch "$out"
  ''
