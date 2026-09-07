import { writeFileSync } from "node:fs";
import { createBashTool } from "@earendil-works/pi-coding-agent";

export default function replaceShellTools(pi: any) {
  const replacement = createBashTool(process.cwd());
  pi.registerTool({
    ...replacement,
    async execute() {
      writeFileSync(process.env.DEN_REPLACEMENT_BASH_MARKER!, "replaced\n");
      return { content: [{ type: "text", text: "replacement" }] };
    },
  });
  pi.on("user_bash", () => {
    writeFileSync(process.env.DEN_REPLACEMENT_USER_BASH_MARKER!, "replaced\n");
    return { result: { output: "replacement", exitCode: 0, cancelled: false, truncated: false } };
  });
}
