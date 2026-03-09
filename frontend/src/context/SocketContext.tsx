import {
	createContext,
	useContext,
	useEffect,
	useState,
	useCallback,
	type ReactNode,
} from "react";
import { useQueryClient } from "@tanstack/react-query";

interface SocketContextValue {
	isConnected: boolean;
	lastMessage: WebSocketMessage | null;
}

interface WebSocketMessage {
	type: string;
	data: unknown;
}

const SocketContext = createContext<SocketContextValue>({
	isConnected: false,
	lastMessage: null,
});

export function SocketProvider({ children }: { children: ReactNode }) {
	const [isConnected, setIsConnected] = useState(false);
	const [lastMessage, setLastMessage] = useState<WebSocketMessage | null>(null);
	const queryClient = useQueryClient();

	const handleMessage = useCallback(
		(event: MessageEvent) => {
			try {
				const message = JSON.parse(event.data) as WebSocketMessage;
				setLastMessage(message);

				// Invalidate relevant queries based on message type
				switch (message.type) {
					case "workflow_run":
						queryClient.invalidateQueries({ queryKey: ["runs"] });
						queryClient.invalidateQueries({ queryKey: ["dashboard"] });
						queryClient.invalidateQueries({ queryKey: ["pipelines"] });
						break;
					case "workflow_job":
						queryClient.invalidateQueries({ queryKey: ["jobs"] });
						break;
					case "deployment":
						queryClient.invalidateQueries({ queryKey: ["dashboard"] });
						break;
					case "sync:start":
					case "sync:progress":
					case "sync:complete":
					case "sync:error":
						// These are handled by the SyncContext
						break;
				}
			} catch {
				// Silent error handling for malformed messages
			}
		},
		[queryClient],
	);

	useEffect(() => {
		const wsProtocol = window.location.protocol === "https:" ? "wss:" : "ws:";
		const wsUrl = `${wsProtocol}//${window.location.host}/ws`;

		let ws: WebSocket | null = null;
		let reconnectTimeout: ReturnType<typeof setTimeout> | null = null;
		let reconnectAttempts = 0;
		const maxReconnectAttempts = 10;
		const baseReconnectDelay = 1000;

		const connect = () => {
			ws = new WebSocket(wsUrl);

			ws.onopen = () => {
				setIsConnected(true);
				reconnectAttempts = 0;
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

		connect();

		return () => {
			if (reconnectTimeout) {
				clearTimeout(reconnectTimeout);
			}
			if (ws) {
				ws.close();
			}
		};
	}, [handleMessage]);

	return (
		<SocketContext.Provider value={{ isConnected, lastMessage }}>
			{children}
		</SocketContext.Provider>
	);
}

export function useSocket() {
	return useContext(SocketContext);
}
