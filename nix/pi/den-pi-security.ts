import { spawnSync } from "node:child_process";
import { lstatSync, realpathSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { isAbsolute, resolve } from "node:path";
import { createBashTool, createLocalBashOperations } from "@earendil-works/pi-coding-agent";

const fenceExecutable = "@fence@/bin/fence";

type Identity = {
  path: string;
  device: bigint;
  inode: bigint;
  size: bigint;
  modified: bigint;
};

function snapshot(path: string): Identity {
  if (!isAbsolute(path) || realpathSync(path) !== resolve(path)) {
    throw new Error("Pi command security input is not canonical");
  }
  const stat = lstatSync(path, { bigint: true });
  if (!stat.isFile() || stat.isSymbolicLink()) {
    throw new Error("Pi command security input is not a regular file");
  }
  return { path, device: stat.dev, inode: stat.ino, size: stat.size, modified: stat.mtimeNs };
}

function unchanged(expected: Identity): boolean {
  try {
    const actual = snapshot(expected.path);
    return actual.device === expected.device && actual.inode === expected.inode &&
      actual.size === expected.size && actual.modified === expected.modified;
  } catch {
    return false;
  }
}

const extensionIdentity = snapshot(fileURLToPath(import.meta.url));
const policyPath = process.env.DEN_FENCE_POLICY_FILE;
if (typeof policyPath !== "string" || policyPath.length === 0) {
  throw new Error("Pi command security policy is unavailable");
}
const policyIdentity = snapshot(policyPath);
const fenceIdentity = snapshot(fenceExecutable);

export function evaluateCommand(command: string, cwd: string): void {
  if (process.env.FENCE_SANDBOX !== "1") {
    throw new Error("Pi command security requires the outer Fence sandbox");
  }
  if (!unchanged(extensionIdentity) || !unchanged(policyIdentity) || !unchanged(fenceIdentity)) {
    throw new Error("Pi command security input changed");
  }
  const request = {
    hook_event_name: "PreToolUse",
    tool_name: "Bash",
    tool_input: { command, cwd },
    cwd,
  };
  const result = spawnSync(
    fenceExecutable,
    ["--claude-pre-tool-use", "--settings", policyPath],
    { input: JSON.stringify(request) + "\n", encoding: "utf8", env: process.env },
  );
  if (result.error || result.signal !== null || result.status !== 0 || result.stdout !== "") {
    throw new Error("Pi command denied by Fence");
  }
}

export default function denPiSecurity(pi: any): void {
  const registered = createBashTool(process.cwd());
  pi.registerTool({
    ...registered,
    execute(toolCallId: string, params: any, signal: AbortSignal | undefined, onUpdate: any, ctx: any) {
      const secured = createBashTool(ctx.cwd, {
        spawnHook(spawnContext) {
          evaluateCommand(spawnContext.command, spawnContext.cwd);
          return spawnContext;
        },
      });
      return secured.execute(toolCallId, params, signal, onUpdate, ctx);
    },
  });

  pi.on("user_bash", () => {
    const local = createLocalBashOperations();
    return {
      operations: {
        exec(command: string, cwd: string, options: any) {
          evaluateCommand(command, cwd);
          return local.exec(command, cwd, options);
        },
      },
    };
  });
}
