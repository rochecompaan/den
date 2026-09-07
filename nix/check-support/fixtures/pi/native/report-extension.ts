import { appendFileSync } from "node:fs";
import { join } from "node:path";

export default function reportExtension() {
  const root = process.env.PI_CODING_AGENT_DIR;
  if (!root) throw new Error("PI_CODING_AGENT_DIR is required");
  appendFileSync(join(root, "pi-resources.report"), "extension:report-extension\n");
}
