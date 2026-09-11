import { appendFileSync, copyFileSync, existsSync, readFileSync, readdirSync, renameSync, statSync, symlinkSync } from "node:fs";
import { createHash } from "node:crypto";
import childProcess from "node:child_process";
import http from "node:http";
import https from "node:https";
import { syncBuiltinESMExports } from "node:module";
import { basename, join } from "node:path";

function report(line: string) {
  const root = process.env.PI_CODING_AGENT_DIR;
  if (!root) throw new Error("PI_CODING_AGENT_DIR is required");
  appendFileSync(join(root, "pi-resources.report"), `${line}\n`);
}

function replaceSessionPath(target: string, outside: string, marker: string) {
  const held = `${target}.validated`;
  renameSync(target, held);
  if (process.env.DEN_PI_SWAP_KIND === "regular") copyFileSync(outside, target);
  else symlinkSync(outside, target);
  report(marker);
}

async function exercisePackageManager(mode: string) {
  const packageRoot = process.env.PI_PACKAGE_DIR;
  const agentDir = process.env.PI_CODING_AGENT_DIR;
  if (!packageRoot || !agentDir) throw new Error("Pi package fixture environment is required");
  const { DefaultPackageManager } = await import(join(packageRoot, "dist/core/package-manager.js"));
  const sources = ["npm:missing-package", "https://example.invalid/missing.git"];
  const { SettingsManager } = await import(join(packageRoot, "dist/core/settings-manager.js"));
  const settings = SettingsManager.create(process.cwd(), agentDir);
  settings.setProjectTrusted(true);
  if (settings.getGlobalSettings().packages?.length !== 2 || settings.getProjectSettings().packages?.length !== 2) {
    throw new Error("real global and project package declarations were not loaded");
  }
  const manager = new DefaultPackageManager({ cwd: process.cwd(), agentDir, settingsManager: settings });
  if (mode === "changed") process.env.PI_OFFLINE = "0";
  else if (mode === "removed") delete process.env.PI_OFFLINE;
  else throw new Error("unknown package attack mode");

  const npmSource = { type: "npm", spec: "x", name: "x", pinned: false };
  const gitSource = { type: "git", host: "example.invalid", path: "x", repo: "https://example.invalid/x.git", pinned: false };
  const managed = join(agentDir, "managed");
  const marker = join(agentDir, "update-marker");
  const mutations: Array<[string, () => Promise<unknown> | unknown]> = [
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
    ["repairMissingGitDependencies", () => manager.repairMissingGitDependencies(managed)],
    ["cleanAndInstallGitDependencies", () => manager.cleanAndInstallGitDependencies(managed, marker)],
    ["ensureGitRef", () => manager.ensureGitRef(managed, ["fetch"], "HEAD")],
    ["refreshTemporaryGitSource", () => manager.refreshTemporaryGitSource(gitSource, gitSource.repo)],
    ["removeGit", () => manager.removeGit(gitSource, "user")],
    ["pruneEmptyGitParents", () => manager.pruneEmptyGitParents(managed, agentDir)],
    ["ensureNpmProject", () => manager.ensureNpmProject(managed)],
    ["ensureGitIgnore", () => manager.ensureGitIgnore(managed)],
    ["spawnCommand", () => manager.spawnCommand("git", ["status"])],
    ["spawnCaptureCommand", () => manager.spawnCaptureCommand("npm", ["view", "x"])],
    ["runCommandCapture", () => manager.runCommandCapture("git", ["status"])],
    ["runCommand", () => manager.runCommand("git", ["status"])],
    ["runCommandSync", () => manager.runCommandSync("git", ["status"])],
  ];
  // Observe every network/process entry point used by the real manager. Any
  // attempted request fails this test, even if Fence would independently deny it.
  let requests = 0;
  const rejectRequest = () => { requests++; throw new Error("package resolution request attempted"); };
  const saved: Array<[any, string, any]> = [];
  for (const [object, names] of [[childProcess, ["spawn", "spawnSync", "exec", "execSync", "execFile", "execFileSync"]], [http, ["request", "get"]], [https, ["request", "get"]], [globalThis, ["fetch"]]] as const) {
    for (const name of names) { saved.push([object, name, (object as any)[name]]); (object as any)[name] = rejectRequest; }
  }
  syncBuiltinESMExports();
  const paths = [agentDir, join(process.cwd(), ".pi")].flatMap(root => ["settings.json", "packages", "npm", "git"].map(name => join(root, name)));
  const before = packageSnapshot(paths);
  let checks = 0;
  const unchanged = () => {
    if (packageSnapshot(paths) !== before) throw new Error("package operation changed state");
    if (requests !== 0) throw new Error("package operation attempted resolution");
    checks++;
  };
  try {
    for (const [name, mutation] of mutations) {
      let denied = false;
      try { await mutation(); }
      catch (error) {
        if (!String(error).includes("Pi package mutation is disabled by Den")) throw new Error(`${name}: wrong rejection: ${error}`);
        denied = true;
      }
      if (!denied) throw new Error(`${name}: mutation unexpectedly succeeded`);
      unchanged();
    }
    await manager.resolve(); unchanged();
    await manager.resolveExtensionSources(sources); unchanged();
    if ((await manager.checkForAvailableUpdates()).length !== 0) throw new Error("update check was not disabled");
    unchanged();
  } finally {
    for (const [object, name, original] of saved) object[name] = original;
    syncBuiltinESMExports();
  }
  report(`package-attack:${mode}:denied:${mutations.length}`);
  report(`package-resolution:requests:${requests}:state-checks:${checks}`);
}

function packageSnapshot(paths: string[]) {
  const hash = createHash("sha256");
  const visit = (path: string) => {
    hash.update(path);
    if (!existsSync(path)) { hash.update("missing"); return; }
    if (statSync(path).isDirectory()) for (const name of readdirSync(path).sort()) visit(join(path, name));
    else hash.update(readFileSync(path));
  };
  paths.forEach(visit);
  return hash.digest("hex");
}

async function exerciseState() {
  const root = process.env.PI_CODING_AGENT_DIR!;
  const packageRoot = process.env.PI_PACKAGE_DIR!;
  const { AuthStorage } = await import(join(packageRoot, "dist/core/auth-storage.js"));
  const { ProjectTrustStore } = await import(join(packageRoot, "dist/core/trust-manager.js"));
  await AuthStorage.create().modify("native-fixture", async () => ({ type: "api_key", key: "native-fixture-only-key" }));
  new ProjectTrustStore(root).set(process.cwd(), true);
  report("state-probe:credential-and-trust-written");
  let denied = 0;
  for (const [name, home] of [["invoking", process.env.DEN_NATIVE_INVOKING_HOME], ["runtime", process.env.HOME]]) {
    if (!home) throw new Error("synthetic home is required");
    if (readFileSync(join(home, "control.txt"), "utf8") !== "fixture-only host sentinel\n") throw new Error("home control was not readable");
    report(`state-probe:control-readable:${name}`);
    for (const relative of [".pi/agent/auth.json", ".agents/skills/host/SKILL.md"]) {
      let rejected = false;
      try { readFileSync(join(home, relative)); }
      catch (error: any) {
        // Controls and protected sentinels share the granted worktree. The host
        // proves all files exist; Fence hides denied trees on Linux.
        if (!["EACCES", "EPERM", "ENOENT"].includes(error.code)) throw error;
        rejected = true;
      }
      if (!rejected) throw new Error(`protected ${name} home resource was readable`);
      report(`state-probe:denied:${name}:${relative}`);
      denied++;
    }
  }
  report(`state-probe:home-denied:${denied}`);
}

export default async function switchExtension(pi: any) {
  report("extension:switch-extension");
  if (process.env.DEN_PI_OBSERVE_INTERACTIVE_REJECTION) {
    // Pi exits from its interactive fatal handler before the PTY renderer can
    // flush. Observe its actual error, then retain the original fatal behavior.
    const { InteractiveMode } = await import(join(process.env.PI_PACKAGE_DIR!, "dist/modes/interactive/interactive-mode.js"));
    const original = InteractiveMode.prototype.handleFatalRuntimeError;
    InteractiveMode.prototype.handleFatalRuntimeError = function (prefix: string, error: Error) {
      if (error.message === "Pi session target must be an existing regular file") report("session-interactive-error:regular-file");
      return original.call(this, prefix, error);
    };
  }
  if (process.env.DEN_PI_MUTATE_SESSION_ENV) {
    process.env.PI_CODING_AGENT_SESSION_DIR = process.env.DEN_PI_MUTATE_SESSION_ENV;
  }
  pi.on("session_start", (event: { reason: string }, ctx: any) => {
    report(`session-start:${event.reason}:${basename(ctx.cwd)}:${ctx.isProjectTrusted()}:${basename(process.env.PI_CODING_AGENT_SESSION_DIR!)}`);
    for (const entry of ctx.sessionManager.getEntries()) {
      if (entry.type === "custom" && entry.customType === "den-session-fixture" && typeof entry.data?.marker === "string") {
        report(`session-entry:${entry.data.marker}`);
      }
    }
  });
  pi.on("session_before_switch", (event: { targetSessionFile?: string }) => {
    report(`session-before:${basename(event.targetSessionFile ?? "none")}`);
    if (event.targetSessionFile === process.env.DEN_PI_SWAP_TARGET) {
      replaceSessionPath(event.targetSessionFile, process.env.DEN_PI_SWAP_OUTSIDE!, "session-target-swapped");
    }
  });
  pi.on("session_before_fork", () => {
    if (process.env.DEN_PI_FORK_SWAP_TARGET) {
      replaceSessionPath(process.env.DEN_PI_FORK_SWAP_TARGET, process.env.DEN_PI_FORK_SWAP_OUTSIDE!, "session-fork-target-swapped");
    }
  });
  pi.registerCommand("native-switch", {
    description: "Exercise Den session containment",
    handler: async (args: string, ctx: any) => {
      const separator = args.indexOf(" ");
      const mode = separator < 0 ? args : args.slice(0, separator);
      const target = separator < 0 ? "" : args.slice(separator + 1);
      if (mode === "state-probe") { await exerciseState(); return; }
      if (mode === "package-changed" || mode === "package-removed") {
        await exercisePackageManager(mode.slice("package-".length));
        return;
      }
      if (mode === "reject") {
        let rejected = false;
        try { await ctx.switchSession(target); }
        catch (error) {
          const message = String(error);
          if (!message.includes("Pi session target escapes the configured directory") && !message.includes("Pi session target must be an existing regular file")) throw error;
          rejected = true;
          report(`session-rejected:${mode}:${basename(target)}`);
        }
        if (!rejected) throw new Error("invalid extension switch succeeded");
        return;
      }
      if (mode === "swap-persist") {
        let rejected = false;
        try {
          await ctx.switchSession(target, {
            withSession: async (fresh: any) => {
              const markers = fresh.sessionManager.getEntries()
                .filter((entry: any) => entry.type === "custom" && entry.customType === "den-session-fixture")
                .map((entry: any) => entry.data?.marker);
              report(`session-switch-loaded:${markers.join(",")}`);
              fresh.sessionManager.appendCustomEntry("den-session-fixture", { marker: "after-switch-persist" });
            },
          });
        } catch (error) {
          const message = String(error);
          if (!message.includes("Pi session target must be an existing regular file") && !message.includes("Pi session target changed after validation")) throw error;
          rejected = true;
          report("session-swap-persist-rejected:identity");
        }
        if (!rejected) report("session-swap-persist-unexpected-success");
        return;
      }
      if (mode === "fork-replace") {
        let rejected = false;
        try {
          await ctx.fork(target, {
            position: "at",
            withSession: async (fresh: any) => {
              const markers = fresh.sessionManager.getEntries()
                .filter((entry: any) => entry.type === "custom" && entry.customType === "den-session-fixture")
                .map((entry: any) => entry.data?.marker);
              report(`session-fork-loaded:${markers.join(",")}`);
            },
          });
        } catch (error) {
          const message = String(error);
          if (!message.includes("Pi session target must be an existing regular file") && !message.includes("Pi session target changed after validation")) throw error;
          rejected = true;
          report("session-fork-rejected:identity");
        }
        if (!rejected) report("session-fork-unexpected-success");
        return;
      }
      await ctx.switchSession(target, {
        withSession: async (fresh: any) => {
          const cwd = fresh.cwd ?? fresh.sessionManager.getCwd();
          report(`session-loaded:${mode}:${basename(cwd)}`);
        },
      });
    },
  });
}
