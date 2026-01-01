export interface User {
	id: number;
	login: string;
	name: string | null;
	email: string | null;
	avatar_url: string | null;
}

export interface Organization {
	id: number;
	github_id: number;
	login: string;
	name: string | null;
	avatar_url: string | null;
}

export interface Repository {
	id: number;
	github_id: number;
	org_id: number | null;
	name: string;
	full_name: string;
	description: string | null;
	default_branch: string;
	html_url: string;
	is_private: boolean;
	is_active: boolean;
	workflow_count?: number;
	organization?: Organization;
}

export interface Workflow {
	id: number;
	github_id: number;
	repo_id: number;
	name: string;
	path: string;
	state: string;
	badge_url: string | null;
	html_url: string | null;
	repository?: Repository;
	last_run?: WorkflowRun;
	total_runs?: number;
	success_rate?: number;
	avg_duration_seconds?: number;
}

export interface WorkflowRun {
	id: number;
	github_id: number;
	workflow_id: number;
	repo_id: number;
	run_number: number;
	name: string;
	status: string;
	conclusion: string | null;
	event: string;
	branch: string;
	commit_sha: string;
	commit_message: string | null;
	actor_login: string;
	actor_avatar: string | null;
	html_url: string;
	started_at: string;
	completed_at: string | null;
	duration_seconds: number | null;
	is_deployment: boolean;
	environment: string | null;
	workflow?: Workflow;
	repository?: Repository;
	jobs?: WorkflowJob[];
}

export interface WorkflowJob {
	id: number;
	github_id: number;
	run_id: number;
	name: string;
	status: string;
	conclusion: string | null;
	runner_name: string | null;
	runner_group: string | null;
	labels: string[];
	steps: JobStep[];
	started_at: string;
	completed_at: string | null;
	duration_seconds: number | null;
}

export interface JobStep {
	name: string;
	status: string;
	conclusion: string | null;
	number: number;
	started_at: string | null;
	completed_at: string | null;
}

export interface DashboardSummary {
	repositories: {
		total: number;
		active: number;
		inactive: number;
	};
	workflows: {
		total: number;
		active: number;
		disabled: number;
	};
	runs: {
		total: number;
		success: number;
		failed: number;
		in_progress: number;
		queued: number;
		cancelled: number;
		success_rate: number;
		total_duration_seconds: number;
	};
	recent_runs: WorkflowRun[];
	failed_runs: WorkflowRun[];
}

export interface Trend {
	date: string;
	total_runs: number;
	successful_runs: number;
	failed_runs: number;
	avg_duration: number;
	deployment_count: number;
}

export interface Pagination {
	page: number;
	page_size: number;
	total: number;
}

export interface ListResponse<T> {
	data: T[];
	pagination: Pagination;
}

export interface RunFilters {
	status?: string;
	conclusion?: string;
	branch?: string;
	event?: string;
	actor?: string;
	workflow_id?: number;
	repo_id?: number;
}

export interface JobDependency {
	job_id: string;
	name: string;
	needs: string[];
}
