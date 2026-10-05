import { describe, it, expect, beforeEach } from "vitest";
import { QueryClient } from "@tanstack/react-query";
import { applyLiveUpdate } from "./liveUpdates";

/**
 * Seeds the cache with the query keys the pages use, then reports which of them a live event
 * marked stale. Inactive queries are not refetched, so invalidation is observable via state.
 */
function seedQueries(queryClient: QueryClient): Record<string, readonly unknown[]> {
	const keys: Record<string, readonly unknown[]> = {
		runsList: ["runs", { status: "", conclusion: "", branch: "" }],
		runDetail: ["runs", "12"],
		runJobs: ["runs", "12", "jobs"],
		otherRunJobs: ["runs", "99", "jobs"],
		runDefinition: ["runs", "12", "workflow-definition"],
		runAnnotations: ["runs", "12", "annotations"],
		pipelines: ["pipelines", "active"],
		summary: ["dashboard", "summary"],
		trends: ["dashboard", "trends"],
		workflows: ["workflows"],
		workflowRuns: ["workflows", "3", "runs"],
		repositories: ["repositories", 1, "", 20],
		tokens: ["api-tokens"],
	};
	for (const key of Object.values(keys)) {
		queryClient.setQueryData(key, { seeded: true });
	}
	return keys;
}

function invalidated(queryClient: QueryClient, keys: Record<string, readonly unknown[]>): string[] {
	return Object.entries(keys)
		.filter(([, key]) => queryClient.getQueryState(key)?.isInvalidated)
		.map(([name]) => name)
		.sort();
}

describe("applyLiveUpdate", () => {
	let queryClient: QueryClient;
	let keys: Record<string, readonly unknown[]>;

	beforeEach(() => {
		queryClient = new QueryClient();
		keys = seedQueries(queryClient);
	});

	it("workflow_run refreshes every view that shows a run status, not the GitHub-backed extras", () => {
		applyLiveUpdate(queryClient, { type: "workflow_run", data: { id: 12, status: "completed" } });

		expect(invalidated(queryClient, keys)).toEqual(
			[
				"runsList",
				"runDetail",
				"runJobs",
				"otherRunJobs",
				"pipelines",
				"summary",
				"trends",
				"workflows",
				"workflowRuns",
			].sort(),
		);
	});

	it("workflow_job refreshes only the jobs of the run that changed", () => {
		applyLiveUpdate(queryClient, { type: "workflow_job", data: { run_id: 12, run_github_id: 500 } });

		expect(invalidated(queryClient, keys)).toEqual(["runJobs"]);
	});

	it("workflow_job without a run id refreshes every open jobs list", () => {
		applyLiveUpdate(queryClient, { type: "workflow_job", data: {} });

		expect(invalidated(queryClient, keys)).toEqual(["otherRunJobs", "runJobs"]);
	});

	it("deployment refreshes the dashboard only", () => {
		applyLiveUpdate(queryClient, { type: "deployment", data: {} });

		expect(invalidated(queryClient, keys)).toEqual(["summary", "trends"]);
	});

	it("sync events are left to SyncContext", () => {
		applyLiveUpdate(queryClient, { type: "sync:progress", data: { synced: 1, total: 2 } });

		expect(invalidated(queryClient, keys)).toEqual([]);
	});
});
