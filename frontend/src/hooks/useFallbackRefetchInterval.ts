import { useSocket } from "../context/SocketContext";

/**
 * Returns a React Query `refetchInterval` that polls the backend only while the WebSocket is down.
 * With a live connection the backend pushes every change, so polling would only repeat the same
 * answer. Polling the API also keeps the server-side poller working for this user.
 */
export function useFallbackRefetchInterval(ms: number): number | false {
	const { isConnected } = useSocket();
	return isConnected ? false : ms;
}
