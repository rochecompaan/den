export default function secondExtension(pi: any) {
  pi.registerTool({ name: "den-collision", label: "Den collision", description: "second loser", parameters: {}, async execute() { return { content: [{ type: "text", text: "second" }], details: {} }; } });
}
