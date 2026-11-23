import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { SyncProvider, useSync } from "./SyncContext";
import type { LiveMessage } from "../lib/liveUpdates";

const listeners = new Set<(message: LiveMessage) => void>();

vi.mock("./SocketContext", () => ({
	useSocket: () => ({
		isConnected: true,
		lastMessage: null,
		subscribe: (listener: (message: LiveMessage) => void) => {
			listeners.add(listener);
			return () => listeners.delete(listener);
		},
	}),
}));

function push(message: LiveMessage) {
	listeners.forEach((listener) => listener(message));
}

describe("SyncProvider", () => {
	const wrapper = ({ children }: { children: ReactNode }) => (
		<QueryClientProvider client={new QueryClient()}>
			<SyncProvider>{children}</SyncProvider>
		</QueryClientProvider>
	);

	beforeEach(() => {
		vi.useFakeTimers();
		listeners.clear();
	});

	afterEach(() => {
		vi.useRealTimers();
	});

	it("applies every event of a burst, in order", () => {
		const { result } = renderHook(() => useSync(), { wrapper });

		act(() => {
			// Delivered within one tick, like the backend's sync:start and first sync:progress
			push({ type: "sync:start", data: { total: 4 } });
			push({ type: "sync:progress", data: { synced: 1, total: 4, current: "acme/a" } });
			push({ type: "sync:progress", data: { synced: 2, total: 4, current: "acme/b" } });
		});

		expect(result.current.sync).toMatchObject({
			isSyncing: true,
			syncedCount: 2,
			totalToSync: 4,
			progress: 50,
			currentRepo: "acme/b",
		});
	});

	it("finishes on sync:complete and resets after 2 seconds", () => {
		const { result } = renderHook(() => useSync(), { wrapper });

		act(() => {
			push({ type: "sync:start", data: { total: 2 } });
			push({ type: "sync:complete", data: {} });
		});
		expect(result.current.sync).toMatchObject({ isSyncing: false, progress: 100 });

		act(() => {
			vi.advanceTimersByTime(2000);
		});
		expect(result.current.sync).toMatchObject({ isSyncing: false, progress: 0, totalToSync: 0 });
	});

	it("does not reset a sync that starts before the previous reset timer fires", () => {
		const { result } = renderHook(() => useSync(), { wrapper });

		act(() => {
			push({ type: "sync:complete", data: {} });
		});
		act(() => {
			vi.advanceTimersByTime(1000);
			push({ type: "sync:start", data: { total: 3 } });
		});
		act(() => {
			vi.advanceTimersByTime(1500);
		});

		expect(result.current.sync).toMatchObject({ isSyncing: true, totalToSync: 3 });
	});

	it("ends the overlay when the sync request is rejected", () => {
		const { result } = renderHook(() => useSync(), { wrapper });

		act(() => result.current.startSync());
		expect(result.current.sync.isSyncing).toBe(true);

		act(() => result.current.failSync());
		expect(result.current.sync.isSyncing).toBe(false);
	});
});
