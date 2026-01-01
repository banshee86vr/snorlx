import { useState, useCallback, useMemo, useRef, useEffect } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useParams, useNavigate } from "react-router-dom";
import {
	ReactFlow,
	Background,
	Controls,
	Handle,
	Position,
	type Node,
	type Edge,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";

// Glow pulse animation styles
const glowPulseStyles = `
@keyframes glow-pulse {
  0%, 100% {
    box-shadow: 0 0 15px var(--glow-color, rgba(16, 185, 129, 0.5));
  }
  50% {
    box-shadow: 0 0 30px var(--glow-color, rgba(16, 185, 129, 0.8)), 0 0 40px var(--glow-color, rgba(16, 185, 129, 0.4));
  }
}
`;

// Helper to get glow color based on job status/conclusion
function getJobGlowColor(
	status: string,
	conclusion: string | null,
	opacity = 0.6,
): string {
	if (status === "in_progress") return `rgba(59, 130, 246, ${opacity})`;
	if (conclusion === "success") return `rgba(16, 185, 129, ${opacity})`;
	if (conclusion === "failure") return `rgba(239, 68, 68, ${opacity})`;
	if (conclusion === "skipped") return `rgba(156, 163, 175, ${opacity})`;
	return `rgba(245, 158, 11, ${opacity})`;
}
import {
	ArrowLeft,
	ExternalLink,
	CheckCircle,
	XCircle,
	Clock,
	Loader2,
	RefreshCw,
	StopCircle,
	GitBranch,
	GitCommit,
	User,
	FileText,
	Play,
	ChevronDown,
	RotateCw,
	Pause,
	Circle,
	AlertTriangle,
} from "lucide-react";
import { runsApi, jobsApi } from "../services/api";
import {
	cn,
	formatDuration,
	formatDateTime,
	getStatusColor,
} from "../lib/utils";
import type { WorkflowJob } from "../types";

// Custom node component for jobs in the flow
function JobNode({
	data,
}: {
	data: { job: WorkflowJob; selected: boolean; onClick: () => void };
}) {
	const { job, selected, onClick } = data;

	const getStatusStyles = () => {
		if (job.status === "in_progress") {
			return "border-blue-500 bg-blue-500/10 ring-blue-500/30";
		}
		if (
			job.status === "queued" ||
			job.status === "pending" ||
			job.status === "waiting"
		) {
			return "border-amber-500 bg-amber-500/10 ring-amber-500/30 animate-pulse";
		}
		if (job.conclusion === "success") {
			return "border-emerald-500 bg-emerald-500/10 ring-emerald-500/30";
		}
		if (job.conclusion === "failure") {
			return "border-red-500 bg-red-500/10 ring-red-500/30";
		}
		if (job.conclusion === "skipped") {
			return "border-gray-400 bg-gray-400/10 ring-gray-400/30";
		}
		return "border-amber-500 bg-amber-500/10 ring-amber-500/30";
	};

	return (
		<button
			type="button"
			onClick={onClick}
			className={cn(
				"px-4 py-3 rounded-xl border-2 cursor-pointer transition-all duration-200 min-w-[180px] relative",
				"hover:scale-105 hover:shadow-lg",
				getStatusStyles(),
				selected && "ring-2 scale-105",
			)}
			style={
				selected
					? {
							animation: "glow-pulse 2s ease-in-out infinite",
							["--glow-color" as string]: getJobGlowColor(
								job.status,
								job.conclusion,
							),
						}
					: undefined
			}
		>
			<Handle
				type="target"
				position={Position.Left}
				className="!bg-gray-400 !w-2 !h-2"
			/>
			<div className="flex items-center gap-2">
				<JobStatusIcon status={job.status} conclusion={job.conclusion} />
				<div className="flex-1 min-w-0 text-left">
					<p className="font-medium text-gray-900 dark:text-gray-100 text-sm truncate">
						{job.name}
					</p>
					<p className="text-xs text-gray-500 dark:text-gray-400">
						{formatDuration(job.duration_seconds)}
					</p>
				</div>
			</div>
			<Handle
				type="source"
				position={Position.Right}
				className="!bg-gray-400 !w-2 !h-2"
			/>
		</button>
	);
}

const nodeTypes = { jobNode: JobNode };

export function RunDetail() {
	const { id } = useParams<{ id: string }>();
	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const [selectedJob, setSelectedJob] = useState<WorkflowJob | null>(null);
	const [stepsExpanded, setStepsExpanded] = useState(true);
	const [autoRefresh, setAutoRefresh] = useState(false);
	const [refreshInterval, setRefreshInterval] = useState(15); // seconds
	const [intervalDropdownOpen, setIntervalDropdownOpen] = useState(false);
	const intervalDropdownRef = useRef<HTMLDivElement>(null);

	// Close dropdown when clicking outside
	useEffect(() => {
		function handleClickOutside(event: MouseEvent) {
			if (
				intervalDropdownRef.current &&
				!intervalDropdownRef.current.contains(event.target as HTMLElement)
			) {
				setIntervalDropdownOpen(false);
			}
		}
		document.addEventListener("mousedown", handleClickOutside);
		return () => document.removeEventListener("mousedown", handleClickOutside);
	}, []);

	const intervalOptions = [
		{ value: 15, label: "15s" },
		{ value: 30, label: "30s" },
		{ value: 60, label: "1m" },
		{ value: 300, label: "5m" },
	];

	const {
		data: run,
		isLoading,
		refetch: refetchRun,
		isFetching: isRefetching,
	} = useQuery({
		queryKey: ["runs", id],
		queryFn: () => runsApi.get(Number(id)),
		enabled: !!id,
		refetchInterval: (query) => {
			const data = query.state.data;
			// Auto-refresh for in-progress runs or if manual auto-refresh is enabled
			if (data?.status === "in_progress" || data?.status === "queued") {
				return 5000;
			}
			return autoRefresh ? refreshInterval * 1000 : false;
		},
	});

	const {
		data: jobs,
		isLoading: jobsLoading,
		refetch: refetchJobs,
	} = useQuery({
		queryKey: ["runs", id, "jobs"],
		queryFn: () => runsApi.getJobs(Number(id)),
		enabled: !!id,
		refetchInterval: autoRefresh ? refreshInterval * 1000 : false,
	});

	// Fetch workflow definition to get job dependencies (needs)
	const { data: workflowDefinition } = useQuery({
		queryKey: ["runs", id, "workflow-definition"],
		queryFn: () => runsApi.getWorkflowDefinition(Number(id)),
		enabled: !!id,
		staleTime: 1000 * 60 * 60, // Cache for 1 hour since workflow definition doesn't change
	});

	// Fetch annotations when run has failed but has no jobs
	const { data: annotations, isLoading: annotationsLoading } = useQuery({
		queryKey: ["runs", id, "annotations"],
		queryFn: () => runsApi.getAnnotations(Number(id)),
		enabled:
			!!id && run?.conclusion === "failure" && (!jobs || jobs.length === 0),
	});

	// Manual refresh handler
	const handleRefresh = useCallback(() => {
		refetchRun();
		refetchJobs();
	}, [refetchRun, refetchJobs]);

	const rerunMutation = useMutation({
		mutationFn: () => runsApi.rerun(Number(id)),
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: ["runs", id] });
		},
	});

	const cancelMutation = useMutation({
		mutationFn: () => runsApi.cancel(Number(id)),
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: ["runs", id] });
		},
	});

	const handleJobClick = useCallback(
		(job: WorkflowJob) => {
			if (selectedJob?.id === job.id) {
				// Toggle accordion if clicking the same job
				setStepsExpanded(!stepsExpanded);
			} else {
				// Select new job and expand
				setSelectedJob(job);
				setStepsExpanded(true);
			}
		},
		[selectedJob, stepsExpanded],
	);

	// Create nodes and edges for ReactFlow using actual workflow definition dependencies
	const { nodes, edges } = useMemo(() => {
		if (!jobs || jobs.length === 0) {
			return { nodes: [], edges: [] };
		}

		const nodeList: Node[] = [];
		const edgeList: Edge[] = [];

		// Build a map from job name to job (for matching with workflow definition)
		const jobByName = new Map<string, WorkflowJob>();
		for (const job of jobs) {
			jobByName.set(job.name, job);
		}

		// Build dependency map using workflow definition if available
		// Key: job name, Value: array of job names this job depends on
		const dependencyMap = new Map<string, string[]>();
		const jobIdByName = new Map<string, string>(); // job_id from YAML -> job name

		if (workflowDefinition && workflowDefinition.length > 0) {
			// Build mapping from YAML job_id to actual job name
			for (const dep of workflowDefinition) {
				// Try to find matching job by comparing names
				// The YAML job_id might be different from the display name
				const matchingJob = jobs.find(
					(j) =>
						j.name === dep.name ||
						j.name === dep.job_id ||
						j.name.toLowerCase() === dep.name.toLowerCase() ||
						j.name.toLowerCase() === dep.job_id.toLowerCase(),
				);
				if (matchingJob) {
					jobIdByName.set(dep.job_id, matchingJob.name);
					dependencyMap.set(matchingJob.name, dep.needs);
				}
			}
		}

		// Calculate depth for each job based on dependencies
		const jobDepth = new Map<string, number>();

		function calculateDepth(jobName: string, visited: Set<string>): number {
			if (visited.has(jobName)) return 0; // Prevent cycles
			const cached = jobDepth.get(jobName);
			if (cached !== undefined) return cached;

			visited.add(jobName);
			const needs = dependencyMap.get(jobName) || [];

			if (needs.length === 0) {
				jobDepth.set(jobName, 0);
				return 0;
			}

			let maxParentDepth = -1;
			for (const needId of needs) {
				// Map the need job_id to actual job name
				const needName = jobIdByName.get(needId) || needId;
				const parentDepth = calculateDepth(needName, new Set(visited));
				maxParentDepth = Math.max(maxParentDepth, parentDepth);
			}

			const depth = maxParentDepth + 1;
			jobDepth.set(jobName, depth);
			return depth;
		}

		// Calculate depths for all jobs
		for (const job of jobs) {
			calculateDepth(job.name, new Set());
		}

		// If no workflow definition, fallback to putting all jobs at depth 0
		if (!workflowDefinition || workflowDefinition.length === 0) {
			for (const job of jobs) {
				if (!jobDepth.has(job.name)) {
					jobDepth.set(job.name, 0);
				}
			}
		}

		// Build a set of jobs that have children (are depended upon by other jobs)
		const jobsWithChildren = new Set<string>();
		for (const [, needs] of dependencyMap) {
			for (const needId of needs) {
				const needName = jobIdByName.get(needId) || needId;
				jobsWithChildren.add(needName);
			}
		}

		// Identify orphan jobs (no dependencies AND no children)
		// These will be placed in the leftmost column (depth 0), below other jobs
		const orphanJobs: WorkflowJob[] = [];
		const connectedJobs: WorkflowJob[] = [];

		for (const job of jobs) {
			const needs = dependencyMap.get(job.name) || [];
			const hasChildren = jobsWithChildren.has(job.name);
			const hasDependencies = needs.length > 0;

			if (!hasChildren && !hasDependencies) {
				orphanJobs.push(job);
			} else {
				connectedJobs.push(job);
			}
		}

		// Group connected jobs by depth
		const groups = new Map<number, WorkflowJob[]>();
		for (const job of connectedJobs) {
			const depth = jobDepth.get(job.name) ?? 0;
			if (!groups.has(depth)) groups.set(depth, []);
			groups.get(depth)?.push(job);
		}

		// Sort connected jobs within each group: jobs with children first, then alphabetically
		for (const [, g] of groups.entries()) {
			g.sort((a, b) => {
				const aHasChildren = jobsWithChildren.has(a.name);
				const bHasChildren = jobsWithChildren.has(b.name);
				if (aHasChildren && !bHasChildren) return -1;
				if (!aHasChildren && bHasChildren) return 1;
				return a.name.localeCompare(b.name);
			});
		}

		// Sort orphan jobs alphabetically
		orphanJobs.sort((a, b) => a.name.localeCompare(b.name));

		// Layout nodes
		const depths = Array.from(groups.keys()).sort((a, b) => a - b);
		const spacing = { x: 300, y: 90 };
		const startX = 100;
		const centerY = 180;

		// First pass: layout connected jobs and track the maximum Y position
		let maxY = 0;

		depths.forEach((depth, col) => {
			const group = groups.get(depth) || [];
			const totalHeight = group.length * spacing.y;
			const startY = centerY - totalHeight / 2 + spacing.y / 2;

			group.forEach((job, row) => {
				const y = startY + row * spacing.y;
				maxY = Math.max(maxY, y);

				nodeList.push({
					id: `job-${job.id}`,
					type: "jobNode",
					position: {
						x: startX + col * spacing.x,
						y,
					},
					data: {
						job,
						selected: selectedJob?.id === job.id,
						onClick: () => handleJobClick(job),
					},
					draggable: false,
					selectable: false,
					connectable: false,
				});
			});
		});

		// Second pass: layout orphan jobs in the leftmost column, below all other jobs
		if (orphanJobs.length > 0) {
			const orphanStartY = maxY + spacing.y; // Start below the lowest connected job

			orphanJobs.forEach((job, idx) => {
				nodeList.push({
					id: `job-${job.id}`,
					type: "jobNode",
					position: {
						x: startX, // Leftmost column
						y: orphanStartY + idx * spacing.y,
					},
					data: {
						job,
						selected: selectedJob?.id === job.id,
						onClick: () => handleJobClick(job),
					},
					draggable: false,
					selectable: false,
					connectable: false,
				});
			});
		}

		// Draw edges based on actual dependencies from workflow definition
		for (const job of jobs) {
			const needs = dependencyMap.get(job.name) || [];
			for (const needId of needs) {
				// Map the need job_id to actual job name
				const needName = jobIdByName.get(needId) || needId;
				const parentJob = jobByName.get(needName);

				if (parentJob) {
					edgeList.push({
						id: `edge-${parentJob.id}-${job.id}`,
						source: `job-${parentJob.id}`,
						target: `job-${job.id}`,
						animated: job.status === "in_progress",
						style: { stroke: "#6b7280", strokeWidth: 2 },
						type: "smoothstep",
					});
				}
			}
		}

		return { nodes: nodeList, edges: edgeList };
	}, [jobs, workflowDefinition, selectedJob, handleJobClick]);

	const handleViewLogs = useCallback(async () => {
		if (!selectedJob) return;
		try {
			const data = await jobsApi.getLogs(selectedJob.id);
			if (data.url) {
				window.open(data.url, "_blank", "noopener,noreferrer");
			}
		} catch (error) {
			console.error("Failed to fetch logs:", error);
		}
	}, [selectedJob]);

	if (isLoading) {
		return (
			<div className="flex items-center justify-center min-h-[400px]">
				<Loader2 className="w-8 h-8 animate-spin text-primary-500" />
			</div>
		);
	}

	if (!run) {
		return (
			<div className="text-center py-12">
				<p className="text-gray-500 dark:text-gray-400">Run not found</p>
			</div>
		);
	}

	const isRunning = run.status === "in_progress" || run.status === "queued";

	return (
		<div
			className="-m-6 flex flex-col overflow-hidden"
			style={{ height: "calc(100vh - 4rem - 1px)" }}
		>
			{/* Inject glow animation styles */}
			<style>{glowPulseStyles}</style>
			{/* Header */}
			<div className="flex-shrink-0 px-4 py-3 border-b border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900">
				<div className="flex items-start justify-between">
					<div className="flex items-center gap-4">
						<button
							type="button"
							onClick={() => navigate(-1)}
							className="p-2 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-800 transition-colors"
						>
							<ArrowLeft className="w-5 h-5 text-gray-500" />
						</button>
						<div>
							<div className="flex items-center gap-3">
								<RunStatusIcon
									status={run.status}
									conclusion={run.conclusion}
									size="lg"
								/>
								<h1 className="text-xl font-bold text-gray-900 dark:text-gray-100">
									{run.name}
								</h1>
							</div>
							<p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
								{run.repository?.full_name} • Run #{run.run_number}
							</p>
						</div>
					</div>
					<div className="flex items-center gap-2">
						{/* Refresh button */}
						<button
							type="button"
							onClick={handleRefresh}
							disabled={isRefetching}
							className="btn-secondary flex items-center gap-2"
							title="Refresh run data"
						>
							<RotateCw
								className={cn("w-4 h-4", isRefetching && "animate-spin")}
							/>
							Refresh
						</button>

						{/* Auto-refresh split button */}
						<div ref={intervalDropdownRef} className="relative flex">
							{/* Play/Pause button */}
							<button
								type="button"
								onClick={() => setAutoRefresh(!autoRefresh)}
								className={cn(
									"px-3 py-2 flex items-center justify-center rounded-l-lg border border-r-0 transition-all",
									autoRefresh
										? "bg-primary-500 border-primary-500 text-white hover:bg-primary-600"
										: "bg-white dark:bg-gray-800 border-gray-300 dark:border-gray-600 text-gray-700 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-700",
								)}
								title={
									autoRefresh ? "Disable auto-refresh" : "Enable auto-refresh"
								}
							>
								{autoRefresh ? (
									<Pause className="w-4 h-4 animate-pulse" />
								) : (
									<Play className="w-4 h-4" />
								)}
							</button>
							{/* Interval dropdown trigger */}
							<button
								type="button"
								onClick={() => setIntervalDropdownOpen(!intervalDropdownOpen)}
								className={cn(
									"px-2 py-2 text-sm flex items-center justify-between gap-1 rounded-r-lg border transition-colors w-[60px]",
									autoRefresh
										? "bg-primary-500 border-primary-500 text-white hover:bg-primary-600"
										: "bg-white dark:bg-gray-800 border-gray-300 dark:border-gray-600 text-gray-700 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-700",
								)}
								title="Auto-refresh interval"
							>
								<span>
									{
										intervalOptions.find((o) => o.value === refreshInterval)
											?.label
									}
								</span>
								<ChevronDown
									className={cn(
										"w-3 h-3 flex-shrink-0 transition-transform",
										intervalDropdownOpen && "rotate-180",
									)}
								/>
							</button>
							{/* Dropdown menu */}
							{intervalDropdownOpen && (
								<div className="absolute top-full right-0 mt-1 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-600 rounded-lg shadow-lg py-1 z-50 min-w-[80px]">
									{intervalOptions.map((option) => (
										<button
											key={option.value}
											type="button"
											onClick={() => {
												setRefreshInterval(option.value);
												setIntervalDropdownOpen(false);
											}}
											className={cn(
												"w-full px-3 py-2 text-left text-sm transition-colors",
												refreshInterval === option.value
													? "bg-primary-100 dark:bg-primary-900/30 text-primary-700 dark:text-primary-300 font-medium"
													: "text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700",
											)}
										>
											{option.label}
										</button>
									))}
								</div>
							)}
						</div>

						{isRunning && (
							<button
								type="button"
								onClick={() => cancelMutation.mutate()}
								disabled={cancelMutation.isPending}
								className="btn-danger flex items-center gap-2"
							>
								<StopCircle className="w-4 h-4" />
								Cancel
							</button>
						)}
						<button
							type="button"
							onClick={() => rerunMutation.mutate()}
							disabled={rerunMutation.isPending || isRunning}
							className="btn-secondary flex items-center gap-2"
						>
							<RefreshCw
								className={cn(
									"w-4 h-4",
									rerunMutation.isPending && "animate-spin",
								)}
							/>
							Re-run
						</button>
						<a
							href={run.html_url}
							target="_blank"
							rel="noopener noreferrer"
							className="btn-secondary flex items-center gap-2"
						>
							<ExternalLink className="w-4 h-4" />
							GitHub
						</a>
					</div>
				</div>

				{/* Info Cards */}
				<div className="grid grid-cols-4 gap-3 mt-3">
					<div className="flex items-center gap-2 px-3 py-2 bg-gray-50 dark:bg-gray-800 rounded-lg">
						<GitBranch className="w-4 h-4 text-gray-400" />
						<span className="text-sm font-medium text-gray-900 dark:text-gray-100 truncate">
							{run.branch}
						</span>
					</div>
					<div className="flex items-center gap-2 px-3 py-2 bg-gray-50 dark:bg-gray-800 rounded-lg">
						<GitCommit className="w-4 h-4 text-gray-400" />
						<span className="text-sm font-mono text-gray-900 dark:text-gray-100">
							{run.commit_sha.substring(0, 7)}
						</span>
					</div>
					<div className="flex items-center gap-2 px-3 py-2 bg-gray-50 dark:bg-gray-800 rounded-lg">
						<User className="w-4 h-4 text-gray-400" />
						<span className="text-sm font-medium text-gray-900 dark:text-gray-100 truncate">
							{run.actor_login}
						</span>
					</div>
					<div className="flex items-center gap-2 px-3 py-2 bg-gray-50 dark:bg-gray-800 rounded-lg">
						<Clock className="w-4 h-4 text-gray-400" />
						<span className="text-sm font-medium text-gray-900 dark:text-gray-100">
							{formatDuration(run.duration_seconds)}
						</span>
					</div>
				</div>
			</div>

			{/* Main Content - Graph and Right Panel */}
			<div className="flex-1 flex min-h-0">
				{/* ReactFlow Graph - Main Area */}
				<div className="flex-1 relative bg-slate-900">
					{jobsLoading ? (
						<div className="flex items-center justify-center h-full">
							<Loader2 className="w-8 h-8 animate-spin text-primary-500" />
						</div>
					) : jobs && jobs.length > 0 ? (
						<ReactFlow
							nodes={nodes}
							edges={edges}
							nodeTypes={nodeTypes}
							fitView
							fitViewOptions={{ padding: 0.8, maxZoom: 1 }}
							minZoom={0.5}
							maxZoom={1.5}
							proOptions={{ hideAttribution: true }}
							nodesDraggable={false}
							nodesConnectable={false}
							elementsSelectable={false}
							panOnDrag={true}
							zoomOnScroll={true}
							zoomOnPinch={true}
							panOnScroll={false}
							preventScrolling={true}
							onNodeClick={(_, node) => {
								const job = jobs?.find((j) => `job-${j.id}` === node.id);
								if (job) handleJobClick(job);
							}}
						>
							<Background color="#334155" gap={20} size={1} />
							<Controls
								showInteractive={false}
								className="!bg-slate-800 !border-slate-600 !rounded-lg !shadow-xl [&>button]:!bg-slate-700 [&>button]:!border-slate-600 [&>button]:!text-slate-300 [&>button:hover]:!bg-slate-600 [&>button]:!w-7 [&>button]:!h-7 [&>button>svg]:!fill-slate-300"
							/>
						</ReactFlow>
					) : run.conclusion === "failure" ? (
						<div className="flex flex-col items-center justify-center h-full p-8 overflow-auto">
							<div className="bg-gradient-to-br from-red-950/50 to-red-900/30 border border-red-700/50 rounded-2xl p-8 max-w-2xl w-full shadow-2xl shadow-red-900/20">
								{/* Header */}
								<div className="text-center mb-6">
									<div className="w-16 h-16 rounded-full bg-red-600/20 border border-red-500/40 flex items-center justify-center mx-auto mb-4">
										<AlertTriangle className="w-8 h-8 text-red-400" />
									</div>
									<h3 className="text-xl font-bold text-white mb-2">
										Workflow Failed
									</h3>
									<p className="text-gray-400 text-sm">
										This workflow failed before any jobs could be executed
									</p>
								</div>

								{/* Annotations */}
								{annotationsLoading ? (
									<div className="flex items-center justify-center gap-3 text-gray-400 text-sm py-6 mb-6 bg-slate-800/50 rounded-xl border border-slate-700/50">
										<Loader2 className="w-5 h-5 animate-spin text-red-400" />
										Loading error details...
									</div>
								) : annotations && annotations.length > 0 ? (
									<div className="space-y-3 mb-6">
										<h4 className="text-xs font-semibold text-gray-400 uppercase tracking-wider">
											Error Details
										</h4>
										{annotations.map((annotation) => (
											<div
												key={`${annotation.path}-${annotation.start_line}-${annotation.message.slice(0, 50)}`}
												className="bg-slate-900/70 rounded-xl p-4 border border-red-600/30"
											>
												{annotation.title && (
													<p className="font-semibold text-red-300 mb-2">
														{annotation.title}
													</p>
												)}
												<p className="text-gray-200 whitespace-pre-wrap font-mono text-sm leading-relaxed">
													{annotation.message}
												</p>
												{annotation.path && (
													<p className="text-gray-500 text-xs mt-3 font-mono bg-slate-800/50 inline-block px-2 py-1 rounded">
														📁 {annotation.path}
														{annotation.start_line > 0 &&
															`:${annotation.start_line}`}
														{annotation.end_line > 0 &&
															annotation.end_line !== annotation.start_line &&
															`-${annotation.end_line}`}
													</p>
												)}
											</div>
										))}
									</div>
								) : (
									<div className="text-center py-6 mb-6 bg-slate-800/30 rounded-xl border border-slate-700/50">
										<p className="text-gray-400 text-sm mb-2">
											Error details not available via API
										</p>
										<p className="text-gray-500 text-xs">
											This usually happens with workflow syntax errors.
											<br />
											Check GitHub for the full error message.
										</p>
									</div>
								)}

								{/* Action Button */}
								<div className="text-center">
									<a
										href={run.html_url}
										target="_blank"
										rel="noopener noreferrer"
										className="inline-flex items-center gap-2 px-6 py-3 bg-red-600 hover:bg-red-500 rounded-xl !text-white hover:!text-white font-medium transition-all shadow-lg shadow-red-900/30 hover:shadow-red-900/50"
									>
										<ExternalLink className="w-4 h-4" />
										View Details on GitHub
									</a>
								</div>
							</div>
						</div>
					) : (
						<div className="flex flex-col items-center justify-center h-full text-gray-500 dark:text-gray-400">
							<Play className="w-16 h-16 mb-4 opacity-30" />
							<p className="text-lg font-medium">No jobs found</p>
							<p className="text-sm">
								Jobs will appear here once the workflow starts
							</p>
						</div>
					)}
				</div>

				{/* Right Panel */}
				<div className="w-80 flex-shrink-0 border-l border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900 flex flex-col min-h-0">
					{/* Timeline Section */}
					<div className="flex-shrink-0 px-4 py-2 border-b border-gray-200 dark:border-gray-700">
						<h2 className="text-xs font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wide mb-1.5 flex items-center gap-1.5">
							<Clock className="w-3 h-3" />
							Timeline
						</h2>
						<div className="space-y-0.5 text-xs">
							<div className="flex items-center justify-between">
								<span className="text-gray-500 dark:text-gray-400">
									Started
								</span>
								<span className="text-gray-900 dark:text-gray-100 font-medium">
									{formatDateTime(run.started_at)}
								</span>
							</div>
							{run.completed_at && (
								<div className="flex items-center justify-between">
									<span className="text-gray-500 dark:text-gray-400">
										Completed
									</span>
									<span className="text-gray-900 dark:text-gray-100 font-medium">
										{formatDateTime(run.completed_at)}
									</span>
								</div>
							)}
						</div>
					</div>

					{/* Jobs List Section with inline accordions */}
					<div className="flex-1 flex flex-col min-h-0 overflow-hidden">
						<div className="flex-shrink-0 px-4 py-2 border-b border-gray-200 dark:border-gray-700">
							<h2 className="text-xs font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wide flex items-center gap-1.5">
								<FileText className="w-3 h-3" />
								Jobs ({jobs?.length || 0})
							</h2>
						</div>
						<div className="flex-1 overflow-y-auto min-h-0 [&::-webkit-scrollbar]:w-0 [&::-webkit-scrollbar]:h-0">
							{jobs && jobs.length > 0 ? (
								<div className="divide-y divide-gray-100 dark:divide-gray-800">
									{jobs.map((job) => {
										const isSelected = selectedJob?.id === job.id;
										const isExpanded = isSelected && stepsExpanded;
										return (
											<div key={job.id} className="relative">
												{/* Green line overlay */}
												{isExpanded && (
													<div className="absolute left-0 top-0 bottom-0 w-0.5 bg-primary-500 z-10" />
												)}
												{/* Job Header - Accordion trigger */}
												<button
													type="button"
													onClick={() => handleJobClick(job)}
													className={cn(
														"w-full px-3 py-2 text-left hover:bg-gray-50 dark:hover:bg-gray-800 transition-colors",
														isSelected &&
															"bg-primary-50 dark:bg-primary-900/20",
													)}
												>
													<div className="flex items-center gap-3">
														<JobStatusIcon
															status={job.status}
															conclusion={job.conclusion}
														/>
														<div className="flex-1 min-w-0">
															<p className="font-medium text-gray-900 dark:text-gray-100 text-sm truncate">
																{job.name}
															</p>
															<div className="flex items-center gap-2 mt-0.5">
																<span
																	className={cn(
																		"text-xs",
																		getStatusColor(
																			job.conclusion || job.status,
																		),
																	)}
																>
																	{job.conclusion || job.status}
																</span>
																<span className="text-xs text-gray-500 dark:text-gray-400">
																	{formatDuration(job.duration_seconds)}
																</span>
															</div>
														</div>
														<ChevronDown
															className={cn(
																"w-4 h-4 text-gray-400 transition-transform duration-200 flex-shrink-0",
																isSelected && stepsExpanded && "rotate-180",
															)}
														/>
													</div>
												</button>

												{/* Accordion Content - Steps */}
												{isExpanded && (
													<div className="bg-gray-50 dark:bg-gray-800/50">
														<div className="px-4 py-3">
															{/* Steps */}
															{job.steps && job.steps.length > 0 && (
																<div className="space-y-1 mb-3">
																	{(
																		job.steps as Array<{
																			name: string;
																			conclusion: string | null;
																			number: number;
																		}>
																	).map((step) => (
																		<div
																			key={step.number}
																			className="flex items-center gap-2 text-xs py-1 pl-2"
																		>
																			<StepStatusIcon
																				conclusion={step.conclusion}
																			/>
																			<span className="text-gray-600 dark:text-gray-300 truncate">
																				{step.name}
																			</span>
																		</div>
																	))}
																</div>
															)}

															<button
																type="button"
																onClick={handleViewLogs}
																className="w-full btn-primary flex items-center justify-center gap-2 text-xs py-1.5"
															>
																<FileText className="w-3.5 h-3.5" />
																View Logs
															</button>
														</div>
													</div>
												)}
											</div>
										);
									})}
								</div>
							) : run.conclusion === "failure" ? (
								<div className="p-4">
									{/* Header */}
									<div className="flex items-center gap-3 mb-4 p-3 bg-red-950/40 rounded-xl border border-red-700/30">
										<div className="w-10 h-10 rounded-full bg-red-600/20 border border-red-500/30 flex items-center justify-center flex-shrink-0">
											<AlertTriangle className="w-5 h-5 text-red-400" />
										</div>
										<div>
											<p className="text-sm font-semibold text-white">
												Workflow Failed
											</p>
											<p className="text-xs text-gray-400">No jobs executed</p>
										</div>
									</div>

									{/* Show annotations in right panel */}
									{annotationsLoading ? (
										<div className="flex items-center justify-center gap-2 text-gray-400 text-xs py-4 bg-slate-800/50 rounded-lg">
											<Loader2 className="w-3 h-3 animate-spin text-red-400" />
											Loading errors...
										</div>
									) : annotations && annotations.length > 0 ? (
										<div className="space-y-2 mb-4">
											{annotations.slice(0, 3).map((annotation) => (
												<div
													key={`${annotation.path}-${annotation.start_line}-${annotation.message.slice(0, 30)}`}
													className="bg-slate-800/70 border border-red-600/20 rounded-lg p-3"
												>
													{annotation.title && (
														<p className="font-medium text-red-300 text-xs mb-1 truncate">
															{annotation.title}
														</p>
													)}
													<p className="text-gray-300 text-xs line-clamp-2">
														{annotation.message}
													</p>
												</div>
											))}
											{annotations.length > 3 && (
												<p className="text-xs text-gray-500 text-center">
													+{annotations.length - 3} more errors
												</p>
											)}
										</div>
									) : (
										<div className="text-center py-3 mb-4 bg-slate-800/30 rounded-lg border border-slate-700/50">
											<p className="text-xs text-gray-500">
												Error details not available via API
											</p>
										</div>
									)}

									<a
										href={run.html_url}
										target="_blank"
										rel="noopener noreferrer"
										className="flex items-center justify-center gap-2 w-full px-3 py-2 bg-red-600 hover:bg-red-500 rounded-lg !text-white hover:!text-white text-xs font-medium transition-colors"
									>
										<ExternalLink className="w-3 h-3" />
										View on GitHub
									</a>
								</div>
							) : (
								<div className="flex flex-col items-center justify-center py-8 text-gray-500 dark:text-gray-400">
									<Clock className="w-8 h-8 mb-2 opacity-30" />
									<p className="text-sm">No jobs found</p>
								</div>
							)}
						</div>
					</div>
				</div>
			</div>
		</div>
	);
}

function RunStatusIcon({
	status,
	conclusion,
	size = "md",
}: {
	status: string;
	conclusion: string | null;
	size?: "md" | "lg";
}) {
	const sizeClass = size === "lg" ? "w-10 h-10" : "w-8 h-8";
	const iconSize = size === "lg" ? "w-5 h-5" : "w-4 h-4";

	if (status === "in_progress") {
		return (
			<div
				className={cn(
					sizeClass,
					"rounded-full bg-blue-100 dark:bg-blue-900 flex items-center justify-center",
				)}
			>
				<Loader2
					className={cn(
						iconSize,
						"text-blue-600 dark:text-blue-400 animate-spin",
					)}
				/>
			</div>
		);
	}

	if (status === "queued" || status === "pending" || status === "waiting") {
		return (
			<div
				className={cn(
					sizeClass,
					"rounded-full bg-amber-100 dark:bg-amber-900 flex items-center justify-center animate-pulse",
				)}
			>
				<Circle
					className={cn(iconSize, "text-amber-600 dark:text-amber-400")}
				/>
			</div>
		);
	}

	if (conclusion === "success") {
		return (
			<div
				className={cn(
					sizeClass,
					"rounded-full bg-green-100 dark:bg-green-900 flex items-center justify-center",
				)}
			>
				<CheckCircle
					className={cn(iconSize, "text-green-600 dark:text-green-400")}
				/>
			</div>
		);
	}

	if (conclusion === "failure") {
		return (
			<div
				className={cn(
					sizeClass,
					"rounded-full bg-red-100 dark:bg-red-900 flex items-center justify-center",
				)}
			>
				<XCircle className={cn(iconSize, "text-red-600 dark:text-red-400")} />
			</div>
		);
	}

	return (
		<div
			className={cn(
				sizeClass,
				"rounded-full bg-gray-100 dark:bg-gray-800 flex items-center justify-center",
			)}
		>
			<Clock className={cn(iconSize, "text-gray-600 dark:text-gray-400")} />
		</div>
	);
}

function JobStatusIcon({
	status,
	conclusion,
}: {
	status: string;
	conclusion: string | null;
}) {
	if (status === "in_progress") {
		return (
			<Loader2 className="w-4 h-4 text-blue-500 animate-spin flex-shrink-0" />
		);
	}
	if (status === "queued" || status === "pending" || status === "waiting") {
		return (
			<Circle className="w-4 h-4 text-amber-500 animate-pulse flex-shrink-0" />
		);
	}
	if (conclusion === "success") {
		return <CheckCircle className="w-4 h-4 text-emerald-500 flex-shrink-0" />;
	}
	if (conclusion === "failure") {
		return <XCircle className="w-4 h-4 text-red-500 flex-shrink-0" />;
	}
	if (conclusion === "skipped") {
		return <Clock className="w-4 h-4 text-gray-400 flex-shrink-0" />;
	}
	return <Clock className="w-4 h-4 text-amber-500 flex-shrink-0" />;
}

function StepStatusIcon({ conclusion }: { conclusion: string | null }) {
	if (conclusion === "success") {
		return <CheckCircle className="w-3 h-3 text-emerald-500 flex-shrink-0" />;
	}
	if (conclusion === "failure") {
		return <XCircle className="w-3 h-3 text-red-500 flex-shrink-0" />;
	}
	if (conclusion === "skipped") {
		return <Clock className="w-3 h-3 text-gray-400 flex-shrink-0" />;
	}
	return <Clock className="w-3 h-3 text-gray-400 flex-shrink-0" />;
}
