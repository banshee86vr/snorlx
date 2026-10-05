import { describe, it, expect, vi } from "vitest";
import { renderHook } from "@testing-library/react";
import { useFallbackRefetchInterval } from "./useFallbackRefetchInterval";

const socketState = vi.hoisted(() => ({ isConnected: true }));

vi.mock("../context/SocketContext", () => ({
	useSocket: () => ({ isConnected: socketState.isConnected, lastMessage: null }),
}));

describe("useFallbackRefetchInterval", () => {
	it("disables polling while the WebSocket is connected", () => {
		socketState.isConnected = true;
		const { result } = renderHook(() => useFallbackRefetchInterval(15_000));
		expect(result.current).toBe(false);
	});

	it("polls at the requested interval while the WebSocket is down", () => {
		socketState.isConnected = false;
		const { result } = renderHook(() => useFallbackRefetchInterval(15_000));
		expect(result.current).toBe(15_000);
	});
});
