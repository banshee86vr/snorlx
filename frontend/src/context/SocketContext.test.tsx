import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { act, render } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { SocketProvider } from "./SocketContext";

/** Minimal WebSocket double: records sent frames and lets the test fire open/message. */
class FakeWebSocket {
	static OPEN = 1;
	static instances: FakeWebSocket[] = [];
	readyState = 0;
	sent: string[] = [];
	onopen: (() => void) | null = null;
	onclose: (() => void) | null = null;
	onerror: (() => void) | null = null;
	onmessage: ((event: MessageEvent) => void) | null = null;

	constructor(public url: string) {
		FakeWebSocket.instances.push(this);
	}

	send(data: string) {
		this.sent.push(data);
	}

	close() {
		this.readyState = 3;
	}

	open() {
		this.readyState = FakeWebSocket.OPEN;
		this.onopen?.();
	}

	receive(payload: unknown) {
		this.onmessage?.({ data: JSON.stringify(payload) } as MessageEvent);
	}

	presenceFrames(): boolean[] {
		return this.sent
			.map((frame) => JSON.parse(frame) as { type: string; data: { active: boolean } })
			.filter((frame) => frame.type === "presence")
			.map((frame) => frame.data.active);
	}
}

function setVisibility(state: "visible" | "hidden") {
	Object.defineProperty(document, "visibilityState", { configurable: true, get: () => state });
	document.dispatchEvent(new Event("visibilitychange"));
}

describe("SocketProvider presence", () => {
	let queryClient: QueryClient;
	const originalWebSocket = globalThis.WebSocket;

	beforeEach(() => {
		FakeWebSocket.instances = [];
		vi.stubGlobal("WebSocket", FakeWebSocket);
		Object.defineProperty(document, "visibilityState", { configurable: true, get: () => "visible" });
		queryClient = new QueryClient();
		queryClient.setQueryData(["pipelines", "active"], []);
	});

	afterEach(() => {
		vi.stubGlobal("WebSocket", originalWebSocket);
	});

	function renderProvider() {
		return render(
			<QueryClientProvider client={queryClient}>
				<SocketProvider>
					<span>child</span>
				</SocketProvider>
			</QueryClientProvider>,
		);
	}

	it("reports a visible page as soon as the socket opens", () => {
		renderProvider();
		const socket = FakeWebSocket.instances[0];

		act(() => socket.open());

		expect(socket.presenceFrames()).toEqual([true]);
	});

	it("reports hidden and visible, holds live events while hidden and catches up on return", () => {
		renderProvider();
		const socket = FakeWebSocket.instances[0];
		act(() => socket.open());

		act(() => setVisibility("hidden"));
		expect(socket.presenceFrames()).toEqual([true, false]);

		act(() => socket.receive({ type: "workflow_run", data: { id: 1, status: "completed" } }));
		expect(queryClient.getQueryState(["pipelines", "active"])?.isInvalidated).toBe(false);

		act(() => setVisibility("visible"));
		expect(socket.presenceFrames()).toEqual([true, false, true]);
		expect(queryClient.getQueryState(["pipelines", "active"])?.isInvalidated).toBe(true);
	});

	it("applies live events immediately while the page is visible", () => {
		renderProvider();
		const socket = FakeWebSocket.instances[0];
		act(() => socket.open());

		act(() => socket.receive({ type: "workflow_run", data: { id: 1, status: "completed" } }));

		expect(queryClient.getQueryState(["pipelines", "active"])?.isInvalidated).toBe(true);
	});

	it("returning to a page that missed nothing does not refetch", () => {
		renderProvider();
		const socket = FakeWebSocket.instances[0];
		act(() => socket.open());

		act(() => setVisibility("hidden"));
		act(() => setVisibility("visible"));

		expect(queryClient.getQueryState(["pipelines", "active"])?.isInvalidated).toBe(false);
	});
});
