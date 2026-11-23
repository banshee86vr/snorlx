import {
	createContext,
	useContext,
	useCallback,
	useEffect,
	useRef,
	useState,
	type ReactNode,
} from "react";
import { useQueryClient } from "@tanstack/react-query";
import { applyLiveUpdate, type LiveMessage } from "../lib/liveUpdates";
import { BASE_PATH } from "../lib/basePath";

interface SocketContextValue {
	isConnected: boolean;
	lastMessage: LiveMessage | null;
	/**
	 * Calls listener for every message, in order. Unlike lastMessage, which keeps only the newest
	 * message of a render, nothing is skipped. Returns the unsubscribe function.
	 */
	subscribe: (listener: (message: LiveMessage) => void) => () => void;
}

const SocketContext = createContext<SocketContextValue>({
	isConnected: false,
	lastMessage: null,
	subscribe: () => () => {},
});

/** A page counts as active while it is visible; a background tab or minimized window is not. */
function isPageVisible(): boolean {
	return document.visibilityState !== "hidden";
}

export function SocketProvider({ children }: { children: ReactNode }) {
	const [isConnected, setIsConnected] = useState(false);
	const [lastMessage, setLastMessage] = useState<LiveMessage | null>(null);
	const queryClient = useQueryClient();
	const listeners = useRef(new Set<(message: LiveMessage) => void>());

	const subscribe = useCallback((listener: (message: LiveMessage) => void) => {
		listeners.current.add(listener);
		return () => {
			listeners.current.delete(listener);
		};
	}, []);

	useEffect(() => {
		const wsProtocol = window.location.protocol === "https:" ? "wss:" : "ws:";
		const wsUrl = `${wsProtocol}//${window.location.host}${BASE_PATH}/ws`;

		let ws: WebSocket | null = null;
		let reconnectTimeout: ReturnType<typeof setTimeout> | null = null;
		let reconnectAttempts = 0;
		let hadConnection = false;
		// Set while the page is hidden and a live event or a reconnect was skipped; the page
		// refetches everything once when it is visible again.
		let missedWhileHidden = false;
		const maxReconnectAttempts = 10;
		const baseReconnectDelay = 1000;

		// The backend polls GitHub only for users whose page is visible, so every visibility
		// change is reported. Hidden pages also stop refetching: nobody is looking.
		const sendPresence = () => {
			if (ws?.readyState === WebSocket.OPEN) {
				ws.send(JSON.stringify({ type: "presence", data: { active: isPageVisible() } }));
			}
		};

		const refetchEverything = () => {
			if (!isPageVisible()) {
				missedWhileHidden = true;
				return;
			}
			missedWhileHidden = false;
			void queryClient.invalidateQueries();
		};

		const handleVisibilityChange = () => {
			sendPresence();
			if (isPageVisible() && missedWhileHidden) {
				refetchEverything();
			}
		};

		const handleMessage = (event: MessageEvent) => {
			try {
				const message = JSON.parse(event.data) as LiveMessage;
				setLastMessage(message);
				listeners.current.forEach((listener) => listener(message));
				if (!isPageVisible()) {
					missedWhileHidden = true;
					return;
				}
				applyLiveUpdate(queryClient, message);
			} catch {
				// Silent error handling for malformed messages
			}
		};

		const connect = () => {
			ws = new WebSocket(wsUrl);

			ws.onopen = () => {
				setIsConnected(true);
				reconnectAttempts = 0;
				sendPresence();
				if (hadConnection) {
					// Events pushed while the socket was down are lost: refetch everything once.
					refetchEverything();
				}
				hadConnection = true;
			};

			ws.onclose = () => {
				setIsConnected(false);

				// Attempt to reconnect with exponential backoff
				if (reconnectAttempts < maxReconnectAttempts) {
					const delay = baseReconnectDelay * Math.pow(2, reconnectAttempts);
					reconnectTimeout = setTimeout(() => {
						reconnectAttempts++;
						connect();
					}, delay);
				}
			};

			ws.onerror = () => {
				// Silent error handling - reconnection will be attempted
			};

			ws.onmessage = handleMessage;
		};

		document.addEventListener("visibilitychange", handleVisibilityChange);
		connect();

		return () => {
			document.removeEventListener("visibilitychange", handleVisibilityChange);
			if (reconnectTimeout) {
				clearTimeout(reconnectTimeout);
			}
			if (ws) {
				ws.close();
			}
		};
	}, [queryClient]);

	return (
		<SocketContext.Provider value={{ isConnected, lastMessage, subscribe }}>
			{children}
		</SocketContext.Provider>
	);
}

export function useSocket() {
	return useContext(SocketContext);
}
