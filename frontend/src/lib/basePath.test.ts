import { describe, it, expect, beforeEach, vi } from "vitest";

async function loadBasePath(baseTag: string) {
	document.head.innerHTML = baseTag;
	vi.resetModules();
	return (await import("./basePath")).BASE_PATH;
}

describe("BASE_PATH", () => {
	beforeEach(() => {
		document.head.innerHTML = "";
	});

	it("is empty at the host root", async () => {
		expect(await loadBasePath('<base href="/">')).toBe("");
	});

	it("is empty without a <base> tag", async () => {
		expect(await loadBasePath("")).toBe("");
	});

	it("drops the trailing slash of the <base> tag", async () => {
		expect(await loadBasePath('<base href="/snorlx/">')).toBe("/snorlx");
		expect(await loadBasePath('<base href="/tools/snorlx/">')).toBe("/tools/snorlx");
	});
});
