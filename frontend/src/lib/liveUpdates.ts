import type { Query, QueryClient } from "@tanstack/react-query";

/** A message pushed by the backend over the WebSocket. */
export interface LiveMessage {
	type: string;
	data: unknown;
}

/**
 * Query sub-keys under ["runs", id, ...] that are answered by GitHub rather than by local
 * storage. A run status change never alters them, so a live event must not refetch them.
 */
const GITHUB_BACKED_RUN_QUERIES = new Set(["workflow-definition", "annotations"]);

function isRunQuery(query: Query): boolean {
	return query.queryKey[0] === "runs";
}

function isStorageBackedRunQuery(query: Query): boolean {
	if (!isRunQuery(query)) return false;
	return !query.queryKey.some(
		(part) => typeof part === "string" && GITHUB_BACKED_RUN_QUERIES.has(part),
	);
}

function isJobsQueryOf(runId: number | null) {
	return (query: Query): boolean => {
		if (!isRunQuery(query) || query.queryKey[2] !== "jobs") return false;
		return runId === null || String(query.queryKey[1]) === String(runId);
	};
}

function runIdOf(data: unknown): number | null {
	if (typeof data !== "object" || data === null) return null;
	const runId = (data as { run_id?: unknown }).run_id;
	return typeof runId === "number" ? runId : null;
}

/**
 * Marks the caches affected by a live event as stale so mounted views refetch from the backend.
 * The backend already stored the change; refetching reads local storage, never GitHub.
 */
export function applyLiveUpdate(queryClient: QueryClient, message: LiveMessage): void {
	switch (message.type) {
		case "workflow_run":
			void queryClient.invalidateQueries({ predicate: isStorageBackedRunQuery });
			void queryClient.invalidateQueries({ queryKey: ["pipelines"] });
			void queryClient.invalidateQueries({ queryKey: ["dashboard"] });
			// Workflow lists and details show the last run of each workflow.
			void queryClient.invalidateQueries({ queryKey: ["workflows"] });
			return;
		case "workflow_job":
			void queryClient.invalidateQueries({ predicate: isJobsQueryOf(runIdOf(message.data)) });
			return;
		case "deployment":
			void queryClient.invalidateQueries({ queryKey: ["dashboard"] });
			return;
		default:
			// sync:* events are handled by SyncContext
			return;
	}
}
