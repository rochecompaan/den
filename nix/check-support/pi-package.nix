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
assert pi.actualLockHash == builtins.convertHash {
  hash = expected.lockHash;
  toHashFormat = "base16";
};
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
    test "$(${pkgs.jq}/bin/jq -r .name "${pi.packageRoot}/package.json")" = "@earendil-works/pi-coding-agent"
    test "$(${pkgs.jq}/bin/jq -r .version "${pi.packageRoot}/package.json")" = "0.84.4"
    while IFS= read -r dependency; do
      test -f "${pi.packageRoot}/node_modules/$dependency/package.json" || {
        echo "missing direct runtime dependency: $dependency" >&2
        exit 1
      }
    done < <(${pkgs.jq}/bin/jq -r '.dependencies | keys[]' "${pi.packageRoot}/package.json")
    PACKAGE_ROOT="${pi.packageRoot}" "${pi.nodejs}/bin/node" --input-type=module - <<'EOF'
    import { existsSync, readFileSync, realpathSync } from "node:fs";
    import { dirname, join } from "node:path";

    const seen = new Set();
    const resolveDependency = (packageDir, name) => {
      let cursor = packageDir;
      while (true) {
        const candidate = join(cursor, "node_modules", name, "package.json");
        if (existsSync(candidate)) return realpathSync(candidate);
        const parent = dirname(cursor);
        if (parent === cursor) throw new Error(`missing runtime dependency ''${name} required by ''${packageDir}`);
        cursor = parent;
      }
    };
    const visit = (manifestPath) => {
      if (seen.has(manifestPath)) return;
      seen.add(manifestPath);
      const manifest = JSON.parse(readFileSync(manifestPath, "utf8"));
      for (const dependency of Object.keys(manifest.dependencies ?? {})) {
        visit(resolveDependency(dirname(manifestPath), dependency));
      }
    };
    visit(join(process.env.PACKAGE_ROOT, "package.json"));
    EOF
    "${pi.nodejs}/bin/node" -e '
      const [major, minor] = process.versions.node.split(".").map(Number);
      process.exit(major === 22 && minor >= 19 ? 0 : 1);
    '
    test "$("$pi/bin/pi" --version)" = "0.84.4"
    ! ${pkgs.gnugrep}/bin/grep -E '(npm|git|curl|wget|fetch)' "$pi/bin/pi"
    credential=$(cat "$credentialFile")
    ! ${pkgs.gnugrep}/bin/grep -R -F -q -- "$credential" "$pi"

    cat > "$TMPDIR/session-switch.mjs" <<'EOF'
    import { mkdirSync, renameSync, symlinkSync, writeFileSync } from "node:fs";
    import { join } from "node:path";

    const sessionRoot = process.env.PI_CODING_AGENT_SESSION_DIR;
    const trustedCwd = join(process.env.TMPDIR, "trusted-cwd");
    const hostileCwd = join(process.env.TMPDIR, "hostile-cwd");
    mkdirSync(sessionRoot, { recursive: true });
    mkdirSync(trustedCwd);
    mkdirSync(hostileCwd);
    const target = join(sessionRoot, "target.jsonl");
    const validated = join(sessionRoot, "validated.jsonl");
    const outside = join(process.env.TMPDIR, "outside.jsonl");
    const session = (id, cwd) => `''${JSON.stringify({ type: "session", version: 3, id, timestamp: "2026-09-05T00:00:00.000Z", cwd })}\n`;
    writeFileSync(target, session("trusted", trustedCwd));
    writeFileSync(outside, session("hostile", hostileCwd));

    const { AgentSessionRuntime } = await import(process.env.PI_SESSION_RUNTIME);
    let loadedCwd;
    const currentSession = {
      sessionFile: undefined,
      extensionRunner: {
        hasHandlers: (event) => event === "session_before_switch",
        emit: async (event) => {
          if (event.targetSessionFile !== target) throw new Error("before-switch target was not canonical");
          renameSync(target, validated);
          symlinkSync(outside, target);
          return {};
        },
      },
      abort: async () => {},
      dispose: () => {},
    };
    const runtime = new AgentSessionRuntime(
      currentSession,
      { cwd: trustedCwd, agentDir: process.env.TMPDIR },
      async ({ sessionManager }) => {
        loadedCwd = sessionManager.getCwd();
        return { session: currentSession, services: { cwd: loadedCwd, agentDir: process.env.TMPDIR }, diagnostics: [] };
      },
    );
    await runtime.switchSession(target);
    if (loadedCwd !== trustedCwd) {
      throw new Error(`atomic replacement session was read: ''${loadedCwd}`);
    }
    EOF
    PI_CODING_AGENT_SESSION_DIR="$TMPDIR/sessions" \
      PI_SESSION_RUNTIME="${pi.packageRoot}/dist/core/agent-session-runtime.js" \
      "${pi.nodejs}/bin/node" "$TMPDIR/session-switch.mjs"

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
    ${pkgs.coreutils}/bin/touch "$PI_REQUEST_MARKER"
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
      getNpmCommand: () => ["npm"],
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
    const npmSource = { type: "npm", spec: "x", name: "x", pinned: false };
    const gitSource = { type: "git", host: "example.invalid", path: "x", repo: "https://example.invalid/x.git", pinned: false };
    const managedPath = `''${process.env.PI_AGENT}/managed`;
    const markerPath = `''${process.env.PI_AGENT}/update-marker`;
    const mutators = [
      ["addSourceToSettings", () => manager.addSourceToSettings("npm:x")],
      ["removeSourceFromSettings", () => manager.removeSourceFromSettings("npm:x")],
      ["install", () => manager.install("npm:x")],
      ["installAndPersist", () => manager.installAndPersist("npm:x")],
      ["remove", () => manager.remove("npm:x")],
      ["removeAndPersist", () => manager.removeAndPersist("npm:x")],
      ["update", () => manager.update()],
      ["updateConfiguredSources", () => manager.updateConfiguredSources([])],
      ["updateNpmBatch", () => manager.updateNpmBatch([], "user")],
      ["installNpmBatch", () => manager.installNpmBatch([], "user")],
      ["installParsedSource", () => manager.installParsedSource(npmSource, "user")],
      ["runNpmCommand", () => manager.runNpmCommand(["view", "x"])],
      ["runNpmCommandSync", () => manager.runNpmCommandSync(["view", "x"])],
      ["installNpm", () => manager.installNpm(npmSource, "user")],
      ["uninstallNpm", () => manager.uninstallNpm(npmSource, "user")],
      ["installGit", () => manager.installGit(gitSource, "user")],
      ["updateGit", () => manager.updateGit(gitSource, "user")],
      ["repairMissingGitDependencies", () => manager.repairMissingGitDependencies(managedPath)],
      ["cleanAndInstallGitDependencies", () => manager.cleanAndInstallGitDependencies(managedPath, markerPath)],
      ["ensureGitRef", () => manager.ensureGitRef(managedPath, ["fetch"], "HEAD")],
      ["refreshTemporaryGitSource", () => manager.refreshTemporaryGitSource(gitSource, gitSource.repo)],
      ["removeGit", () => manager.removeGit(gitSource, "user")],
      ["pruneEmptyGitParents", () => manager.pruneEmptyGitParents(managedPath, process.env.PI_AGENT)],
      ["ensureNpmProject", () => manager.ensureNpmProject(managedPath)],
      ["ensureGitIgnore", () => manager.ensureGitIgnore(managedPath)],
      ["spawnCommand", () => manager.spawnCommand("git", ["status"])],
      ["spawnCaptureCommand", () => manager.spawnCaptureCommand("npm", ["view", "x"])],
      ["runCommandCapture", () => manager.runCommandCapture("git", ["status"])],
      ["runCommand", () => manager.runCommand("git", ["status"])],
      ["runCommandSync", () => manager.runCommandSync("git", ["status"])],
    ];
    for (const offlineMode of ["changed", "deleted"]) {
      if (offlineMode === "changed") process.env.PI_OFFLINE = "0";
      else delete process.env.PI_OFFLINE;
      for (const [name, operation] of mutators) {
        await reject(`''${offlineMode}:''${name}`, operation);
      }
    }
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
