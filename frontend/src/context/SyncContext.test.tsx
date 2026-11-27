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

const mockStatus = vi.fn();

vi.mock("../services/api", () => ({
	api: { syncStatus: () => mockStatus() },
}));

function push(message: LiveMessage) {
	listeners.forEach((listener) => listener(message));
}

describe("SyncProvider", () => {
	let queryClient: QueryClient;
	const wrapper = ({ children }: { children: ReactNode }) => (
		<QueryClientProvider client={queryClient}>
			<SyncProvider>{children}</SyncProvider>
		</QueryClientProvider>
	);

	beforeEach(() => {
		vi.useFakeTimers();
		listeners.clear();
		queryClient = new QueryClient();
		mockStatus.mockReset();
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

	describe("without WebSocket events", () => {
		it("shows the progress the status endpoint reports", async () => {
			mockStatus.mockResolvedValue({ running: true, total: 10, synced: 5, current: "acme/e" });
			const { result } = renderHook(() => useSync(), { wrapper });

			act(() => result.current.startSync());
			act(() => result.current.syncAccepted());
			await act(async () => {
				await vi.advanceTimersByTimeAsync(2000);
			});

			expect(result.current.sync).toMatchObject({
				isSyncing: true,
				syncedCount: 5,
				totalToSync: 10,
				progress: 50,
				currentRepo: "acme/e",
			});
		});

		it("finishes and reloads the data when the backend reports the sync as done", async () => {
			const invalidate = vi.spyOn(queryClient, "invalidateQueries");
			mockStatus.mockResolvedValue({ running: false, total: 10, synced: 10, current: "acme/j" });
			const { result } = renderHook(() => useSync(), { wrapper });

			act(() => result.current.startSync());
			act(() => result.current.syncAccepted());
			await act(async () => {
				await vi.advanceTimersByTimeAsync(2000);
			});

			expect(result.current.sync).toMatchObject({ isSyncing: false, progress: 100 });
			expect(invalidate).toHaveBeenCalledWith({ queryKey: ["repositories"] });

			// Polling stops once the sync is over
			mockStatus.mockClear();
			await act(async () => {
				await vi.advanceTimersByTimeAsync(10000);
			});
			expect(mockStatus).not.toHaveBeenCalled();
		});

		it("ends the overlay without a data reload when the sync failed", async () => {
			const invalidate = vi.spyOn(queryClient, "invalidateQueries");
			mockStatus.mockResolvedValue({ running: false, total: 0, synced: 0, current: "", error: "Failed to fetch repositories from GitHub" });
			const { result } = renderHook(() => useSync(), { wrapper });

			act(() => result.current.startSync());
			act(() => result.current.syncAccepted());
			await act(async () => {
				await vi.advanceTimersByTimeAsync(2000);
			});

			expect(result.current.sync.isSyncing).toBe(false);
			expect(invalidate).not.toHaveBeenCalled();
		});

		it("does not ask the backend before the request was accepted", async () => {
			const { result } = renderHook(() => useSync(), { wrapper });

			act(() => result.current.startSync());
			await act(async () => {
				await vi.advanceTimersByTimeAsync(10000);
			});

			expect(mockStatus).not.toHaveBeenCalled();
			expect(result.current.sync.isSyncing).toBe(true);
		});

		it("keeps polling through a failed status request", async () => {
			mockStatus
				.mockRejectedValueOnce(new Error("network"))
				.mockResolvedValue({ running: true, total: 4, synced: 1, current: "acme/a" });
			const { result } = renderHook(() => useSync(), { wrapper });

			act(() => result.current.startSync());
			act(() => result.current.syncAccepted());
			await act(async () => {
				await vi.advanceTimersByTimeAsync(4000);
			});

			expect(result.current.sync).toMatchObject({ isSyncing: true, syncedCount: 1, progress: 25 });
		});
	});
});
