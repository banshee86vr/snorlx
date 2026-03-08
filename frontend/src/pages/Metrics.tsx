import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
	Rocket,
	Clock,
	AlertTriangle,
	RotateCcw,
	Loader2,
	TrendingUp,
} from "lucide-react";
import {
	AreaChart,
	Area,
	XAxis,
	YAxis,
	CartesianGrid,
	Tooltip,
	ResponsiveContainer,
} from "recharts";
import { metricsApi, dashboardApi } from "../services/api";
import { useTheme } from "../context/ThemeContext";
import { cn } from "../lib/utils";

const PERIODS = [
	{ value: "7d" as const, label: "Last 7 days" },
	{ value: "30d" as const, label: "Last 30 days" },
	{ value: "90d" as const, label: "Last 90 days" },
];

function ratingBadgeClass(rating: string, isDark: boolean) {
	const base = "inline-flex items-center px-2 py-0.5 rounded text-xs font-medium capitalize ";
	switch (rating) {
		case "elite":
			return base + (isDark ? "bg-emerald-500/20 text-emerald-400" : "bg-emerald-100 text-emerald-700");
		case "high":
			return base + (isDark ? "bg-green-500/20 text-green-400" : "bg-green-100 text-green-700");
		case "medium":
			return base + (isDark ? "bg-amber-500/20 text-amber-400" : "bg-amber-100 text-amber-700");
		case "low":
			return base + (isDark ? "bg-red-500/20 text-red-400" : "bg-red-100 text-red-700");
		default:
			return base + (isDark ? "bg-gray-500/20 text-gray-400" : "bg-gray-100 text-gray-600");
	}
}

export function Metrics() {
	const { isDark } = useTheme();
	const [period, setPeriod] = useState<"7d" | "30d" | "90d">("30d");

	const { data: metrics, isLoading } = useQuery({
		queryKey: ["metrics", period],
		queryFn: () => metricsApi.getDevOps(period),
	});

	const days = period === "7d" ? 7 : period === "90d" ? 90 : 30;
	const { data: trendsData } = useQuery({
		queryKey: ["dashboard", "trends", days],
		queryFn: () => dashboardApi.getTrends(days),
	});

	const chartColors = {
		area: isDark ? "#10b981" : "#059669",
		grid: isDark ? "#374151" : "#d1d5db",
		axis: isDark ? "#6b7280" : "#9ca3af",
		tooltipBg: isDark ? "#1f2937" : "#ffffff",
		tooltipBorder: isDark ? "#374151" : "#e5e7eb",
		tooltipText: isDark ? "#f3f4f6" : "#111827",
	};

	if (isLoading) {
		return (
			<div className="flex items-center justify-center min-h-[400px]">
				<Loader2 className="w-8 h-8 animate-spin text-primary-500" />
			</div>
		);
	}

	return (
		<div className="space-y-6">
			<div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
				<div>
					<h1 className="text-2xl font-bold text-gray-900 dark:text-gray-100">
						DORA Metrics
					</h1>
					<p className="text-gray-500 dark:text-gray-400 mt-1">
						Deployment frequency, lead time, change failure rate, and MTTR
					</p>
				</div>
				<div className="flex rounded-lg border border-gray-200 dark:border-gray-700 p-1 bg-gray-50 dark:bg-gray-800/50">
					{PERIODS.map((p) => (
						<button
							key={p.value}
							type="button"
							onClick={() => setPeriod(p.value)}
							className={cn(
								"px-3 py-1.5 text-sm font-medium rounded-md transition-colors",
								period === p.value
									? "bg-primary-600 text-white dark:bg-primary-500"
									: "text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-100"
							)}
						>
							{p.label}
						</button>
					))}
				</div>
			</div>

			{metrics && (
				<>
					{/* Four DORA cards */}
					<div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
						<div className="card p-5">
							<div className="flex items-center gap-3 mb-3">
								<div className="w-10 h-10 rounded-lg bg-primary-100 dark:bg-primary-900/30 flex items-center justify-center">
									<Rocket className="w-5 h-5 text-primary-600 dark:text-primary-400" />
								</div>
								<span className="text-sm font-medium text-gray-500 dark:text-gray-400">
									Deployment frequency
								</span>
							</div>
							<p className="text-2xl font-bold text-gray-900 dark:text-gray-100">
								{metrics.deployment_frequency.deployments_per_day.toFixed(2)}
								<span className="text-base font-normal text-gray-500 dark:text-gray-400 ml-1">
									/ day
								</span>
							</p>
							<p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
								{metrics.deployment_frequency.total_deployments} deployments
							</p>
							<span
								className={cn(
									"mt-2 inline-block",
									ratingBadgeClass(metrics.deployment_frequency.rating, isDark)
								)}
							>
								{metrics.deployment_frequency.rating}
							</span>
						</div>

						<div className="card p-5">
							<div className="flex items-center gap-3 mb-3">
								<div className="w-10 h-10 rounded-lg bg-blue-100 dark:bg-blue-900/30 flex items-center justify-center">
									<Clock className="w-5 h-5 text-blue-600 dark:text-blue-400" />
								</div>
								<span className="text-sm font-medium text-gray-500 dark:text-gray-400">
									Lead time for changes
								</span>
							</div>
							<p className="text-2xl font-bold text-gray-900 dark:text-gray-100">
								{formatLeadTime(metrics.lead_time.median_minutes)}
							</p>
							<p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
								Median
							</p>
							<span
								className={cn(
									"mt-2 inline-block",
									ratingBadgeClass(metrics.lead_time.rating, isDark)
								)}
							>
								{metrics.lead_time.rating}
							</span>
						</div>

						<div className="card p-5">
							<div className="flex items-center gap-3 mb-3">
								<div className="w-10 h-10 rounded-lg bg-amber-100 dark:bg-amber-900/30 flex items-center justify-center">
									<AlertTriangle className="w-5 h-5 text-amber-600 dark:text-amber-400" />
								</div>
								<span className="text-sm font-medium text-gray-500 dark:text-gray-400">
									Change failure rate
								</span>
							</div>
							<p className="text-2xl font-bold text-gray-900 dark:text-gray-100">
								{metrics.change_failure_rate.rate.toFixed(1)}%
							</p>
							<p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
								{metrics.change_failure_rate.failed_deployments} failed of{" "}
								{metrics.change_failure_rate.total_deployments}
							</p>
							<span
								className={cn(
									"mt-2 inline-block",
									ratingBadgeClass(metrics.change_failure_rate.rating, isDark)
								)}
							>
								{metrics.change_failure_rate.rating}
							</span>
						</div>

						<div className="card p-5">
							<div className="flex items-center gap-3 mb-3">
								<div className="w-10 h-10 rounded-lg bg-violet-100 dark:bg-violet-900/30 flex items-center justify-center">
									<RotateCcw className="w-5 h-5 text-violet-600 dark:text-violet-400" />
								</div>
								<span className="text-sm font-medium text-gray-500 dark:text-gray-400">
									MTTR
								</span>
							</div>
							<p className="text-2xl font-bold text-gray-900 dark:text-gray-100">
								{formatLeadTime(metrics.mttr.median_minutes)}
							</p>
							<p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
								Median time to recover
							</p>
							<span
								className={cn(
									"mt-2 inline-block",
									ratingBadgeClass(metrics.mttr.rating, isDark)
								)}
							>
								{metrics.mttr.rating}
							</span>
						</div>
					</div>

					{/* Deployment trend */}
					{trendsData?.trends && trendsData.trends.length > 0 && (
						<div className="card p-5">
							<div className="flex items-center gap-2 mb-4">
								<TrendingUp className="w-5 h-5 text-primary-500" />
								<h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100">
									Deployments over time
								</h2>
							</div>
							<div className="h-64">
								<ResponsiveContainer width="100%" height="100%">
									<AreaChart
										data={trendsData.trends.map((t) => ({
											date: t.date,
											deployments: t.deployment_count,
											runs: t.total_runs,
										}))}
										margin={{ top: 10, right: 10, left: 0, bottom: 0 }}
									>
										<CartesianGrid
											strokeDasharray="3 3"
											stroke={chartColors.grid}
											vertical={false}
										/>
										<XAxis
											dataKey="date"
											tick={{ fill: chartColors.axis, fontSize: 12 }}
											tickLine={false}
											axisLine={{ stroke: chartColors.grid }}
										/>
										<YAxis
											tick={{ fill: chartColors.axis, fontSize: 12 }}
											tickLine={false}
											axisLine={false}
											allowDecimals={false}
										/>
										<Tooltip
											contentStyle={{
												backgroundColor: chartColors.tooltipBg,
												border: `1px solid ${chartColors.tooltipBorder}`,
												borderRadius: "8px",
											}}
											labelStyle={{ color: chartColors.tooltipText }}
											formatter={(value) => [value ?? 0, "Deployments"]}
											labelFormatter={(label) => `Date: ${label}`}
										/>
										<Area
											type="monotone"
											dataKey="deployments"
											stroke={chartColors.area}
											fill={chartColors.area}
											fillOpacity={0.3}
											strokeWidth={2}
										/>
									</AreaChart>
								</ResponsiveContainer>
							</div>
						</div>
					)}
				</>
			)}
		</div>
	);
}

function formatLeadTime(minutes: number): string {
	if (minutes < 60) return `${minutes} min`;
	if (minutes < 1440) return `${(minutes / 60).toFixed(1)} h`;
	return `${(minutes / 1440).toFixed(1)} d`;
}
