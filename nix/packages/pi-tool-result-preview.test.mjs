import assert from "node:assert/strict";
import test from "node:test";
import { resolve } from "node:path";
import { pathToFileURL } from "node:url";

const packageRoot = process.env.PI_PACKAGE_ROOT;
assert.ok(packageRoot, "PI_PACKAGE_ROOT is required");
const fromPackage = (path) => pathToFileURL(resolve(packageRoot, path)).href;

const { ToolExecutionComponent } = await import(
	fromPackage("dist/modes/interactive/components/tool-execution.js")
);
const { initTheme } = await import(fromPackage("dist/modes/interactive/theme/theme.js"));
const { stripAnsi } = await import(fromPackage("dist/utils/ansi.js"));

initTheme("dark");

const fakeTui = { requestRender() {} };
const fallbackDefinition = {};

function createComponent(toolDefinition = fallbackDefinition) {
	return new ToolExecutionComponent(
		"custom_tool",
		"tool-preview",
		{},
		{ showImages: false },
		toolDefinition,
		fakeTui,
		process.cwd(),
	);
}

function createComponentWithoutDefinition() {
	return new ToolExecutionComponent(
		"missing_tool",
		"tool-missing",
		{},
		{ showImages: false },
		undefined,
		fakeTui,
		process.cwd(),
	);
}

function renderText(component, width = 20_000) {
	return stripAnsi(component.render(width).join("\n"));
}

test("bounds fallback text before sanitizing a collapsed result", () => {
	const sliceCalls = [];
	const guardedText = {
		length: 20_000,
		slice(start, end) {
			sliceCalls.push([start, end]);
			return "bounded preview";
		},
		replace() {
			throw new Error("sanitized unbounded text");
		},
	};
	const component = createComponent();

	assert.doesNotThrow(() =>
		component.updateResult(
			{ content: [{ type: "text", text: guardedText }], details: {}, isError: false },
			false,
		),
	);
	assert.deepEqual(sliceCalls, [[0, 10_000]]);
	assert.equal(sliceCalls.length, 1);
	assert.match(renderText(component), /bounded preview/);
});

test("bounds a collapsed fallback preview for one long line", () => {
	const component = createComponent();
	component.updateResult(
		{ content: [{ type: "text", text: `${"x".repeat(12_000)}TAIL_MARKER` }], details: {}, isError: false },
		false,
	);

	const collapsed = renderText(component);
	assert.doesNotMatch(collapsed, /TAIL_MARKER/);
	assert.match(collapsed, /output truncated/);
	assert.doesNotMatch(collapsed, /x{501}/);

	component.setExpanded(true);
	assert.match(renderText(component), /TAIL_MARKER/);
});

test("bounds a collapsed fallback preview by line count", () => {
	const component = createComponent();
	const output = Array.from({ length: 12 }, (_, index) => `line-${index + 1}`).join("\n");
	component.updateResult({ content: [{ type: "text", text: output }], details: {}, isError: false }, false);

	const collapsed = renderText(component);
	assert.match(collapsed, /line-10/);
	assert.doesNotMatch(collapsed, /line-11/);
	assert.match(collapsed, /2 more lines/);

	component.setExpanded(true);
	assert.match(renderText(component), /line-12/);
});

test("bounds a fallback preview without a tool definition", () => {
	const component = createComponentWithoutDefinition();
	component.updateResult(
		{ content: [{ type: "text", text: `${"x".repeat(12_000)}TAIL_MARKER` }], details: {}, isError: false },
		false,
	);

	const collapsed = renderText(component);
	assert.doesNotMatch(collapsed, /TAIL_MARKER/);
	assert.match(collapsed, /output truncated/);
	assert.doesNotMatch(collapsed, /x{501}/);

	component.setExpanded(true);
	assert.match(renderText(component), /TAIL_MARKER/);
});

test("preserves image blocks after the fallback text limit", () => {
	const component = createComponent();
	component.updateResult(
		{
			content: [
				{ type: "text", text: "x".repeat(10_001) },
				{
					type: "image",
					data: "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVQIHWP4z8DwHwAFgAI/ScL9fAAAAABJRU5ErkJggg==",
					mimeType: "image/png",
				},
			],
			details: {},
			isError: false,
		},
		false,
	);

	assert.match(renderText(component), /image/i);
});

test("keeps short fallback results unchanged", () => {
	const component = createComponent();
	component.updateResult(
		{ content: [{ type: "text", text: "first line\nsecond line" }], details: {}, isError: false },
		false,
	);

	const collapsed = renderText(component);
	assert.match(collapsed, /first line/);
	assert.match(collapsed, /second line/);
	assert.doesNotMatch(collapsed, /to expand/);
});

test("shows an expansion hint when an ANSI-only preview omits text", () => {
	const component = createComponent();
	component.updateResult(
		{
			content: [{ type: "text", text: `${"\u001b[31m".repeat(2_000)}TAIL_MARKER` }],
			details: {},
			isError: false,
		},
		false,
	);

	const collapsed = renderText(component);
	assert.doesNotMatch(collapsed, /TAIL_MARKER/);
	assert.match(collapsed, /output truncated/);

	component.setExpanded(true);
	assert.match(renderText(component), /TAIL_MARKER/);
});
