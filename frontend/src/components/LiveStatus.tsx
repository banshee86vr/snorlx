import { useSocket } from "../context/SocketContext";
import { cn } from "../lib/utils";

/**
 * Tells the user how the view stays current: live over the WebSocket, or polling while the
 * connection is down. Replaces manual refresh controls as the primary way to get fresh data.
 */
export function LiveStatus({ className }: { className?: string }) {
	const { isConnected } = useSocket();

	return (
		<span
			className={cn(
				"inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-xs font-medium",
				isConnected
					? "bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300"
					: "bg-amber-50 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300",
				className,
			)}
			title={
				isConnected
					? "Updates arrive automatically as soon as GitHub reports a change"
					: "Live connection lost: this view polls the server until it reconnects"
			}
			role="status"
		>
			<span className="relative flex h-2 w-2">
				{isConnected && (
					<span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-75" />
				)}
				<span
					className={cn(
						"relative inline-flex h-2 w-2 rounded-full",
						isConnected ? "bg-emerald-500" : "bg-amber-500",
					)}
				/>
			</span>
			{isConnected ? "Live" : "Polling"}
		</span>
	);
}
