import {
	useState,
	useCallback,
	useMemo,
	useRef,
	useEffect,
	useLayoutEffect,
} from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useParams, useNavigate } from "react-router-dom";
import {
	ReactFlow,
	Background,
	Controls,
	useNodesState,
	useEdgesState,
	useReactFlow,
	ReactFlowProvider,
	type Node,
	type Edge,
} from "@xyflow/react";
import dagre from "dagre";
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

// Dagre layout configuration
const NODE_WIDTH = 320;
const NODE_HEIGHT = 70;

// Apply dagre layout algorithm to nodes and edges
function getLayoutedElements(
	nodes: Node[],
	edges: Edge[],
	direction: "TB" | "LR" = "LR",
): { nodes: Node[]; edges: Edge[] } {
	const dagreGraph = new dagre.graphlib.Graph();
	dagreGraph.setDefaultEdgeLabel(() => ({}));

	dagreGraph.setGraph({
		rankdir: direction,
		nodesep: 80, // Vertical spacing between nodes in same rank
		ranksep: 120, // Horizontal spacing between ranks
		marginx: 30,
		marginy: 30,
	});

	// Separate parent nodes from child nodes
	// Child nodes (with parentId) have relative positions and shouldn't be layouted by dagre
	const parentNodes = nodes.filter((node) => !node.parentId);
	const childNodes = nodes.filter((node) => node.parentId);

	// Add only parent/standalone nodes to dagre graph
	for (const node of parentNodes) {
		const width =
			node.type === "matrixGroup" ? (node.data.width as number) : NODE_WIDTH;
		const height =
			node.type === "matrixGroup" ? (node.data.height as number) : NODE_HEIGHT;
		dagreGraph.setNode(node.id, { width, height });
	}

	// Add edges to dagre graph (only edges between parent nodes)
	const parentNodeIds = new Set(parentNodes.map((n) => n.id));
	for (const edge of edges) {
		// Only add edges where both source and target are parent nodes
		if (parentNodeIds.has(edge.source) && parentNodeIds.has(edge.target)) {
			dagreGraph.setEdge(edge.source, edge.target);
		}
	}

	// Run the dagre layout
	dagre.layout(dagreGraph);

	// Apply the calculated positions to parent nodes
	const layoutedParentNodes = parentNodes.map((node) => {
		const nodeWithPosition = dagreGraph.node(node.id);
		const width =
			node.type === "matrixGroup" ? (node.data.width as number) : NODE_WIDTH;
		const height =
			node.type === "matrixGroup" ? (node.data.height as number) : NODE_HEIGHT;

		return {
			...node,
			position: {
				// Dagre gives center positions, we need top-left
				x: nodeWithPosition.x - width / 2,
				y: nodeWithPosition.y - height / 2,
			},
		};
	});

	// Child nodes keep their relative positions (they move with their parent)
	// Return parent nodes first, then child nodes (ReactFlow requires parents before children)
	// Ensure all edges use bezier curves (type: "default")
	const bezierEdges = edges.map((edge) => ({
		...edge,
		type: "default" as const,
	}));
	return { nodes: [...layoutedParentNodes, ...childNodes], edges: bezierEdges };
}

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

// Helper to get edge color based on job status/conclusion (hex format for edges)
function getJobEdgeColor(status: string, conclusion: string | null): string {
	if (status === "in_progress") return "#3b82f6"; // blue-500
	if (conclusion === "success") return "#10b981"; // emerald-500
	if (conclusion === "failure") return "#ef4444"; // red-500
	if (conclusion === "skipped") return "#9ca3af"; // gray-400
	return "#f59e0b"; // amber-500
}

// Helper to get aggregate color for a group of jobs (matrix strategy)
// Priority: failure > in_progress > queued/pending > success > skipped
function getGroupEdgeColor(jobs: WorkflowJob[]): string {
	if (jobs.length === 0) return "#6b7280"; // gray-500

	let hasFailure = false;
	let hasInProgress = false;
	let hasPending = false;
	let hasSuccess = false;

	for (const job of jobs) {
		if (job.conclusion === "failure") hasFailure = true;
		else if (job.status === "in_progress") hasInProgress = true;
		else if (
			job.status === "queued" ||
			job.status === "waiting" ||
			job.status === "pending"
		)
			hasPending = true;
		else if (job.conclusion === "success") hasSuccess = true;
	}

	if (hasFailure) return "#ef4444"; // red-500
	if (hasInProgress) return "#3b82f6"; // blue-500
	if (hasPending) return "#f59e0b"; // amber-500
	if (hasSuccess) return "#10b981"; // emerald-500
	return "#9ca3af"; // gray-400 (all skipped or unknown)
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
	LayoutGrid,
} from "lucide-react";
import { runsApi, jobsApi, repositoriesApi } from "../services/api";
import {
	cn,
	formatDuration,
	formatDateTime,
	getStatusColor,
} from "../lib/utils";
import { useTheme } from "../context/ThemeContext";
import type { WorkflowJob } from "../types";

// Custom node component for jobs in the flow
function JobNode({
	data,
}: {
	data: {
		job: WorkflowJob;
		selected: boolean;
		onClick: () => void;
		displayName?: string;
	};
}) {
	const { job, selected, onClick, displayName } = data;

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

	// Get short name for display (strip prefix if exists)
	const getShortName = (name: string) => {
		const slashIdx = name.indexOf(" / ");
		if (slashIdx > 0) {
			return name.substring(slashIdx + 3);
		}
		return name;
	};

	const shortName = displayName || getShortName(job.name);

	return (
		<button
			type="button"
			onClick={onClick}
			className={cn(
				"px-4 py-3 rounded-xl border-2 cursor-pointer transition-all duration-200 w-[320px] relative",
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
			<div className="flex items-center gap-2">
				<JobStatusIcon status={job.status} conclusion={job.conclusion} />
				<div className="flex-1 min-w-0 text-left">
					<p
						className="font-medium text-gray-900 dark:text-gray-100 text-sm truncate"
						title={job.name}
					>
						{shortName}
					</p>
					<p className="text-xs text-gray-500 dark:text-gray-400">
						{formatDuration(job.duration_seconds)}
					</p>
				</div>
			</div>
		</button>
	);
}

// Custom group node for matrix jobs
function MatrixGroupNode({
	data,
}: {
	data: { label: string; jobs: WorkflowJob[]; width: number; height: number };
}) {
	const { label, jobs } = data;
	const { isDark } = useTheme();

	// Determine overall status of the group
	const hasInProgress = jobs.some((j) => j.status === "in_progress");
	const hasPending = jobs.some(
		(j) => j.status === "queued" || j.status === "pending",
	);
	const allSuccess = jobs.every((j) => j.conclusion === "success");
	const hasFailed = jobs.some((j) => j.conclusion === "failure");

	const getBorderColor = () => {
		if (hasInProgress) return "border-blue-500/50";
		if (hasFailed) return "border-red-500/50";
		if (allSuccess) return "border-emerald-500/50";
		if (hasPending) return "border-amber-500/50";
		return "border-gray-500/50";
	};

	return (
		<div
			className={cn(
				"rounded-xl border-2 border-dashed p-3 pt-8",
				isDark ? "bg-slate-800/30" : "bg-gray-100/50",
				getBorderColor(),
			)}
			style={{ width: data.width, height: data.height }}
		>
			<div
				className={cn(
					"absolute -top-3 left-3 px-2 py-0.5 rounded-sm text-xs font-medium",
					isDark ? "bg-slate-800 text-gray-400" : "bg-gray-200 text-gray-600",
				)}
			>
				Matrix: {label}
			</div>
		</div>
	);
}

const nodeTypes = { jobNode: JobNode, matrixGroup: MatrixGroupNode };

// Inner component that has access to ReactFlow hooks
function RunDetailInner() {
	const { id } = useParams<{ id: string }>();
	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const { isDark } = useTheme();
	const [selectedJob, setSelectedJob] = useState<WorkflowJob | null>(null);
	const [stepsExpanded, setStepsExpanded] = useState(true);
	const [autoRefresh, setAutoRefresh] = useState(false);
	const [refreshInterval, setRefreshInterval] = useState(15); // seconds
	const [intervalDropdownOpen, setIntervalDropdownOpen] = useState(false);
	const intervalDropdownRef = useRef<HTMLDivElement>(null);
	const [cancelError, setCancelError] = useState<string | null>(null);
	const [rerunError, setRerunError] = useState<string | null>(null);

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
		isFetching: isRefetching,
	} = useQuery({
		queryKey: ["runs", id],
		queryFn: () => runsApi.get(Number(id)),
		enabled: !!id,
		staleTime: 0,
	});

	const {
		data: jobs,
		isLoading: jobsLoading,
	} = useQuery({
		queryKey: ["runs", id, "jobs"],
		queryFn: () => runsApi.getJobs(Number(id)),
		enabled: !!id,
		staleTime: 0,
	});

	// Fetch run + jobs from GitHub, update cache directly
	const refreshFromGitHub = useCallback(async () => {
		const numId = Number(id);
		const [freshRun, freshJobs] = await Promise.all([
			runsApi.get(numId, { refresh: true }),
			runsApi.getJobs(numId, { refresh: true }),
		]);
		queryClient.setQueryData(["runs", id], freshRun);
		queryClient.setQueryData(["runs", id, "jobs"], freshJobs);
		return freshRun;
	}, [id, queryClient]);

	// Poll from GitHub when run is active or auto-refresh is enabled
	useEffect(() => {
		if (!run || !id) return;
		const isActive = run.status === "in_progress" || run.status === "queued";
		const ms = isActive ? 5000 : autoRefresh ? refreshInterval * 1000 : 0;
		if (!ms) return;
		const t = setInterval(() => {
			refreshFromGitHub().catch(() => {});
		}, ms);
		return () => clearInterval(t);
	}, [run, run?.status, autoRefresh, refreshInterval, id, refreshFromGitHub]);

	// Fetch workflow definition to get job dependencies (needs)
	const { data: workflowDefinition } = useQuery({
		queryKey: ["runs", id, "workflow-definition"],
		queryFn: async () => {
			const result = await runsApi.getWorkflowDefinition(Number(id));
			return result;
		},
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

	// Manual refresh: fetch run + jobs from GitHub, update cache
	const refreshRunMutation = useMutation({
		mutationFn: () => refreshFromGitHub(),
	});

	const handleRefresh = useCallback(() => {
		refreshRunMutation.mutate();
	}, [refreshRunMutation]);

	const rerunMutation = useMutation({
		mutationFn: () => runsApi.rerun(Number(id)),
		onSuccess: async () => {
			setRerunError(null);
			// Light sync: only this run's repo so the new run appears without a full sync
			try {
				if (run?.repo_id != null) {
					await repositoriesApi.syncRepo(run.repo_id);
				}
			} finally {
				queryClient.invalidateQueries({ queryKey: ["runs"] });
				queryClient.invalidateQueries({ queryKey: ["runs", id] });
				queryClient.invalidateQueries({ queryKey: ["runs", id, "jobs"] });
				if (run?.workflow_id != null) {
					queryClient.invalidateQueries({
						queryKey: ["workflows", run.workflow_id, "runs"],
					});
				}
			}
		},
		onError: (err: Error) => {
			console.error("Re-run failed:", err);
			setRerunError(err.message || "Failed to re-run workflow");
		},
	});

	const cancelMutation = useMutation({
		mutationFn: () => runsApi.cancel(Number(id)),
		onSuccess: () => {
			setCancelError(null);
			queryClient.invalidateQueries({ queryKey: ["runs", id] });
			queryClient.invalidateQueries({ queryKey: ["runs", id, "jobs"] });
		},
		onError: (err: Error) => {
			console.error("Cancel run failed:", err);
			setCancelError(err.message || "Failed to cancel run");
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

	// Stable ref for handleJobClick so node creation doesn't recompute on every selection change
	const handleJobClickRef = useRef(handleJobClick);
	handleJobClickRef.current = handleJobClick;

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

		// Track matrix jobs for grouping
		const matrixJobBaseNames = new Map<string, string>(); // job name -> base name

		// Track reusable workflow prefixes for matching
		const prefixedJobIds = new Map<string, string>(); // job_id -> prefix

		// Track detected reusable workflow prefixes from job names
		// Jobs from reusable workflows have names like "calling_job / inner_job"
		const detectedPrefixes = new Set<string>();
		for (const job of jobs) {
			const slashIdx = job.name.indexOf(" / ");
			if (slashIdx > 0) {
				detectedPrefixes.add(job.name.substring(0, slashIdx));
			}
		}

		if (workflowDefinition && workflowDefinition.length > 0) {
			// First pass: collect prefixes from reusable workflows
			for (const dep of workflowDefinition) {
				if (dep.prefix) {
					prefixedJobIds.set(dep.job_id, dep.prefix);
				}
			}

			// Helper to extract base name from job name
			// Handles: "prefix / base (matrix)" -> base
			// Handles: "prefix / base" -> base
			// Handles: "base (matrix)" -> base
			// Handles: "base" -> base
			const extractBaseName = (
				jobName: string,
			): { prefix: string | null; base: string; hasMatrix: boolean } => {
				let prefix: string | null = null;
				let base = jobName;
				let hasMatrix = false;

				// Check for prefix pattern "prefix / rest"
				const slashIdx = base.indexOf(" / ");
				if (slashIdx > 0) {
					prefix = base.substring(0, slashIdx);
					base = base.substring(slashIdx + 3);
				}

				// Check for matrix pattern "base (matrix values)" or truncated "base (matrix..."
				// GitHub truncates long job names with "..." so we need to handle both cases
				if (
					base.endsWith(")") ||
					(base.includes(" (") && base.includes("..."))
				) {
					const parenIdx = base.lastIndexOf(" (");
					if (parenIdx > 0) {
						base = base.substring(0, parenIdx);
						hasMatrix = true;
					}
				}

				return { prefix, base, hasMatrix };
			};

			// Build mapping from YAML job_id to actual job name
			for (const dep of workflowDefinition) {
				const depNameLower = dep.name.toLowerCase();
				const depJobIdLower = dep.job_id.toLowerCase();
				const depPrefixLower = dep.prefix?.toLowerCase() || null;

				// Try to find matching job by comparing names
				const matchingJobs = jobs.filter((j) => {
					const jobNameLower = j.name.toLowerCase();
					const { prefix, base } = extractBaseName(jobNameLower);
					const baseLower = base.toLowerCase();

					// For jobs with prefix from reusable workflows
					if (depPrefixLower) {
						// The actual job must have a matching prefix
						if (prefix && prefix === depPrefixLower) {
							// And the base name must match either the job name or job_id
							if (baseLower === depNameLower || baseLower === depJobIdLower) {
								return true;
							}
						}
						return false;
					}

					// For jobs without prefix (regular jobs)
					// Exact match
					if (jobNameLower === depNameLower || jobNameLower === depJobIdLower) {
						return true;
					}

					// Job might have matrix suffix: "job_name (matrix values)"
					// Or prefix from somewhere: "prefix / job_name" or "prefix / job_name (matrix)"
					if (baseLower === depNameLower || baseLower === depJobIdLower) {
						// Only match if there's no prefix mismatch
						// (prefix should be null for non-reusable workflow jobs)
						if (!prefix) {
							return true;
						}
					}

					return false;
				});

				// Map all matching jobs
				for (const matchingJob of matchingJobs) {
					// Create a unique key for this job definition (considering prefix)
					const depKey = dep.prefix
						? `${dep.prefix}/${dep.job_id}`
						: dep.job_id;
					if (!jobIdByName.has(depKey)) {
						jobIdByName.set(depKey, matchingJob.name);
					}
					// Also map just the job_id for resolving needs
					if (!jobIdByName.has(dep.job_id)) {
						jobIdByName.set(dep.job_id, matchingJob.name);
					}

					dependencyMap.set(matchingJob.name, dep.needs);

					// Track matrix jobs for grouping
					if (dep.is_matrix) {
						matrixJobBaseNames.set(
							matchingJob.name,
							dep.prefix ? `${dep.prefix} / ${dep.name}` : dep.name,
						);
					}
				}
			}
		}

		// FALLBACK: For jobs that weren't matched by workflow definition,
		// infer dependencies from job name patterns (reusable workflow pattern)
		// Jobs like "prefix / inner_job" where prefix is a reusable workflow caller
		const fallbackJobsAdded: string[] = [];

		// Group unmatched jobs by their prefix to build dependency chains
		const unmatchedJobsByPrefix = new Map<string, WorkflowJob[]>();

		for (const job of jobs) {
			if (dependencyMap.has(job.name)) continue; // Already has dependencies from workflow def

			const slashIdx = job.name.indexOf(" / ");
			if (slashIdx > 0) {
				const prefix = job.name.substring(0, slashIdx);
				const innerPart = job.name.substring(slashIdx + 3);

				// Group by prefix for building dependency chains
				if (!unmatchedJobsByPrefix.has(prefix)) {
					unmatchedJobsByPrefix.set(prefix, []);
				}
				unmatchedJobsByPrefix.get(prefix)!.push(job);
				fallbackJobsAdded.push(job.name);

				// Track for matrix grouping if it has matrix pattern
				// Also handle truncated names that end with "..." but have " (" pattern
				const endsWithParen = innerPart.endsWith(")");
				const isTruncated = innerPart.endsWith("...");
				const parenIdx = innerPart.indexOf(" ("); // Use indexOf to find FIRST " ("

				if (parenIdx > 0 && (endsWithParen || isTruncated)) {
					const baseName = innerPart.substring(0, parenIdx);
					matrixJobBaseNames.set(job.name, `${prefix} / ${baseName}`);
				}
			}
		}

		// Build dependency chains for jobs with same prefix
		// Jobs are chained in the order they appear, with matrix jobs depending on non-matrix jobs
		for (const [, prefixJobs] of unmatchedJobsByPrefix) {
			// Separate matrix jobs from non-matrix jobs
			const nonMatrixJobs = prefixJobs.filter(
				(j) => !matrixJobBaseNames.has(j.name),
			);
			const matrixJobs = prefixJobs.filter((j) =>
				matrixJobBaseNames.has(j.name),
			);

			// Build a linear chain for non-matrix jobs
			for (let i = 0; i < nonMatrixJobs.length; i++) {
				const job = nonMatrixJobs[i];
				if (i === 0) {
					dependencyMap.set(job.name, []); // First job has no dependencies
				} else {
					// Each job depends on the previous one
					const prevJobName = nonMatrixJobs[i - 1].name;
					dependencyMap.set(job.name, [prevJobName]);
					jobIdByName.set(prevJobName, prevJobName); // Ensure it can be resolved
				}
			}

			// Matrix jobs depend on the last non-matrix job (if any)
			const lastNonMatrixJob =
				nonMatrixJobs.length > 0
					? nonMatrixJobs[nonMatrixJobs.length - 1]
					: null;
			for (const job of matrixJobs) {
				if (lastNonMatrixJob) {
					dependencyMap.set(job.name, [lastNonMatrixJob.name]);
					jobIdByName.set(lastNonMatrixJob.name, lastNonMatrixJob.name);
				} else {
					dependencyMap.set(job.name, []);
				}
			}
		}

		// Calculate depth for each job based on dependencies
		const jobDepth = new Map<string, number>();

		// Helper to resolve a job_id to actual job name
		// For reusable workflows, also check for prefixed versions
		// contextPrefix is used when resolving needs within a reusable workflow
		function resolveJobName(
			nameOrId: string,
			contextPrefix?: string,
		): string | undefined {
			// First try direct matches
			if (jobByName.has(nameOrId)) return nameOrId;
			if (jobIdByName.has(nameOrId)) return jobIdByName.get(nameOrId);

			// If we have a context prefix, try to find a job with that prefix
			if (contextPrefix) {
				const prefixedName = `${contextPrefix} / ${nameOrId}`;
				if (jobByName.has(prefixedName)) return prefixedName;

				// Also check for matrix variants
				for (const [jobName] of jobByName) {
					if (jobName.startsWith(prefixedName + " (")) return jobName;
				}
			}

			// Try to find a job that matches with or without prefix/matrix
			for (const [jobName] of jobByName) {
				const lowerJobName = jobName.toLowerCase();
				const lowerNameOrId = nameOrId.toLowerCase();

				// Check if job name matches with or without prefix/matrix
				if (lowerJobName === lowerNameOrId) return jobName;
				if (lowerJobName.startsWith(lowerNameOrId + " (")) return jobName;
				if (lowerJobName.startsWith(lowerNameOrId + " / ")) return jobName;

				// Check if the base part matches (after stripping prefix and matrix)
				const slashIdx = lowerJobName.indexOf(" / ");
				if (slashIdx > 0) {
					const afterSlash = lowerJobName.substring(slashIdx + 3);
					if (afterSlash === lowerNameOrId) return jobName;
					if (afterSlash.startsWith(lowerNameOrId + " (")) return jobName;
				}
			}

			return undefined;
		}

		// Helper to get the prefix from a job name
		function getJobPrefix(jobName: string): string | undefined {
			const slashIdx = jobName.indexOf(" / ");
			if (slashIdx > 0) {
				return jobName.substring(0, slashIdx);
			}
			return undefined;
		}

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

			// Get the prefix of the current job for context-aware resolution
			const contextPrefix = getJobPrefix(jobName);

			let maxParentDepth = -1;
			for (const needId of needs) {
				// Map the need job_id to actual job name, using context prefix
				const needName = resolveJobName(needId, contextPrefix);
				if (needName) {
					const parentDepth = calculateDepth(needName, new Set(visited));
					maxParentDepth = Math.max(maxParentDepth, parentDepth);
				}
			}

			const depth = maxParentDepth >= 0 ? maxParentDepth + 1 : 1;
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
		for (const [jobName, needs] of dependencyMap) {
			const contextPrefix = getJobPrefix(jobName);
			for (const needId of needs) {
				const needName = resolveJobName(needId, contextPrefix);
				if (needName) {
					jobsWithChildren.add(needName);
				}
			}
		}

		// Identify orphan jobs (no dependencies AND no children AND not part of a workflow group)
		// These will be placed in the leftmost column (depth 0), below other jobs
		const orphanJobs: WorkflowJob[] = [];
		const connectedJobs: WorkflowJob[] = [];

		for (const job of jobs) {
			const needs = dependencyMap.get(job.name) || [];
			const hasChildren = jobsWithChildren.has(job.name);
			const hasDependencies = needs.length > 0;
			// A job is "connected" if it's in the dependency map (even with empty deps)
			// This handles reusable workflow jobs that we couldn't get full info for
			const isInDependencyMap = dependencyMap.has(job.name);

			if (!hasChildren && !hasDependencies && !isInDependencyMap) {
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

		// Helper to extract grouping key from job name for MATRIX grouping only
		// Returns base name for matrix jobs (jobs with "(matrix values)" suffix)
		// Also handles truncated job names (ending with "...") where the ")" was cut off
		const getMatrixGroupKey = (jobName: string): string | null => {
			// First check if we have explicit matrix info from workflow definition
			if (matrixJobBaseNames.has(jobName)) {
				return matrixJobBaseNames.get(jobName)!;
			}
			// Only use name-parsing heuristic when no workflow definition is available
			if (workflowDefinition && workflowDefinition.length > 0) {
				return null;
			}

			// Check for prefix pattern "prefix / rest"
			const slashIdx = jobName.indexOf(" / ");
			if (slashIdx > 0) {
				const prefix = jobName.substring(0, slashIdx);
				const rest = jobName.substring(slashIdx + 3);
				// Check if this has matrix values (parentheses suffix)
				// Also handle truncated names that end with "..." but have " (" pattern
				const endsWithParen = rest.endsWith(")");
				const isTruncated = rest.endsWith("...");
				// Find parentheses - try " (" first, then just "("
				let parenIdx = rest.indexOf(" (");
				if (parenIdx < 0) {
					parenIdx = rest.indexOf("(");
				}

				if (parenIdx > 0 && (endsWithParen || isTruncated)) {
					// Return "prefix / base" as group key for matrix jobs
					return prefix + " / " + rest.substring(0, parenIdx).trim();
				}
				// No matrix pattern - don't group by prefix alone
				return null;
			}
			// Check for matrix pattern without prefix
			const endsWithParen = jobName.endsWith(")");
			const isTruncated = jobName.endsWith("...");
			// Find parentheses - try " (" first, then just "("
			let parenIdx = jobName.indexOf(" (");
			if (parenIdx < 0) {
				parenIdx = jobName.indexOf("(");
			}

			if (parenIdx > 0 && (endsWithParen || isTruncated)) {
				return jobName.substring(0, parenIdx).trim();
			}
			return null;
		};

		// Only run name-based matrix grouping heuristic when no workflow definition is available
		if (!workflowDefinition || workflowDefinition.length === 0) {
			// Also group jobs by their base name if multiple jobs share the same base
			// This helps catch matrix jobs even if the pattern detection missed them
			const potentialMatrixGroups = new Map<string, WorkflowJob[]>();
			for (const job of connectedJobs) {
				// Extract base name (everything before first parenthesis or the whole name)
				let baseName = job.name;
				const slashIdx = baseName.indexOf(" / ");
				if (slashIdx > 0) {
					baseName = baseName.substring(slashIdx + 3);
				}
				const parenIdx = baseName.indexOf("(");
				if (parenIdx > 0) {
					baseName = baseName.substring(0, parenIdx).trim();
				}
				// Include prefix in key if present
				const fullKey =
					slashIdx > 0
						? job.name.substring(0, slashIdx) + " / " + baseName
						: baseName;

				if (!potentialMatrixGroups.has(fullKey)) {
					potentialMatrixGroups.set(fullKey, []);
				}
				potentialMatrixGroups.get(fullKey)?.push(job);
			}

			// If multiple jobs share the same base name, add them to matrixJobBaseNames
			for (const [baseName, groupJobs] of potentialMatrixGroups.entries()) {
				if (groupJobs.length > 1) {
					for (const job of groupJobs) {
						if (!matrixJobBaseNames.has(job.name)) {
							matrixJobBaseNames.set(job.name, baseName);
						}
					}
				}
			}
		}

		// Group matrix jobs by their base name for visual clustering
		const matrixGroups = new Map<string, WorkflowJob[]>();
		const standaloneJobs: WorkflowJob[] = [];

		for (const job of connectedJobs) {
			const groupKey = getMatrixGroupKey(job.name);
			if (groupKey) {
				if (!matrixGroups.has(groupKey)) {
					matrixGroups.set(groupKey, []);
				}
				matrixGroups.get(groupKey)?.push(job);
			} else {
				standaloneJobs.push(job);
			}
		}

		// Keep all matrix groups, even with single items, to show the matrix container

		// Layout nodes
		const depths = Array.from(groups.keys()).sort((a, b) => a - b);
		const spacing = { x: 200, y: 90 }; // Minimal x for very short connections
		const groupSpacing = { x: 380, y: 100 }; // Adjusted for larger groups
		const startX = 30;
		const centerY = 100;

		// Track which jobs have been placed (to avoid duplicates)
		const placedJobs = new Set<number>();

		// First pass: layout standalone jobs and matrix groups by depth
		let maxY = 0;
		let groupIdCounter = 0;
		// Map from groupKey -> group node ID for edge connections
		const groupKeyToNodeId = new Map<string, string>();

		depths.forEach((depth, col) => {
			const depthJobs = groups.get(depth) || [];
			let currentY = centerY;

			// Process jobs at this depth
			for (const job of depthJobs) {
				if (placedJobs.has(job.id)) continue;

				const groupKey = getMatrixGroupKey(job.name);
				const matrixGroup = groupKey ? matrixGroups.get(groupKey) : null;

				if (matrixGroup && matrixGroup.length >= 1) {
					// Check if all jobs in this matrix group are at the same depth
					const allSameDepth = matrixGroup.every(
						(j) => jobDepth.get(j.name) === depth,
					);

					if (allSameDepth && groupKey) {
						// Layout matrix group as a cluster
						const groupJobs = matrixGroup;
						const innerSpacing = 85; // Vertical spacing between nodes inside group
						const groupPadding = { top: 45, bottom: 25, left: 15, right: 15 };
						const groupHeight =
							groupPadding.top +
							groupJobs.length * innerSpacing +
							groupPadding.bottom;
						const groupWidth = 350; // Larger width for better readability
						const groupY = currentY;

						// Create group container node
						const groupNodeId = `group-${groupIdCounter}`;
						groupKeyToNodeId.set(groupKey, groupNodeId);
						nodeList.push({
							id: groupNodeId,
							type: "matrixGroup",
							position: {
								x: startX + col * groupSpacing.x,
								y: groupY,
							},
							data: {
								label: groupKey,
								jobs: groupJobs,
								width: groupWidth,
								height: groupHeight,
							},
							draggable: true,
							selectable: true,
							connectable: false,
						});

						// Layout jobs inside the group (as children of the group node)
						groupJobs.forEach((gJob, idx) => {
							placedJobs.add(gJob.id);
							nodeList.push({
								id: `job-${gJob.id}`,
								type: "jobNode",
								// Position is relative to parent node
								position: {
									x: groupPadding.left,
									y: groupPadding.top + idx * innerSpacing,
								},
								parentId: groupNodeId, // Makes this node a child of the group
								extent: "parent" as const, // Constrain movement within parent
								data: {
									job: gJob,
									selected: false,
									onClick: () => handleJobClickRef.current(gJob),
								},
								draggable: false, // Child nodes move with parent, not independently
								selectable: true,
								connectable: false,
							});
						});

						currentY = groupY + groupHeight + spacing.y;
						maxY = Math.max(maxY, currentY);
						groupIdCounter++;
					}
				} else {
					// Standalone job
					placedJobs.add(job.id);
					nodeList.push({
						id: `job-${job.id}`,
						type: "jobNode",
						position: {
							x: startX + col * groupSpacing.x,
							y: currentY,
						},
						data: {
							job,
							selected: false,
							onClick: () => handleJobClickRef.current(job),
						},
						draggable: true,
						selectable: true,
						connectable: false,
					});
					currentY += spacing.y;
					maxY = Math.max(maxY, currentY);
				}
			}
		});

		// Second pass: layout any remaining unplaced jobs
		for (const job of connectedJobs) {
			if (!placedJobs.has(job.id)) {
				const depth = jobDepth.get(job.name) ?? 0;
				placedJobs.add(job.id);
				maxY += spacing.y;
				nodeList.push({
					id: `job-${job.id}`,
					type: "jobNode",
					position: {
						x: startX + depth * groupSpacing.x,
						y: maxY,
					},
					data: {
						job,
						selected: false,
						onClick: () => handleJobClickRef.current(job),
					},
					draggable: true,
					selectable: true,
					connectable: false,
				});
			}
		}

		// Third pass: layout orphan jobs in the leftmost column, below all other jobs
		if (orphanJobs.length > 0) {
			const orphanStartY = maxY + spacing.y;

			orphanJobs.forEach((job, idx) => {
				if (placedJobs.has(job.id)) return;
				placedJobs.add(job.id);
				nodeList.push({
					id: `job-${job.id}`,
					type: "jobNode",
					position: {
						x: startX,
						y: orphanStartY + idx * spacing.y,
					},
					data: {
						job,
						selected: false,
						onClick: () => handleJobClickRef.current(job),
					},
					draggable: true,
					selectable: true,
					connectable: false,
				});
			});
		}

		// Draw edges based on actual dependencies from workflow definition
		// Helper to get all ancestors of a job (for transitive dependency filtering)
		const ancestorCache = new Map<string, Set<string>>();
		function getAncestors(jobName: string): Set<string> {
			if (ancestorCache.has(jobName)) {
				return ancestorCache.get(jobName)!;
			}
			const ancestors = new Set<string>();
			const needs = dependencyMap.get(jobName) || [];
			const contextPrefix = getJobPrefix(jobName);
			for (const needId of needs) {
				const parentName = resolveJobName(needId, contextPrefix);
				if (parentName) {
					ancestors.add(parentName);
					for (const ancestor of getAncestors(parentName)) {
						ancestors.add(ancestor);
					}
				}
			}
			ancestorCache.set(jobName, ancestors);
			return ancestors;
		}

		// Filter out transitive dependencies (simplify visualization like GitHub)
		function filterTransitiveDependencies(
			jobName: string,
			needs: string[],
		): string[] {
			if (needs.length <= 1) return needs;
			const contextPrefix = getJobPrefix(jobName);
			const resolvedNeeds = needs
				.map((needId) => ({
					needId,
					jobName: resolveJobName(needId, contextPrefix),
				}))
				.filter((n) => n.jobName);

			const filtered: string[] = [];
			for (const need of resolvedNeeds) {
				const isTransitive = resolvedNeeds.some((other) => {
					if (other.needId === need.needId) return false;
					const ancestors = getAncestors(other.jobName!);
					return ancestors.has(need.jobName!);
				});
				if (!isTransitive) {
					filtered.push(need.needId);
				}
			}
			return filtered;
		}

		// Track edges already created to avoid duplicates
		const createdEdges = new Set<string>();

		// For matrix groups, we want edges to connect to the group container node
		// This creates a cleaner visualization with edges going to/from the group box
		const getRepresentativeJobId = (job: WorkflowJob): string => {
			const groupKey = getMatrixGroupKey(job.name);
			// If this job is part of a matrix group, return the group node ID
			if (groupKey && groupKeyToNodeId.has(groupKey)) {
				return groupKeyToNodeId.get(groupKey)!;
			}
			return `job-${job.id}`;
		};

		for (const job of jobs) {
			const allNeeds = dependencyMap.get(job.name) || [];
			const needs = filterTransitiveDependencies(job.name, allNeeds);
			const contextPrefix = getJobPrefix(job.name);

			for (const needId of needs) {
				const needName = resolveJobName(needId, contextPrefix);
				const parentJob = needName ? jobByName.get(needName) : undefined;

				if (parentJob) {
					const sourceId = getRepresentativeJobId(parentJob);
					const targetId = getRepresentativeJobId(job);
					const edgeKey = `${sourceId}-${targetId}`;

					// Skip duplicate edges (can happen with matrix groups)
					if (createdEdges.has(edgeKey)) continue;
					createdEdges.add(edgeKey);

					// Check if either job is part of a matrix group
					const sourceGroupKey = getMatrixGroupKey(parentJob.name);
					const targetGroupKey = getMatrixGroupKey(job.name);
					const isSourceInGroup =
						sourceGroupKey && matrixGroups.has(sourceGroupKey);
					const isTargetInGroup =
						targetGroupKey && matrixGroups.has(targetGroupKey);

					edgeList.push({
						id: `edge-${edgeKey}`,
						source: sourceId,
						target: targetId,
						animated:
							job.status === "in_progress" ||
							parentJob.status === "in_progress",
						style: {
							stroke:
								isSourceInGroup || isTargetInGroup ? "#4ade80" : "#6b7280",
							strokeWidth: 2,
							strokeLinecap: "round",
							strokeLinejoin: "round",
						},
						type: "default",
					});
				}
			}
		}

		return { nodes: nodeList, edges: edgeList };
	}, [jobs, workflowDefinition]);

	// State management for ReactFlow nodes and edges
	const [stateNodes, setStateNodes, onNodesChange] = useNodesState<Node>([]);
	const [stateEdges, setStateEdges, onEdgesChange] = useEdgesState<Edge>([]);
	const { fitView } = useReactFlow();

	// Track if layout has been applied for the current structural key
	const syncedStructuralKeyRef = useRef<string>("");

	// Structural key: only changes when jobs are added/removed (triggers re-layout)
	const structuralKey = useMemo(
		() =>
			nodes
				.map((n) => n.id)
				.sort((a, b) => a.localeCompare(b))
				.join(","),
		[nodes],
	);

	// Data key: changes when job statuses/conclusions change (triggers data-only update)
	const dataKey = useMemo(() => {
		if (!jobs) return "";
		return jobs
			.map((j) => `${j.id}:${j.status}:${j.conclusion ?? ""}`)
			.sort()
			.join(",");
	}, [jobs]);

	const isStructureSynced =
		syncedStructuralKeyRef.current === structuralKey && structuralKey !== "";

	// Compute fresh layout from source nodes (only used for initial layout or structural changes)
	const freshLayout = useMemo(() => {
		if (nodes.length === 0) {
			return { nodes: [] as Node[], edges: [] as Edge[] };
		}
		return getLayoutedElements(nodes, edges, "LR");
	}, [nodes, edges]);

	// When structure hasn't changed, update job data in existing state nodes without re-layout
	// When structure has changed, use fresh layout
	const baseNodes = isStructureSynced ? stateNodes : freshLayout.nodes;
	const baseEdges = isStructureSynced ? stateEdges : freshLayout.edges;

	// Build a map from job ID to latest job data for fast lookups
	const jobById = useMemo(() => {
		const map = new Map<number, WorkflowJob>();
		if (jobs) {
			for (const job of jobs) {
				map.set(job.id, job);
			}
		}
		return map;
	}, [jobs]);

	// Update node data (job status, selection) without changing positions
	const layoutedNodes = useMemo(() => {
		return baseNodes.map((node) => {
			const jobIdMatch = /^job-(\d+)$/.exec(node.id);
			const nodeData = node.data as {
				job?: WorkflowJob;
				selected?: boolean;
				onClick?: () => void;
			};
			if (jobIdMatch && nodeData?.job) {
				const latestJob = jobById.get(nodeData.job.id) ?? nodeData.job;
				const isSelected = selectedJob?.id === latestJob.id;
				return {
					...node,
					data: {
						...nodeData,
						job: latestJob,
						selected: isSelected,
					},
				};
			}
			// Update matrix group nodes with latest job data
			if (node.type === "matrixGroup" && nodeData && Array.isArray((nodeData as Record<string, unknown>).jobs)) {
				const groupJobs = (nodeData as Record<string, unknown>).jobs as WorkflowJob[];
				const updatedGroupJobs = groupJobs.map((gj) => jobById.get(gj.id) ?? gj);
				return {
					...node,
					data: {
						...nodeData,
						jobs: updatedGroupJobs,
					},
				};
			}
			return node;
		});
	}, [baseNodes, selectedJob, jobById]);

	// Find all edges that lead to the selected node (for animation)
	const selectedNodeId = selectedJob ? `job-${selectedJob.id}` : null;

	// Find the parent group ID if the selected node is inside a matrix group
	const selectedNodeParentId = useMemo(() => {
		if (!selectedNodeId) return null;
		const selectedNode = baseNodes.find((n) => n.id === selectedNodeId);
		return selectedNode?.parentId || null;
	}, [selectedNodeId, baseNodes]);

	// Get all ancestor node IDs that connect to the selected node (or its parent group)
	const ancestorNodeIds = useMemo(() => {
		if (!selectedNodeId) return new Set<string>();

		const ancestors = new Set<string>();
		const visited = new Set<string>();

		// Start BFS from both the selected node and its parent group (if any)
		const startNodes = [selectedNodeId];
		if (selectedNodeParentId) {
			startNodes.push(selectedNodeParentId);
		}

		const queue = [...startNodes];
		while (queue.length > 0) {
			const currentId = queue.shift();
			if (!currentId || visited.has(currentId)) continue;
			visited.add(currentId);

			// Find all edges where this node is the target
			for (const edge of baseEdges) {
				if (edge.target === currentId && !visited.has(edge.source)) {
					ancestors.add(edge.source);
					queue.push(edge.source);
				}
			}
		}

		return ancestors;
	}, [selectedNodeId, selectedNodeParentId, baseEdges]);

	// Get the color for the selected node based on its status
	const selectedNodeColor = selectedJob
		? getJobEdgeColor(selectedJob.status, selectedJob.conclusion)
		: "#10b981";

	// Build a map from node ID to job for edge coloring
	const nodeIdToJob = useMemo(() => {
		const map = new Map<string, WorkflowJob>();
		for (const [id, job] of jobById) {
			map.set(`job-${id}`, job);
		}
		return map;
	}, [jobById]);

	// Build a map from group node ID to jobs for matrix group edge coloring
	const groupIdToJobs = useMemo(() => {
		const map = new Map<string, WorkflowJob[]>();
		for (const node of layoutedNodes) {
			if (node.type === "matrixGroup" && node.data?.jobs) {
				map.set(node.id, node.data.jobs as WorkflowJob[]);
			}
		}
		return map;
	}, [layoutedNodes]);

	// Update edges with animation for paths leading to selected node
	// Color each edge based on its target node's status
	const layoutedEdges = useMemo(() => {
		return baseEdges.map((edge) => {
			// Check if this edge is part of the path to the selected node or its parent group
			const isPathToSelected =
				selectedNodeId &&
				(edge.target === selectedNodeId ||
					edge.target === selectedNodeParentId ||
					ancestorNodeIds.has(edge.target));

			// Get the target color - check if it's a matrix group first
			let targetColor = "#6b7280"; // gray-500 default
			const groupJobs = groupIdToJobs.get(edge.target);
			if (groupJobs) {
				// Target is a matrix group - use aggregate color
				targetColor = getGroupEdgeColor(groupJobs);
			} else {
				// Target is a regular job node
				const targetJob = nodeIdToJob.get(edge.target);
				if (targetJob) {
					targetColor = getJobEdgeColor(targetJob.status, targetJob.conclusion);
				}
			}

			return {
				...edge,
				type: "default", // Force bezier curves - no 90 degree angles
				animated: isPathToSelected || edge.animated,
				style: {
					...edge.style,
					stroke: isPathToSelected ? selectedNodeColor : targetColor,
					strokeWidth: isPathToSelected ? 3 : edge.style?.strokeWidth || 2,
					strokeLinecap: "round" as const,
					strokeLinejoin: "round" as const,
				},
			};
		});
	}, [
		baseEdges,
		selectedNodeId,
		selectedNodeParentId,
		ancestorNodeIds,
		selectedNodeColor,
		nodeIdToJob,
		groupIdToJobs,
	]);

	// Apply layout only when the graph structure changes (jobs added/removed)
	useLayoutEffect(() => {
		if (nodes.length === 0) return;
		if (structuralKey === syncedStructuralKeyRef.current) return;

		syncedStructuralKeyRef.current = structuralKey;
		setStateNodes(freshLayout.nodes);
		setStateEdges(freshLayout.edges);

		setTimeout(() => {
			fitView({ padding: 0.2, maxZoom: 1 });
		}, 50);
	}, [structuralKey, nodes.length, freshLayout, setStateNodes, setStateEdges, fitView]);

	// When only job data changes (status/conclusion), update node data in-place
	// without resetting layout positions
	useEffect(() => {
		if (!isStructureSynced || !jobs || jobs.length === 0) return;

		setStateNodes((prevNodes) =>
			prevNodes.map((node) => {
				const jobIdMatch = /^job-(\d+)$/.exec(node.id);
				const nodeData = node.data as { job?: WorkflowJob };
				if (jobIdMatch && nodeData?.job) {
					const latestJob = jobById.get(nodeData.job.id);
					if (latestJob && (latestJob.status !== nodeData.job.status || latestJob.conclusion !== nodeData.job.conclusion)) {
						return { ...node, data: { ...nodeData, job: latestJob } };
					}
				}
				if (node.type === "matrixGroup" && nodeData && Array.isArray((nodeData as Record<string, unknown>).jobs)) {
					const groupJobs = (nodeData as Record<string, unknown>).jobs as WorkflowJob[];
					const updatedGroupJobs = groupJobs.map((gj) => jobById.get(gj.id) ?? gj);
					const hasChanges = groupJobs.some((gj, i) => gj.status !== updatedGroupJobs[i].status || gj.conclusion !== updatedGroupJobs[i].conclusion);
					if (hasChanges) {
						return { ...node, data: { ...nodeData, jobs: updatedGroupJobs } };
					}
				}
				return node;
			}),
		);

		setStateEdges((prevEdges) =>
			prevEdges.map((edge) => {
				const targetJob = jobById.get(Number(edge.target.replace("job-", "")));
				const sourceJob = jobById.get(Number(edge.source.replace("job-", "")));
				const shouldAnimate =
					(targetJob?.status === "in_progress") ||
					(sourceJob?.status === "in_progress");
				if (edge.animated !== shouldAnimate) {
					return { ...edge, animated: shouldAnimate };
				}
				return edge;
			}),
		);
	}, [dataKey, isStructureSynced, jobs, jobById, setStateNodes, setStateEdges]);

	// Apply dagre layout to nodes and edges (for manual re-layout button)
	const applyLayout = useCallback(() => {
		if (nodes.length === 0) return;

		const { nodes: newNodes, edges: newEdges } = getLayoutedElements(
			nodes,
			edges,
			"LR",
		);

		setStateNodes(newNodes);
		setStateEdges(newEdges);

		// Fit view after layout is applied
		setTimeout(() => {
			fitView({ padding: 0.2, maxZoom: 1 });
		}, 50);
	}, [nodes, edges, setStateNodes, setStateEdges, fitView]);

	// Track if layout is ready
	const isLayoutReady = layoutedNodes.length > 0;

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

			{/* Cancel / Re-run error overlay - center middle of page */}
			{(cancelError || rerunError) && (
				<div
					className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50"
					role="alert"
					aria-live="assertive"
				>
					<div
						className={cn(
							"flex items-center gap-3 px-5 py-4 rounded-xl text-sm max-w-md shadow-2xl",
							"bg-red-50 dark:bg-red-950/90 border-2 border-red-200 dark:border-red-700 text-red-800 dark:text-red-200",
						)}
					>
						<AlertTriangle className="w-6 h-6 shrink-0 text-red-500" />
						<span className="flex-1">{cancelError ?? rerunError}</span>
						<button
							type="button"
							onClick={() => {
								setCancelError(null);
								setRerunError(null);
							}}
							className="p-1.5 rounded-lg hover:bg-red-100 dark:hover:bg-red-900/50 text-red-600 dark:text-red-400 transition-colors"
							aria-label="Dismiss"
						>
							<XCircle className="w-5 h-5" />
						</button>
					</div>
				</div>
			)}

			{/* Header */}
			<div className="shrink-0 px-4 py-3 border-b border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900">
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
							disabled={isRefetching || refreshRunMutation.isPending}
							className="btn-secondary flex items-center gap-2"
							title="Refresh run data from GitHub"
						>
							<RotateCw
								className={cn(
									"w-4 h-4",
									(isRefetching || refreshRunMutation.isPending) && "animate-spin",
								)}
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
										"w-3 h-3 shrink-0 transition-transform",
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
								onClick={() => {
									setCancelError(null);
									cancelMutation.mutate();
								}}
								disabled={cancelMutation.isPending}
								className="btn-danger flex items-center gap-2"
							>
								{cancelMutation.isPending ? (
									<Loader2 className="w-4 h-4 animate-spin" />
								) : (
									<StopCircle className="w-4 h-4" />
								)}
								Cancel
							</button>
						)}
						<button
							type="button"
							onClick={() => {
								setRerunError(null);
								rerunMutation.mutate();
							}}
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
				<div
					className={cn(
						"flex-1 relative",
						isDark ? "bg-slate-900" : "bg-gray-50",
					)}
				>
					{jobsLoading || (jobs && jobs.length > 0 && !isLayoutReady) ? (
						<div className="flex items-center justify-center h-full">
							<Loader2 className="w-8 h-8 animate-spin text-primary-500" />
						</div>
					) : jobs && jobs.length > 0 && isLayoutReady ? (
						<>
							<ReactFlow
								nodes={layoutedNodes}
								edges={layoutedEdges}
								onNodesChange={onNodesChange}
								onEdgesChange={onEdgesChange}
								nodeTypes={nodeTypes}
								defaultEdgeOptions={{
									type: "default",
									style: { strokeLinecap: "round", strokeLinejoin: "round" },
								}}
								fitView
								fitViewOptions={{ padding: 0.2, maxZoom: 1 }}
								minZoom={0.3}
								maxZoom={1.5}
								proOptions={{ hideAttribution: true }}
								nodesDraggable={true}
								nodesConnectable={false}
								elementsSelectable={true}
								panOnDrag={true}
								zoomOnScroll={true}
								zoomOnPinch={true}
								panOnScroll={false}
								preventScrolling={true}
								onNodeClick={(_, node) => {
									const job = jobs?.find((j) => `job-${j.id}` === node.id);
									if (job) handleJobClick(job);
								}}
								onPaneClick={() => {
									// Deselect job when clicking on background
									setSelectedJob(null);
									setStepsExpanded(false);
								}}
							>
								<Background
									color={isDark ? "#334155" : "#d1d5db"}
									gap={20}
									size={1}
								/>
								<Controls
									showInteractive={false}
									className={cn(
										"rounded-lg! shadow-xl! [&>button]:w-7! [&>button]:h-7!",
										isDark
											? "bg-slate-800! border-slate-600! [&>button]:bg-slate-700! [&>button]:border-slate-600! [&>button]:text-slate-300! [&>button:hover]:bg-slate-600! [&>button>svg]:fill-slate-300!"
											: "bg-white! border-gray-300! [&>button]:bg-gray-100! [&>button]:border-gray-300! [&>button]:text-gray-700! [&>button:hover]:bg-gray-200! [&>button>svg]:fill-gray-600!",
									)}
								/>
							</ReactFlow>
							{/* Auto-layout button */}
							<button
								type="button"
								onClick={applyLayout}
								className={cn(
									"absolute top-3 right-3 px-3 py-2 rounded-lg text-sm font-medium flex items-center gap-2 transition-colors shadow-lg z-10",
									isDark
										? "bg-slate-700 hover:bg-slate-600 border border-slate-600 text-slate-200"
										: "bg-white hover:bg-gray-100 border border-gray-300 text-gray-700",
								)}
								title="Auto-arrange nodes"
							>
								<LayoutGrid className="w-4 h-4" />
								Auto Layout
							</button>
						</>
					) : run.conclusion === "failure" ? (
						<div className="flex flex-col items-center justify-center h-full p-8 overflow-auto">
							<div
								className={cn(
									"border rounded-2xl p-8 max-w-2xl w-full shadow-2xl",
									isDark
										? "bg-linear-to-br from-red-950/50 to-red-900/30 border-red-700/50 shadow-red-900/20"
										: "bg-linear-to-br from-red-50 to-red-100/50 border-red-200 shadow-red-200/50",
								)}
							>
								{/* Header */}
								<div className="text-center mb-6">
									<div
										className={cn(
											"w-16 h-16 rounded-full border flex items-center justify-center mx-auto mb-4",
											isDark
												? "bg-red-600/20 border-red-500/40"
												: "bg-red-100 border-red-300",
										)}
									>
										<AlertTriangle
											className={cn(
												"w-8 h-8",
												isDark ? "text-red-400" : "text-red-500",
											)}
										/>
									</div>
									<h3
										className={cn(
											"text-xl font-bold mb-2",
											isDark ? "text-white" : "text-gray-900",
										)}
									>
										Workflow Failed
									</h3>
									<p
										className={cn(
											"text-sm",
											isDark ? "text-gray-400" : "text-gray-600",
										)}
									>
										This workflow failed before any jobs could be executed
									</p>
								</div>

								{/* Annotations */}
								{annotationsLoading ? (
									<div
										className={cn(
											"flex items-center justify-center gap-3 text-sm py-6 mb-6 rounded-xl border",
											isDark
												? "text-gray-400 bg-slate-800/50 border-slate-700/50"
												: "text-gray-500 bg-gray-100 border-gray-200",
										)}
									>
										<Loader2 className="w-5 h-5 animate-spin text-red-400" />
										Loading error details...
									</div>
								) : annotations && annotations.length > 0 ? (
									<div className="space-y-3 mb-6">
										<h4
											className={cn(
												"text-xs font-semibold uppercase tracking-wider",
												isDark ? "text-gray-400" : "text-gray-500",
											)}
										>
											Error Details
										</h4>
										{annotations.map((annotation) => (
											<div
												key={`${annotation.path}-${annotation.start_line}-${annotation.message.slice(0, 50)}`}
												className={cn(
													"rounded-xl p-4 border",
													isDark
														? "bg-slate-900/70 border-red-600/30"
														: "bg-white border-red-200",
												)}
											>
												{annotation.title && (
													<p
														className={cn(
															"font-semibold mb-2",
															isDark ? "text-red-300" : "text-red-600",
														)}
													>
														{annotation.title}
													</p>
												)}
												<p
													className={cn(
														"whitespace-pre-wrap font-mono text-sm leading-relaxed",
														isDark ? "text-gray-200" : "text-gray-700",
													)}
												>
													{annotation.message}
												</p>
												{annotation.path && (
													<p
														className={cn(
															"text-xs mt-3 font-mono inline-block px-2 py-1 rounded-sm",
															isDark
																? "text-gray-500 bg-slate-800/50"
																: "text-gray-500 bg-gray-100",
														)}
													>
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
									<div
										className={cn(
											"text-center py-6 mb-6 rounded-xl border",
											isDark
												? "bg-slate-800/30 border-slate-700/50"
												: "bg-gray-50 border-gray-200",
										)}
									>
										<p
											className={cn(
												"text-sm mb-2",
												isDark ? "text-gray-400" : "text-gray-600",
											)}
										>
											Error details not available via API
										</p>
										<p
											className={cn(
												"text-xs",
												isDark ? "text-gray-500" : "text-gray-500",
											)}
										>
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
										className="inline-flex items-center gap-2 px-6 py-3 bg-red-600 hover:bg-red-500 rounded-xl text-white! hover:text-white! font-medium transition-all shadow-lg shadow-red-900/30 hover:shadow-red-900/50"
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
				<div className="w-80 shrink-0 border-l border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900 flex flex-col min-h-0">
					{/* Timeline Section */}
					<div className="shrink-0 px-4 py-2 border-b border-gray-200 dark:border-gray-700">
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
						<div className="shrink-0 px-4 py-2 border-b border-gray-200 dark:border-gray-700">
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
																"w-4 h-4 text-gray-400 transition-transform duration-200 shrink-0",
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
										<div className="w-10 h-10 rounded-full bg-red-600/20 border border-red-500/30 flex items-center justify-center shrink-0">
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
										className="flex items-center justify-center gap-2 w-full px-3 py-2 bg-red-600 hover:bg-red-500 rounded-lg text-white! hover:text-white! text-xs font-medium transition-colors"
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
			<Loader2 className="w-4 h-4 text-blue-500 animate-spin shrink-0" />
		);
	}
	if (status === "queued" || status === "pending" || status === "waiting") {
		return (
			<Circle className="w-4 h-4 text-amber-500 animate-pulse shrink-0" />
		);
	}
	if (conclusion === "success") {
		return <CheckCircle className="w-4 h-4 text-emerald-500 shrink-0" />;
	}
	if (conclusion === "failure") {
		return <XCircle className="w-4 h-4 text-red-500 shrink-0" />;
	}
	if (conclusion === "skipped") {
		return <Clock className="w-4 h-4 text-gray-400 shrink-0" />;
	}
	return <Clock className="w-4 h-4 text-amber-500 shrink-0" />;
}

function StepStatusIcon({ conclusion }: { conclusion: string | null }) {
	if (conclusion === "success") {
		return <CheckCircle className="w-3 h-3 text-emerald-500 shrink-0" />;
	}
	if (conclusion === "failure") {
		return <XCircle className="w-3 h-3 text-red-500 shrink-0" />;
	}
	if (conclusion === "skipped") {
		return <Clock className="w-3 h-3 text-gray-400 shrink-0" />;
	}
	return <Clock className="w-3 h-3 text-gray-400 shrink-0" />;
}

// Wrapper component that provides ReactFlow context
export function RunDetail() {
	return (
		<ReactFlowProvider>
			<RunDetailInner />
		</ReactFlowProvider>
	);
}
