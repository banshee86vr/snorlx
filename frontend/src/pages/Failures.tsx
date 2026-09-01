import { useEffect, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useSearchParams } from "react-router-dom";
import {
	XCircle,
	ExternalLink,
	Search,
	X,
	Loader2,
	RefreshCw,
	ChevronLeft,
	ChevronRight,
} from "lucide-react";
import { pipelinesApi, type FailedPipelineView } from "../services/api";
import { cn, formatRelativeTime, formatDuration, getStatusColor } from "../lib/utils";

const RECENT_PAGE_SIZE = 50;

export function Failures() {
	const queryClient = useQueryClient();
	const [searchParams, setSearchParams] = useSearchParams();
	const view: FailedPipelineView =
		searchParams.get("view") === "recent" ? "recent" : "current";
	const page = Math.max(1, Number.parseInt(searchParams.get("page") || "1", 10) || 1);

	const [search, setSearch] = useState("");
	const [debouncedSearch, setDebouncedSearch] = useState("");
	const [refreshingFromGitHub, setRefreshingFromGitHub] = useState(false);
	const skipSearchPageReset = useRef(true);

	useEffect(() => {
		const timer = window.setTimeout(() => {
			setDebouncedSearch(search.trim());
		}, 300);
		return () => window.clearTimeout(timer);
	}, [search]);

	useEffect(() => {
		if (skipSearchPageReset.current) {
			skipSearchPageReset.current = false;
			return;
		}
		setSearchParams(
			(prev) => {
				if (!prev.has("page")) {
					return prev;
				}
				const next = new URLSearchParams(prev);
				next.delete("page");
				return next;
			},
			{ replace: true },
		);
	}, [debouncedSearch, setSearchParams]);

	const { data, isLoading, isError, refetch, isRefetching } = useQuery({
		queryKey: ["pipelines", "failed", view, debouncedSearch, view === "recent" ? page : 1],
		queryFn: () =>
			pipelinesApi.listFailed({
				view,
				q: debouncedSearch || undefined,
				page: view === "recent" ? page : undefined,
			}),
		refetchInterval: 15000,
	});

	const runs = data?.data ?? [];
	const total = data?.pagination.total ?? 0;
	const pageSize = data?.pagination.page_size || RECENT_PAGE_SIZE;
	const totalPages = view === "recent" ? Math.max(1, Math.ceil(total / pageSize)) : 1;

	const setView = (next: FailedPipelineView) => {
		setSearchParams((prev) => {
			const params = new URLSearchParams(prev);
			if (next === "current") {
				params.delete("view");
			} else {
				params.set("view", next);
			}
			params.delete("page");
			return params;
		});
	};

	const setPage = (nextPage: number) => {
		setSearchParams((prev) => {
			const params = new URLSearchParams(prev);
			if (nextPage <= 1) {
				params.delete("page");
			} else {
				params.set("page", String(nextPage));
			}
			return params;
		});
	};

	const handleRefresh = async () => {
		setRefreshingFromGitHub(true);
		try {
			const fresh = await pipelinesApi.listFailed({
				view,
				refresh: true,
				q: debouncedSearch || undefined,
				page: view === "recent" ? page : undefined,
			});
			queryClient.setQueryData(
				["pipelines", "failed", view, debouncedSearch, view === "recent" ? page : 1],
				fresh,
			);
		} finally {
			setRefreshingFromGitHub(false);
		}
	};

	return (
		<div className="space-y-6">
			<div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
				<div>
					<h1 className="text-2xl font-bold text-gray-900 dark:text-gray-100">
						Failures
					</h1>
					<p className="text-gray-500 dark:text-gray-400 mt-1">
						Failed workflows across synced repositories
					</p>
				</div>
				<div className="flex items-center gap-3 w-full sm:w-auto">
					<div className="relative flex-1 sm:w-72">
						<Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-400" />
						<input
							type="text"
							value={search}
							onChange={(e) => setSearch(e.target.value)}
							placeholder="Search repo or workflow..."
							className="w-full pl-10 pr-10 py-2 text-sm rounded-lg border border-gray-300 bg-white focus:border-primary-500 focus:ring-2 focus:ring-primary-500/20 focus:outline-hidden dark:bg-gray-800 dark:border-gray-600 dark:text-gray-100"
						/>
						{search && (
							<button
								type="button"
								onClick={() => setSearch("")}
								className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
							>
								<X className="w-4 h-4" />
							</button>
						)}
					</div>
					<button
						type="button"
						onClick={handleRefresh}
						disabled={isRefetching || refreshingFromGitHub}
						className="inline-flex items-center gap-2 rounded-lg px-3 py-2 text-sm font-medium text-primary-600 hover:bg-primary-50 dark:text-primary-400 dark:hover:bg-primary-900/30 disabled:opacity-50"
						title="Refresh from GitHub"
					>
						<RefreshCw
							className={cn(
								"w-4 h-4",
								(isRefetching || refreshingFromGitHub) && "animate-spin",
							)}
						/>
						Refresh
					</button>
				</div>
			</div>

			<div className="flex gap-2">
				<TabButton
					active={view === "current"}
					onClick={() => setView("current")}
					label="Currently broken"
				/>
				<TabButton
					active={view === "recent"}
					onClick={() => setView("recent")}
					label="Recent (7 days)"
				/>
			</div>

			<div className="card">
				{isLoading ? (
					<div className="p-8 flex justify-center">
						<Loader2 className="w-8 h-8 animate-spin text-primary-500" />
					</div>
				) : isError ? (
					<div className="p-8 flex flex-col items-center text-center">
						<XCircle className="w-12 h-12 text-red-400 mb-3" />
						<p className="text-gray-500 dark:text-gray-400 mb-3">
							Could not load failed pipelines.
						</p>
						<button
							type="button"
							onClick={() => refetch()}
							className="inline-flex items-center gap-2 rounded-lg bg-primary-600 px-4 py-2 text-sm font-medium text-white hover:bg-primary-700"
						>
							<RefreshCw className="w-4 h-4" />
							Try again
						</button>
					</div>
				) : (
					<>
						<div
							className={cn(
								"transition-[filter,opacity] duration-200",
								refreshingFromGitHub &&
									"blur-xs opacity-70 pointer-events-none select-none",
							)}
						>
							<div className="table-container">
								<table className="table">
									<thead>
										<tr>
											<th>Workflow</th>
											<th>Repository</th>
											<th>Branch</th>
											<th>Actor</th>
											<th>Duration</th>
											<th>Started</th>
											<th></th>
										</tr>
									</thead>
									<tbody>
										{runs.length > 0 ? (
											runs.map((run) => (
												<tr key={run.id}>
													<td>
														<Link
															to={`/runs/${run.id}`}
															className="flex items-center gap-3"
														>
															<XCircle className="w-5 h-5 text-red-500 shrink-0" />
															<div>
																<p className="font-medium text-gray-900 dark:text-gray-100 hover:text-primary-600 dark:hover:text-primary-400">
																	{run.workflow?.name || run.name}
																</p>
																<p className="text-xs text-gray-500 dark:text-gray-400">
																	{run.name} #{run.run_number}
																</p>
															</div>
														</Link>
													</td>
													<td>
														<span className="text-gray-600 dark:text-gray-300">
															{run.repository?.full_name || "-"}
														</span>
													</td>
													<td>
														<span className="text-gray-600 dark:text-gray-300">
															{run.branch}
														</span>
													</td>
													<td>
														<span className="text-gray-600 dark:text-gray-300">
															{run.actor_login}
														</span>
													</td>
													<td>
														<span className="text-gray-600 dark:text-gray-300">
															{formatDuration(run.duration_seconds)}
														</span>
													</td>
													<td>
														<span className="text-gray-500 dark:text-gray-400">
															{formatRelativeTime(run.started_at)}
														</span>
													</td>
													<td>
														<div className="flex items-center gap-2">
															<span
																className={cn(
																	getStatusColor(run.conclusion || run.status),
																)}
															>
																{run.conclusion || run.status}
															</span>
															<a
																href={run.html_url}
																target="_blank"
																rel="noopener noreferrer"
																className="p-2 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-800 transition-colors"
															>
																<ExternalLink className="w-4 h-4 text-gray-400" />
															</a>
														</div>
													</td>
												</tr>
											))
										) : (
											<tr>
												<td colSpan={7} className="text-center py-8">
													<XCircle className="w-12 h-12 text-gray-300 dark:text-gray-600 mx-auto mb-3" />
													<p className="text-gray-500 dark:text-gray-400">
														{view === "current"
															? "No currently failing workflows in synced repositories."
															: "No failed runs in the last 7 days in synced repositories."}
													</p>
													<p className="text-xs text-gray-400 dark:text-gray-500 mt-2 max-w-md mx-auto">
														Unsynced repos are not listed. Refresh only covers
														the first 50 synced repos, and history is limited
														to what the last sync stored.
													</p>
												</td>
											</tr>
										)}
									</tbody>
								</table>
							</div>
						</div>

						{view === "recent" && total > 0 && (
							<div className="flex items-center justify-between px-4 py-3 border-t border-gray-200 dark:border-gray-700">
								<p className="text-sm text-gray-500 dark:text-gray-400">
									Showing {(page - 1) * pageSize + 1}-
									{Math.min(page * pageSize, total)} of {total}
								</p>
								<div className="flex items-center gap-2">
									<button
										type="button"
										onClick={() => setPage(page - 1)}
										disabled={page <= 1}
										className="inline-flex items-center gap-1 rounded-lg px-3 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-100 disabled:opacity-40 dark:text-gray-300 dark:hover:bg-gray-800"
									>
										<ChevronLeft className="w-4 h-4" />
										Prev
									</button>
									<button
										type="button"
										onClick={() => setPage(page + 1)}
										disabled={page >= totalPages}
										className="inline-flex items-center gap-1 rounded-lg px-3 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-100 disabled:opacity-40 dark:text-gray-300 dark:hover:bg-gray-800"
									>
										Next
										<ChevronRight className="w-4 h-4" />
									</button>
								</div>
							</div>
						)}
					</>
				)}
			</div>
		</div>
	);
}

function TabButton({
	active,
	onClick,
	label,
}: {
	active: boolean;
	onClick: () => void;
	label: string;
}) {
	return (
		<button
			type="button"
			onClick={onClick}
			className={cn(
				"px-4 py-2 text-sm font-medium rounded-lg transition-colors",
				active
					? "bg-primary-600 text-white dark:bg-primary-500"
					: "bg-gray-100 text-gray-700 hover:bg-gray-200 dark:bg-slate-800 dark:text-slate-300 dark:hover:bg-slate-700",
			)}
		>
			{label}
		</button>
	);
}
