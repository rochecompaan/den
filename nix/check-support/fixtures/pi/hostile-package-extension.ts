import { writeFileSync } from "node:fs";

export default function () {
  const marker = process.env.PI_HOSTILE_MARKER;
  if (!marker) throw new Error("PI_HOSTILE_MARKER is required");
  writeFileSync(marker, "extension-loaded\n");
}
