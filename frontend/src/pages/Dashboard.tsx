import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
	GitBranch,
	AlertTriangle,
	CheckCircle,
	Clock,
	Loader2,
	Play,
	RefreshCw,
	XCircle,
	Timer,
	GitCommit,
	User,
	Zap,
} from "lucide-react";
import {
	AreaChart,
	Area,
	XAxis,
	YAxis,
	CartesianGrid,
	Tooltip,
	ResponsiveContainer,
	PieChart,
	Pie,
	Cell,
} from "recharts";
import { dashboardApi, pipelinesApi } from "../services/api";
import { cn, formatRelativeTime, formatDuration, truncate } from "../lib/utils";
import { useState } from "react";
import { Link } from "react-router-dom";
import { useTheme } from "../context/ThemeContext";
import type { WorkflowRun } from "../types";

export function Dashboard() {
	const { isDark } = useTheme();
	const queryClient = useQueryClient();
	const [pipelinesRefreshingFromGitHub, setPipelinesRefreshingFromGitHub] =
		useState(false);

	// Theme-aware chart colors
	const chartColors = {
		success: isDark ? "#10b981" : "#059669",
		failed: isDark ? "#f43f5e" : "#dc2626",
		gridStroke: isDark ? "#374151" : "#d1d5db",
		axisStroke: isDark ? "#6b7280" : "#9ca3af",
		tooltipBg: isDark ? "#1f2937" : "#ffffff",
		tooltipBorder: isDark ? "#374151" : "#e5e7eb",
		tooltipText: isDark ? "#f3f4f6" : "#111827",
		pieSuccess: isDark ? "#10b981" : "#059669",
		pieFailed: isDark ? "#f43f5e" : "#dc2626",
		pieOther: isDark ? "#4b5563" : "#e5e7eb",
	};

	const { data: summary, isLoading: summaryLoading } = useQuery({
		queryKey: ["dashboard", "summary"],
		queryFn: dashboardApi.getSummary,
		refetchInterval: 30000,
	});

	const { data: trendsData, isLoading: trendsLoading } = useQuery({
		queryKey: ["dashboard", "trends"],
		queryFn: () => dashboardApi.getTrends(30),
	});

	// Fetch active pipelines (in_progress + queued) — 10s poll from storage; manual refresh fetches from GitHub
	const {
		data: activePipelinesData,
		dataUpdatedAt,
		refetch: refetchPipelines,
		isRefetching: pipelinesRefetching,
		isError: pipelinesError,
	} = useQuery({
		queryKey: ["pipelines", "active"],
		queryFn: async () => {
			const res = await pipelinesApi.listActive(false);
			if (Array.isArray(res)) return res;
			const data = (res as { data?: WorkflowRun[] })?.data;
			return Array.isArray(data) ? data : [];
		},
		refetchInterval: 10000,
		refetchOnMount: "always",
	});

	const activePipelines = Array.isArray(activePipelinesData)
		? activePipelinesData
		: [];

	const handleRefreshPipelines = async () => {
		setPipelinesRefreshingFromGitHub(true);
		try {
			const freshData = await pipelinesApi.listActive(true);
			const normalized = Array.isArray(freshData)
				? freshData
				: (freshData as { data?: WorkflowRun[] })?.data ?? [];
			if (Array.isArray(normalized)) {
				queryClient.setQueryData(["pipelines", "active"], normalized);
			}
		} finally {
			setPipelinesRefreshingFromGitHub(false);
		}
	};

	if (summaryLoading) {
		return <DashboardSkeleton />;
	}

	const runData = summary
		? [
				{
					name: "Success",
					value: summary.runs.success,
					color: chartColors.pieSuccess,
				},
				{
					name: "Failed",
					value: summary.runs.failed,
					color: chartColors.pieFailed,
				},
				{
					name: "Other",
					value:
						summary.runs.cancelled +
						summary.runs.in_progress +
						summary.runs.queued,
					color: chartColors.pieOther,
				},
			]
		: [];

	// Get current month name for display
	const currentMonth = new Date().toLocaleString("default", { month: "long" });

	// Format total duration in hours and minutes
	const formatTotalTime = (seconds: number): string => {
		if (seconds === 0) return "0m";
		const hours = Math.floor(seconds / 3600);
		const minutes = Math.floor((seconds % 3600) / 60);
		if (hours > 0) {
			return `${hours}h ${minutes}m`;
		}
		return `${minutes}m`;
	};

	const totalDuration = summary?.runs.total_duration_seconds || 0;

	return (
		<div className="space-y-6">
			{/* Page Header */}
			<div>
				<h1 className="text-2xl font-bold text-gray-900 dark:text-gray-100">
					Dashboard
				</h1>
				<p className="text-gray-500 dark:text-gray-400 mt-1">
					Monitor your CI/CD pipelines
				</p>
			</div>

			{/* Stats Grid - 4 boxes */}
			<div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
				<StatCard
					title="Workflows"
					value={summary?.workflows.total || 0}
					icon={GitBranch}
					color="primary"
					subtitle={`${summary?.workflows.active || 0} active`}
				/>
				<StatCard
					title={`Runs (${currentMonth})`}
					value={summary?.runs.total || 0}
					icon={Play}
					color="info"
					subtitle={`${summary?.runs.success || 0} successful, ${summary?.runs.failed || 0} failed`}
				/>
				<StatCard
					title="Pending Runs"
					value={(summary?.runs.in_progress || 0) + (summary?.runs.queued || 0)}
					icon={Clock}
					color="warning"
					subtitle={`${summary?.runs.in_progress || 0} in progress, ${summary?.runs.queued || 0} queued`}
				/>
				<StatCard
					title={`Pipeline Time (${currentMonth})`}
					value={formatTotalTime(totalDuration)}
					icon={Timer}
					color="success"
					subtitle={`${summary?.runs.total || 0} runs completed`}
				/>
			</div>

			{/* Active Pipelines Grid */}
			<div>
				<div className="flex flex-wrap items-center gap-3 mb-4">
					<Zap className="w-5 h-5 text-primary-500" />
					<h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100">
						Active Pipelines
					</h2>
					{activePipelines.length > 0 && (
						<span className="text-sm text-gray-500 dark:text-gray-400">
							({activePipelines.length} running or pending)
						</span>
					)}
					{dataUpdatedAt > 0 && (
						<span className="text-xs text-gray-400 dark:text-gray-500">
							Updated{" "}
							{formatRelativeTime(new Date(dataUpdatedAt).toISOString())}
						</span>
					)}
					<button
						type="button"
						onClick={handleRefreshPipelines}
						disabled={pipelinesRefetching || pipelinesRefreshingFromGitHub}
						className="ml-auto inline-flex items-center gap-2 rounded-lg px-3 py-1.5 text-sm font-medium text-primary-600 hover:bg-primary-50 dark:text-primary-400 dark:hover:bg-primary-900/30 disabled:opacity-50"
						title="Refresh from GitHub (fetch latest run statuses)"
					>
						<RefreshCw
							className={cn(
								"w-4 h-4",
								(pipelinesRefetching || pipelinesRefreshingFromGitHub) &&
									"animate-spin",
							)}
						/>
						Refresh
					</button>
				</div>
				<div
					className={cn(
						"transition-[filter,opacity] duration-200",
						pipelinesRefreshingFromGitHub &&
							"blur-xs opacity-70 pointer-events-none select-none",
					)}
				>
					{pipelinesError ? (
						<div className="card p-6">
							<div className="flex flex-col items-center justify-center text-center">
								<AlertTriangle className="w-10 h-10 text-amber-500 mb-3" />
								<p className="text-sm text-gray-600 dark:text-gray-400 mb-3">
									Could not load active pipelines.
								</p>
								<button
									type="button"
									onClick={() => refetchPipelines()}
									className="inline-flex items-center gap-2 rounded-lg bg-primary-600 px-4 py-2 text-sm font-medium text-white hover:bg-primary-700 dark:bg-primary-500 dark:hover:bg-primary-600"
								>
									<RefreshCw className="w-4 h-4" />
									Try again
								</button>
							</div>
						</div>
					) : activePipelines.length > 0 ? (
						<div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
							{activePipelines.map((pipeline) => (
								<PipelineCard key={pipeline.id} pipeline={pipeline} />
							))}
						</div>
					) : (
						<div className="card p-8">
							<div className="flex flex-col items-center justify-center text-center">
								<div className="w-16 h-16 rounded-full bg-gray-100 dark:bg-slate-800 flex items-center justify-center mb-4">
									<CheckCircle className="w-8 h-8 text-green-500" />
								</div>
								<h3 className="text-lg font-medium text-gray-900 dark:text-gray-100 mb-2">
									All pipelines complete
								</h3>
								<p className="text-sm text-gray-500 dark:text-gray-400 max-w-sm mb-4">
									No pipelines are currently running or pending. When a workflow
									starts, it will appear here with a live status indicator.
								</p>
								<button
									type="button"
									onClick={() => refetchPipelines()}
									disabled={pipelinesRefetching}
									className="inline-flex items-center gap-2 rounded-lg border border-gray-300 dark:border-gray-600 px-3 py-1.5 text-sm font-medium text-gray-700 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-800 disabled:opacity-50"
								>
									<RefreshCw
										className={cn(
											"w-4 h-4",
											pipelinesRefetching && "animate-spin",
										)}
									/>
									Refresh
								</button>
							</div>
						</div>
					)}
				</div>
			</div>

			{/* Charts Row */}
			<div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
				{/* Trend Chart */}
				<div className="lg:col-span-2 card p-6">
					<div className="flex items-center justify-between mb-4">
						<h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100">
							Run Trends
						</h2>
						{trendsLoading && (
							<Loader2 className="w-4 h-4 animate-spin text-primary-500" />
						)}
					</div>
					<div className="h-72">
						{trendsData?.trends && trendsData.trends.length > 0 ? (
							<ResponsiveContainer width="100%" height="100%">
								<AreaChart data={trendsData.trends}>
									<defs>
										<linearGradient
											id="successGradient"
											x1="0"
											y1="0"
											x2="0"
											y2="1"
										>
											<stop
												offset="5%"
												stopColor={chartColors.success}
												stopOpacity={0.3}
											/>
											<stop
												offset="95%"
												stopColor={chartColors.success}
												stopOpacity={0}
											/>
										</linearGradient>
										<linearGradient
											id="failedGradient"
											x1="0"
											y1="0"
											x2="0"
											y2="1"
										>
											<stop
												offset="5%"
												stopColor={chartColors.failed}
												stopOpacity={0.3}
											/>
											<stop
												offset="95%"
												stopColor={chartColors.failed}
												stopOpacity={0}
											/>
										</linearGradient>
									</defs>
									<CartesianGrid
										strokeDasharray="3 3"
										stroke={chartColors.gridStroke}
									/>
									<XAxis
										dataKey="date"
										stroke={chartColors.axisStroke}
										tickFormatter={(value) =>
											new Date(value).toLocaleDateString("en-US", {
												month: "short",
												day: "numeric",
											})
										}
									/>
									<YAxis stroke={chartColors.axisStroke} />
									<Tooltip
										contentStyle={{
											backgroundColor: chartColors.tooltipBg,
											border: `1px solid ${chartColors.tooltipBorder}`,
											borderRadius: "8px",
											color: chartColors.tooltipText,
										}}
									/>
									<Area
										type="monotone"
										dataKey="successful_runs"
										name="Successful"
										stroke={chartColors.success}
										fill="url(#successGradient)"
									/>
									<Area
										type="monotone"
										dataKey="failed_runs"
										name="Failed"
										stroke={chartColors.failed}
										fill="url(#failedGradient)"
									/>
								</AreaChart>
							</ResponsiveContainer>
						) : (
							<EmptyState icon={Clock} message="No trend data available" />
						)}
					</div>
				</div>

				{/* Run Distribution Pie Chart */}
				<div className="card p-6">
					<h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100 mb-4">
						Run Distribution
					</h2>
					<div className="h-48">
						{summary && summary.runs.total > 0 ? (
							<ResponsiveContainer width="100%" height="100%">
								<PieChart>
									<Pie
										data={runData}
										cx="50%"
										cy="50%"
										innerRadius={50}
										outerRadius={70}
										paddingAngle={2}
										dataKey="value"
									>
										{runData.map((entry, index) => (
											<Cell key={`cell-${index}`} fill={entry.color} />
										))}
									</Pie>
									<Tooltip
										contentStyle={{
											backgroundColor: chartColors.tooltipBg,
											border: `1px solid ${chartColors.tooltipBorder}`,
											borderRadius: "8px",
											color: chartColors.tooltipText,
										}}
									/>
								</PieChart>
							</ResponsiveContainer>
						) : (
							<EmptyState icon={Play} message="No runs yet" />
						)}
					</div>
					<div className="flex justify-center gap-4 mt-4">
						{runData.map((item) => (
							<div key={item.name} className="flex items-center gap-2">
								<div
									className="w-3 h-3 rounded-full"
									style={{ backgroundColor: item.color }}
								/>
								<span className="text-sm text-gray-600 dark:text-gray-400">
									{item.name}
								</span>
							</div>
						))}
					</div>
				</div>
			</div>

			{/* Recent Runs */}
			<div className="card">
				<div className="px-6 py-4 border-b border-gray-200 dark:border-gray-700 flex items-center justify-between">
					<h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100">
						Recent Runs
					</h2>
					<Link
						to="/runs"
						className="text-sm text-primary-600 hover:text-primary-700 dark:text-primary-400"
					>
						View all
					</Link>
				</div>
				<div className="divide-y divide-gray-100 dark:divide-gray-700">
					{summary?.recent_runs && summary.recent_runs.length > 0 ? (
						summary.recent_runs.slice(0, 5).map((run) => (
							<Link
								key={run.id}
								to={`/runs/${run.id}`}
								className="flex items-center justify-between px-6 py-4 hover:bg-gray-50 dark:hover:bg-gray-800/50 transition-colors"
							>
								<div className="flex items-center gap-4">
									<RunStatusIcon
										status={run.status}
										conclusion={run.conclusion}
									/>
									<div>
										<p className="font-medium text-gray-900 dark:text-gray-100">
											{run.name}
										</p>
										<p className="text-sm text-gray-500 dark:text-gray-400">
											{run.repository?.full_name || "Unknown"} • {run.branch}
										</p>
									</div>
								</div>
								<div className="text-right">
									<p className="text-sm text-gray-900 dark:text-gray-100">
										{formatDuration(run.duration_seconds)}
									</p>
									<p className="text-xs text-gray-500 dark:text-gray-400">
										{formatRelativeTime(run.started_at)}
									</p>
								</div>
							</Link>
						))
					) : (
						<div className="px-6 py-8">
							<EmptyState icon={Play} message="No recent runs" />
						</div>
					)}
				</div>
			</div>
		</div>
	);
}

interface StatCardProps {
	title: string;
	value: number | string;
	icon: React.ElementType;
	color: "primary" | "success" | "warning" | "info" | "danger";
	subtitle?: string;
}

function StatCard({
	title,
	value,
	icon: Icon,
	color,
	subtitle,
}: StatCardProps) {
	const colorClasses = {
		primary:
			"bg-primary-100 text-primary-600 dark:bg-primary-900 dark:text-primary-200",
		success:
			"bg-green-100 text-green-600 dark:bg-green-900 dark:text-green-200",
		warning:
			"bg-amber-100 text-amber-600 dark:bg-amber-900 dark:text-amber-200",
		info: "bg-blue-100 text-blue-600 dark:bg-blue-900 dark:text-blue-200",
		danger: "bg-red-100 text-red-600 dark:bg-red-900 dark:text-red-200",
	};

	return (
		<div className="card p-6">
			<div className="flex items-start justify-between">
				<div className="flex-1">
					<p className="text-sm font-medium text-gray-500 dark:text-gray-400">
						{title}
					</p>
					<p className="text-3xl font-bold text-gray-900 dark:text-gray-100 mt-1">
						{value}
					</p>
					{subtitle && (
						<p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
							{subtitle}
						</p>
					)}
				</div>
				<div className={cn("p-3 rounded-lg", colorClasses[color])}>
					<Icon className="w-6 h-6" />
				</div>
			</div>
		</div>
	);
}

function RunStatusIcon({
	status,
	conclusion,
}: {
	status: string;
	conclusion: string | null;
}) {
	if (status === "in_progress" || status === "queued") {
		return (
			<div className="w-8 h-8 rounded-full bg-blue-100 dark:bg-blue-900 flex items-center justify-center">
				<Loader2 className="w-4 h-4 text-blue-600 dark:text-blue-400 animate-spin" />
			</div>
		);
	}

	if (conclusion === "success") {
		return (
			<div className="w-8 h-8 rounded-full bg-green-100 dark:bg-green-900 flex items-center justify-center">
				<CheckCircle className="w-4 h-4 text-green-600 dark:text-green-400" />
			</div>
		);
	}

	if (conclusion === "failure") {
		return (
			<div className="w-8 h-8 rounded-full bg-red-100 dark:bg-red-900 flex items-center justify-center">
				<XCircle className="w-4 h-4 text-red-600 dark:text-red-400" />
			</div>
		);
	}

	return (
		<div className="w-8 h-8 rounded-full bg-gray-100 dark:bg-gray-800 flex items-center justify-center">
			<AlertTriangle className="w-4 h-4 text-gray-600 dark:text-gray-400" />
		</div>
	);
}

function PipelineCard({ pipeline }: { pipeline: WorkflowRun }) {
	const isRunning = pipeline.status === "in_progress";
	const isPending = pipeline.status === "queued";

	return (
		<Link
			to={`/runs/${pipeline.id}`}
			className="group relative block overflow-hidden rounded-lg bg-white dark:bg-slate-900/50 border border-gray-200 dark:border-secondary-500/30 shadow-xs hover:shadow-md dark:hover:shadow-lg dark:hover:shadow-primary-500/10 transition-all duration-300"
		>
			{/* Card Content */}
			<div className="p-4 pb-5">
				{/* Header: Status + Workflow Name */}
				<div className="flex items-start gap-3 mb-3">
					<div
						className={cn(
							"shrink-0 w-8 h-8 rounded-full flex items-center justify-center",
							isRunning && "bg-blue-100 dark:bg-blue-900/50",
							isPending && "bg-amber-100 dark:bg-amber-900/50",
						)}
					>
						{isRunning ? (
							<Loader2 className="w-4 h-4 text-blue-600 dark:text-blue-400 animate-spin" />
						) : (
							<Clock className="w-4 h-4 text-amber-600 dark:text-amber-400" />
						)}
					</div>
					<div className="flex-1 min-w-0">
						<h3 className="font-semibold text-gray-900 dark:text-gray-100 truncate group-hover:text-primary-600 dark:group-hover:text-primary-400 transition-colors">
							{truncate(pipeline.name, 28)}
						</h3>
						<p className="text-xs text-gray-500 dark:text-gray-400 truncate">
							{pipeline.repository?.full_name || "Unknown repository"}
						</p>
					</div>
				</div>

				{/* Info Grid */}
				<div className="space-y-2 text-sm">
					{/* Branch */}
					<div className="flex items-center gap-2 text-gray-600 dark:text-gray-400">
						<GitBranch className="w-3.5 h-3.5 shrink-0" />
						<span className="truncate">{pipeline.branch}</span>
					</div>

					{/* Commit */}
					<div className="flex items-center gap-2 text-gray-600 dark:text-gray-400">
						<GitCommit className="w-3.5 h-3.5 shrink-0" />
						<span className="font-mono text-xs">
							{pipeline.commit_sha.slice(0, 7)}
						</span>
						{pipeline.commit_message && (
							<span className="truncate text-xs opacity-75">
								{truncate(pipeline.commit_message, 20)}
							</span>
						)}
					</div>

					{/* Actor + Time */}
					<div className="flex items-center justify-between">
						<div className="flex items-center gap-2 text-gray-600 dark:text-gray-400">
							{pipeline.actor_avatar ? (
								<img
									src={pipeline.actor_avatar}
									alt={pipeline.actor_login}
									className="w-4 h-4 rounded-full"
								/>
							) : (
								<User className="w-3.5 h-3.5" />
							)}
							<span className="text-xs">{pipeline.actor_login}</span>
						</div>
						<span className="text-xs text-gray-500 dark:text-gray-500">
							{formatRelativeTime(pipeline.started_at)}
						</span>
					</div>
				</div>

				{/* Status Badge */}
				<div className="mt-3">
					<span
						className={cn(
							"inline-flex items-center gap-1.5 px-2 py-1 rounded-full text-xs font-medium",
							isRunning &&
								"bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300 dark:border dark:border-blue-500/50",
							isPending &&
								"bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300 dark:border dark:border-amber-500/50",
						)}
					>
						{isRunning ? (
							<>
								<span className="relative flex h-2 w-2">
									<span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-blue-400 opacity-75"></span>
									<span className="relative inline-flex rounded-full h-2 w-2 bg-blue-500"></span>
								</span>
								Running
							</>
						) : (
							<>
								<Clock className="w-3 h-3" />
								Pending
							</>
						)}
					</span>
				</div>
			</div>

			{/* Animated Striped Status Bar at Bottom */}
			<div className="absolute bottom-0 left-0 right-0 h-1.5 overflow-hidden rounded-b-lg">
				<div
					className={cn(
						"absolute -left-8 -right-8 top-0 bottom-0",
						isRunning && "pipeline-bar-running",
						isPending && "pipeline-bar-pending",
					)}
				/>
			</div>
		</Link>
	);
}

function EmptyState({
	icon: Icon,
	message,
}: {
	icon: React.ElementType;
	message: string;
}) {
	return (
		<div className="flex flex-col items-center justify-center h-full">
			<Icon className="w-12 h-12 text-gray-300 dark:text-gray-600 mb-3" />
			<p className="text-gray-500 dark:text-gray-400">{message}</p>
		</div>
	);
}

function DashboardSkeleton() {
	return (
		<div className="space-y-6 animate-pulse">
			<div className="h-8 bg-gray-200 dark:bg-gray-700 rounded-sm w-48" />
			<div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
				{[...Array(4)].map((_, i) => (
					<div key={i} className="card p-6 h-32" />
				))}
			</div>
			<div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
				<div className="lg:col-span-2 card h-96" />
				<div className="card h-96" />
			</div>
		</div>
	);
}
