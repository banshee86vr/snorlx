import type {
	User,
	Organization,
	Repository,
	Workflow,
	WorkflowRun,
	WorkflowJob,
	DashboardSummary,
	Trend,
	ListResponse,
	RunFilters,
	JobDependency,
} from "../types";

const API_URL = import.meta.env.VITE_API_URL || "";

async function fetchApi<T>(
	endpoint: string,
	options?: RequestInit,
): Promise<T> {
	const response = await fetch(`${API_URL}${endpoint}`, {
		...options,
		credentials: "include",
		headers: {
			"Content-Type": "application/json",
			...options?.headers,
		},
	});

	if (!response.ok) {
		const error = await response.text();
		throw new Error(error || `HTTP error ${response.status}`);
	}

	return response.json();
}

// Auth API
export const authApi = {
	getStatus: () =>
		fetchApi<{ authenticated: boolean; user?: User }>("/api/auth/status"),
	logout: () => fetchApi<void>("/api/auth/logout", { method: "POST" }),
};

// Organizations API
export const organizationsApi = {
	list: () => fetchApi<Organization[]>("/api/organizations"),
	get: (id: number) => fetchApi<Organization>(`/api/organizations/${id}`),
};

// Repositories API
export const repositoriesApi = {
	list: (page = 1, search?: string, perPage?: number) => {
		const params = new URLSearchParams({ page: String(page) });
		if (search) {
			params.append("search", search);
		}
		if (perPage != null && perPage > 0) {
			params.append("per_page", String(perPage));
		}
		return fetchApi<ListResponse<Repository>>(`/api/repositories?${params}`);
	},
	get: (id: number) => fetchApi<Repository>(`/api/repositories/${id}`),
	sync: () =>
		fetchApi<{ status: string }>("/api/repositories/sync", { method: "POST" }),
};

// Workflows API
export const workflowsApi = {
	list: (repoId?: number) =>
		fetchApi<Workflow[]>(`/api/workflows${repoId ? `?repo_id=${repoId}` : ""}`),
	get: (id: number) => fetchApi<Workflow>(`/api/workflows/${id}`),
	getRuns: (id: number, page = 1) =>
		fetchApi<ListResponse<WorkflowRun>>(
			`/api/workflows/${id}/runs?page=${page}`,
		),
};

// Annotation type for workflow run errors
export interface RunAnnotation {
	path: string;
	start_line: number;
	end_line: number;
	annotation_level: "notice" | "warning" | "failure";
	message: string;
	title?: string;
	raw_details?: string;
}

// Runs API
export const runsApi = {
	list: (filters?: RunFilters, page = 1) => {
		const params = new URLSearchParams({ page: String(page) });
		if (filters) {
			Object.entries(filters).forEach(([key, value]) => {
				if (value !== undefined && value !== "") {
					params.append(key, String(value));
				}
			});
		}
		return fetchApi<ListResponse<WorkflowRun>>(`/api/runs?${params}`);
	},
	get: (id: number, options?: { refresh?: boolean }) => {
		const url = options?.refresh
			? `/api/runs/${id}?refresh=true`
			: `/api/runs/${id}`;
		return fetchApi<WorkflowRun>(url);
	},
	getJobs: (id: number) => fetchApi<WorkflowJob[]>(`/api/runs/${id}/jobs`),
	getLogs: (id: number) =>
		fetchApi<{ url?: string; message?: string }>(`/api/runs/${id}/logs`),
	getAnnotations: (id: number) =>
		fetchApi<RunAnnotation[]>(`/api/runs/${id}/annotations`),
	getWorkflowDefinition: (id: number) =>
		fetchApi<JobDependency[]>(`/api/runs/${id}/workflow-definition`),
	rerun: (id: number) =>
		fetchApi<{ status: string }>(`/api/runs/${id}/rerun`, { method: "POST" }),
	cancel: (id: number) =>
		fetchApi<{ status: string }>(`/api/runs/${id}/cancel`, { method: "POST" }),
};

// Jobs API
export const jobsApi = {
	getLogs: (id: number) => fetchApi<{ url: string }>(`/api/jobs/${id}/logs`),
};

// Dashboard API
export const dashboardApi = {
	getSummary: () => fetchApi<DashboardSummary>("/api/dashboard/summary"),
	getTrends: (days = 30) =>
		fetchApi<{ trends: Trend[] }>(`/api/dashboard/trends?days=${days}`),
};

// Convenience export for common operations
export const api = {
	syncRepositories: () => repositoriesApi.sync(),
};
