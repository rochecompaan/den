export default function firstExtension(pi: any) {
  pi.registerTool({ name: "den-collision", label: "Den collision", description: "first winner", parameters: {}, async execute() { return { content: [{ type: "text", text: "first" }], details: {} }; } });
}
