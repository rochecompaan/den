import { fauxAssistantMessage, fauxProvider } from "@earendil-works/pi-ai";
import { appendFileSync } from "node:fs";
import { join } from "node:path";

export default function providerExtension(pi: { registerProvider(provider: unknown): void }) {
  const root = process.env.PI_CODING_AGENT_DIR;
  if (!root) throw new Error("PI_CODING_AGENT_DIR is required");
  const provider = fauxProvider({
    provider: "den-native",
    models: [{
      id: "fixture",
      name: "Den native fixture",
      reasoning: false,
      input: ["text"],
      cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
      contextWindow: 1024,
      maxTokens: 64,
    }],
  });
  provider.setResponses([fauxAssistantMessage("native provider response")]);
  pi.registerProvider(provider.provider);
  appendFileSync(join(root, "pi-resources.report"), "extension:provider-extension\n");
}
