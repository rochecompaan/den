import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync, readFileSync, renameSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import test, { after } from "node:test";
import { pathToFileURL } from "node:url";

const packageRoot = process.env.PI_PACKAGE_ROOT;
assert.ok(packageRoot, "PI_PACKAGE_ROOT is required");
const root = mkdtempSync(join(tmpdir(), "pi-session-persistence-"));
const sessionDir = join(root, "sessions");
mkdirSync(sessionDir);
process.env.PI_CODING_AGENT_SESSION_DIR = sessionDir;
after(() => rmSync(root, { recursive: true, force: true }));

const { SessionManager } = await import(
	pathToFileURL(resolve(packageRoot, "dist/core/session-manager.js")).href
);

function startSession() {
	const manager = SessionManager.create(root, sessionDir);
	manager.appendMessage({ role: "user", content: "trusted prompt", timestamp: 1 });
	return manager;
}

test("persists the first user message and later entries in the same session", () => {
	const manager = startSession();
	const file = manager.getSessionFile();
	assert.match(readFileSync(file, "utf8"), /trusted prompt/);
	manager.appendThinkingLevelChange("high");
	assert.match(readFileSync(file, "utf8"), /"thinkingLevel":"high"/);
});

for (const replacement of ["regular file", "symlink"]) {
	test(`rejects a ${replacement} replacement after the first user message`, () => {
		const manager = startSession();
		const file = manager.getSessionFile();
		const outside = join(root, `${replacement}.jsonl`);
		const untrusted = "untrusted replacement\n";
		renameSync(file, `${file}.validated`);
		if (replacement === "symlink") {
			writeFileSync(outside, untrusted);
			symlinkSync(outside, file);
		} else {
			writeFileSync(file, untrusted);
		}

		assert.throws(
			() => manager.appendThinkingLevelChange("high"),
			/Pi session target (changed after validation|must be an existing regular file)/,
		);
		assert.equal(readFileSync(file, "utf8"), untrusted);
	});
}
