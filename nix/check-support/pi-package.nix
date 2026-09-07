{ pkgs }:

let
  pi = import ../packages/pi-coding-agent.nix { inherit pkgs; };
  inherit (pkgs) lib;
  expected = {
    tarballHash = "sha256-W852bRnDzroY8/uq2RxEnJ+dc5gfnjQA7O+TIAbwaWg=";
    lockHash = "sha256-/xfQaHHRD9Riiv+hqSHfFzvx+GBeByzCqgpO5Oi0cc4=";
    npmDepsHash = "sha256-rSUYLw/RoIZ2f6gMwpSUdDqGECFUnm0KnNu/uCLYbpE=";
  };
  hostileExtension = ./fixtures/pi/hostile-package-extension.ts;
  credentialFile = pkgs.writeText "pi-package-credential-fixture" "den-pi-credential-must-not-enter-derivation";
in
assert pi.pname == "pi-coding-agent";
assert pi.version == "0.84.4";
assert lib.versionAtLeast pi.nodejs.version "22.19.0";
assert pi.tarballHash == expected.tarballHash;
assert pi.lockHash == expected.lockHash;
assert pi.npmDepsHash == expected.npmDepsHash;
assert pi.actualPatchHash == builtins.convertHash {
  hash = pi.patchHash;
  toHashFormat = "base16";
};
pkgs.runCommand "pi-package"
  {
    nativeBuildInputs = [ pkgs.coreutils pkgs.findutils pkgs.gnugrep pkgs.jq ];
    inherit pi hostileExtension credentialFile;
  }
  ''
    set -euo pipefail

    test -x "$pi/bin/pi"
    test -f "${pi.packageRoot}/package.json"
    test -f "${pi.packageRoot}/dist/cli.js"
    test -f "${pi.packageRoot}/dist/main.js"
    test -d "${pi.packageRoot}/node_modules"
    test -f "${pi.packageRoot}/dist/modes/interactive/assets/clankolas.png"
    test -f "${pi.packageRoot}/dist/modes/interactive/theme/dark.json"
    test -e "${pi.packageRoot}/node_modules/@earendil-works/pi-ai"
    test -e "${pi.packageRoot}/node_modules/@earendil-works/pi-tui"
    "${pi.nodejs}/bin/node" -e '
      const [major, minor] = process.versions.node.split(".").map(Number);
      process.exit(major === 22 && minor >= 19 ? 0 : 1);
    '
    test "$("$pi/bin/pi" --version)" = "0.84.4"
    ! ${pkgs.gnugrep}/bin/grep -E '(npm|git|curl|wget|fetch)' "$pi/bin/pi"
    credential=$(cat "$credentialFile")
    ! ${pkgs.gnugrep}/bin/grep -R -F -q -- "$credential" "$pi"

    state="$TMPDIR/state"
    project="$TMPDIR/project"
    mkdir -p "$state" "$project/.pi"
    printf '{"packages":["npm:missing-package","https://example.invalid/missing.git"]}\n' \
      > "$state/settings.json"
    printf '{"packages":["npm:missing-package","https://example.invalid/missing.git"]}\n' \
      > "$project/.pi/settings.json"
    before_settings=$(tar -C "$TMPDIR" -cf - state project | sha256sum)

    for command in install remove uninstall update list config; do
      marker="$TMPDIR/$command.marker"
      if env -i HOME="$TMPDIR/home" PATH= PI_CODING_AGENT_DIR="$state" \
        PI_HOSTILE_MARKER="$marker" "$pi/bin/pi" "$command" --extension "$hostileExtension" \
        >"$TMPDIR/$command.out" 2>&1; then
        echo "package command unexpectedly succeeded: $command" >&2
        exit 1
      fi
      test ! -e "$marker"
      ${pkgs.gnugrep}/bin/grep -Fq 'disabled by Den' "$TMPDIR/$command.out"
    done

    mkdir -p "$TMPDIR/bin"
    cat > "$TMPDIR/bin/npm" <<'EOF'
    #!${pkgs.runtimeShell}
    touch "$PI_REQUEST_MARKER"
    exit 97
    EOF
    cp "$TMPDIR/bin/npm" "$TMPDIR/bin/git"
    chmod +x "$TMPDIR/bin/npm" "$TMPDIR/bin/git"
    export PI_REQUEST_MARKER="$TMPDIR/network-requested"

    cat > "$TMPDIR/package-manager.mjs" <<'EOF'
    const { DefaultPackageManager } = await import(process.env.PI_PACKAGE_MANAGER);

    const settings = {
      getGlobalSettings: () => ({ packages: ["npm:missing-package", "https://example.invalid/missing.git"] }),
      getProjectSettings: () => ({ packages: ["npm:missing-package", "https://example.invalid/missing.git"] }),
      setPackages: () => { throw new Error("settings mutation escaped"); },
      setProjectPackages: () => { throw new Error("settings mutation escaped"); },
      isProjectTrusted: () => true,
    };
    const manager = new DefaultPackageManager({ cwd: process.env.PI_PROJECT, agentDir: process.env.PI_AGENT, settingsManager: settings });
    const reject = async (name, operation) => {
      try {
        await operation();
      } catch (error) {
        if (String(error).includes("disabled by Den")) return;
        throw new Error(`''${name}: wrong error: ''${error}`);
      }
      throw new Error(`''${name}: mutation unexpectedly succeeded`);
    };
    process.env.PI_OFFLINE = "0";
    await reject("addSourceToSettings", () => manager.addSourceToSettings("npm:x"));
    await reject("removeSourceFromSettings", () => manager.removeSourceFromSettings("npm:x"));
    await reject("install", () => manager.install("npm:x"));
    await reject("installAndPersist", () => manager.installAndPersist("npm:x"));
    delete process.env.PI_OFFLINE;
    await reject("remove", () => manager.remove("npm:x"));
    await reject("removeAndPersist", () => manager.removeAndPersist("npm:x"));
    await reject("update", () => manager.update());
    await reject("installParsedSource", () => manager.installParsedSource({ type: "npm", name: "x" }, "user"));
    await reject("installNpm", () => manager.installNpm({ type: "npm", name: "x" }, "user"));
    await reject("installGit", () => manager.installGit({ type: "git" }, "user"));
    await reject("uninstallNpm", () => manager.uninstallNpm({ type: "npm", name: "x" }, "user"));
    await reject("removeGit", () => manager.removeGit({ type: "git" }, "user"));
    await manager.resolve();
    await manager.resolveExtensionSources(["npm:missing-package", "https://example.invalid/missing.git"]);
    if ((await manager.checkForAvailableUpdates()).length !== 0) throw new Error("update check was not disabled");
    EOF
    PATH="$TMPDIR/bin" PI_PROJECT="$project" PI_AGENT="$state" \
      PI_PACKAGE_MANAGER="${pi.packageRoot}/dist/core/package-manager.js" \
      "${pi.nodejs}/bin/node" "$TMPDIR/package-manager.mjs"
    test ! -e "$TMPDIR/network-requested"
    test "$(tar -C "$TMPDIR" -cf - state project | sha256sum)" = "$before_settings"

    env -i HOME="$TMPDIR/home" PATH= PI_CODING_AGENT_DIR="$state" \
      "$pi/bin/pi" --version > "$TMPDIR/empty-path-version"
    test "$(cat "$TMPDIR/empty-path-version")" = "0.84.4"
    env -i HOME="$TMPDIR/home" PI_CODING_AGENT_DIR="$state" \
      ${pkgs.coreutils}/bin/timeout 10 "$pi/bin/pi" --mode rpc \
      < /dev/null > "$TMPDIR/rpc.out" 2>&1
    touch "$out"
  ''
