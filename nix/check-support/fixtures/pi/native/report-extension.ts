import { appendFileSync } from "node:fs";
import { basename, join } from "node:path";

const label = (path: string) => basename(path).replace(/^[a-z0-9]{32}-/, "").replace(/\.md$/, "");
const group = (path: string) => path.includes("den-native-pi-resource-package") ? "package" : path.startsWith("/nix/store/") ? "direct" : "ambient";

export default async function reportExtension(pi: any) {
  const root = process.env.PI_CODING_AGENT_DIR;
  if (!root || !process.env.PI_PACKAGE_DIR) throw new Error("Pi fixture environment is required");
  const report = (line: string) => appendFileSync(join(root, "pi-resources.report"), `${line}\n`);
  report("extension:report-extension");
  pi.registerTool({ name: "native-collision", label: "Collision", description: "Direct winner", parameters: { type: "object", properties: {} }, execute: async () => ({ content: [] }) });

  // Observe the loader used by this actual CLI runtime, not a second loader or
  // a reconstructed manifest inventory. The getter's result is never changed.
  const { DefaultResourceLoader } = await import(join(process.env.PI_PACKAGE_DIR, "dist/core/resource-loader.js"));
  const original = DefaultResourceLoader.prototype.getExtensions;
  let runtimeLoader: any;
  DefaultResourceLoader.prototype.getExtensions = function () {
    runtimeLoader = this;
    return original.call(this);
  };
  pi.on("session_start", () => {
    if (!runtimeLoader) throw new Error("actual runtime resource loader was not observed");
    for (const [type, result, key] of [
      ["skill", runtimeLoader.getSkills(), "skills"],
      ["prompt", runtimeLoader.getPrompts(), "prompts"],
      ["theme", runtimeLoader.getThemes(), "themes"],
    ] as const) {
      for (const resource of result[key]) {
        report(`inventory:${type}:${label(resource.name)}:${group(resource.filePath ?? resource.sourcePath ?? "")}`);
      }
      for (const diagnostic of result.diagnostics) {
        const collision = diagnostic.collision;
        if (collision) report(`collision:${type}:${label(collision.name)}:${group(collision.winnerPath)}:${group(collision.loserPath)}`);
        else report(`diagnostic:${type}:${diagnostic.type}`);
      }
    }
  });
}
