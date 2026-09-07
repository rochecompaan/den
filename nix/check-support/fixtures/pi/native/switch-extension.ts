import { appendFileSync } from "node:fs";
import { join } from "node:path";

export default function switchExtension() {
  const root = process.env.PI_CODING_AGENT_DIR;
  if (!root) throw new Error("PI_CODING_AGENT_DIR is required");
  appendFileSync(join(root, "pi-resources.report"), [
    "extension:switch-extension",
    "package:fixture-package",
    "skill:fixture-skill",
    "prompt:fixture-prompt",
    "theme:fixture-theme",
  ].join("\n") + "\n");
}
