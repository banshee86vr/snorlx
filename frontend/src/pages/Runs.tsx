import { useState, useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { Play, CheckCircle, XCircle, Clock, Loader2, ExternalLink, Filter, Search, X } from 'lucide-react';
import { runsApi } from '../services/api';
import { cn, formatRelativeTime, formatDuration, getStatusColor } from '../lib/utils';
import { useFallbackRefetchInterval } from '../hooks/useFallbackRefetchInterval';
import type { RunFilters } from '../types';

export function Runs() {
  const [search, setSearch] = useState('');
  const [filters, setFilters] = useState<RunFilters>({
    status: '',
    conclusion: '',
    branch: '',
  });
  // Live events invalidate this list; polling only covers a lost WebSocket.
  const fallbackInterval = useFallbackRefetchInterval(30_000);

  const { data, isLoading } = useQuery({
    queryKey: ['runs', filters],
    queryFn: () => runsApi.list(filters),
    refetchInterval: fallbackInterval,
  });

  const handleFilterChange = (key: keyof RunFilters, value: string) => {
    setFilters({ ...filters, [key]: value });
  };

  // Filter runs by search query (client-side) - only by run name or repository name
  const runsData = data?.data;
  const filteredRuns = useMemo(() => {
    if (!runsData || !search.trim()) return runsData;
    const searchLower = search.toLowerCase();
    return runsData.filter(
      (run) =>
        run.name.toLowerCase().includes(searchLower) ||
        run.repository?.full_name?.toLowerCase().includes(searchLower) ||
        run.repository?.name?.toLowerCase().includes(searchLower)
    );
  }, [runsData, search]);

  return (
    <div className="min-w-0 space-y-6">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="min-w-0">
          <h1 className="text-xl font-bold text-gray-900 sm:text-2xl dark:text-gray-100">Workflow Runs</h1>
          <p className="mt-1 text-gray-500 dark:text-gray-400">
            All workflow runs across your repositories
          </p>
        </div>
        <div className="relative w-full sm:w-72 sm:shrink-0">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400" />
          <input
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search runs..."
            className="w-full rounded-lg border border-gray-300 bg-white py-2 pl-10 pr-10 text-sm focus:border-primary-500 focus:outline-hidden focus:ring-2 focus:ring-primary-500/20 dark:border-gray-600 dark:bg-gray-800 dark:text-gray-100"
          />
          {search && (
            <button
              type="button"
              onClick={() => setSearch('')}
              className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
              aria-label="Clear search"
            >
              <X className="h-4 w-4" />
            </button>
          )}
        </div>
      </div>

      <div className="card p-4">
        <div className="flex flex-col gap-3 sm:flex-row sm:flex-wrap sm:items-center">
          <div className="flex items-center gap-2">
            <Filter className="h-4 w-4 text-gray-500" />
            <span className="text-sm font-medium text-gray-700 dark:text-gray-300">Filters</span>
          </div>
          <select
            value={filters.status}
            onChange={(e) => handleFilterChange('status', e.target.value)}
            className="input w-full sm:w-40"
            aria-label="Status"
          >
            <option value="">All Status</option>
            <option value="completed">Completed</option>
            <option value="in_progress">In Progress</option>
            <option value="queued">Queued</option>
          </select>
          <select
            value={filters.conclusion}
            onChange={(e) => handleFilterChange('conclusion', e.target.value)}
            className="input w-full sm:w-40"
            aria-label="Result"
          >
            <option value="">All Results</option>
            <option value="success">Success</option>
            <option value="failure">Failure</option>
            <option value="cancelled">Cancelled</option>
          </select>
          <input
            type="text"
            placeholder="Branch..."
            value={filters.branch}
            onChange={(e) => handleFilterChange('branch', e.target.value)}
            className="input w-full sm:w-40"
            aria-label="Branch"
          />
        </div>
      </div>

      <div className="card overflow-hidden">
        {isLoading ? (
          <div className="flex justify-center p-8">
            <Loader2 className="h-8 w-8 animate-spin text-primary-500" />
          </div>
        ) : filteredRuns && filteredRuns.length > 0 ? (
          <>
            <div className="divide-y divide-gray-100 md:hidden dark:divide-gray-700">
              {filteredRuns.map((run) => (
                <div key={run.id} className="space-y-2 p-4">
                  <div className="flex items-start gap-3">
                    <RunStatusIcon status={run.status} conclusion={run.conclusion} />
                    <div className="min-w-0 flex-1">
                      <Link
                        to={`/runs/${run.id}`}
                        className="block truncate font-medium text-gray-900 hover:text-primary-600 dark:text-gray-100 dark:hover:text-primary-400"
                      >
                        {run.name}
                      </Link>
                      <p className="truncate text-xs text-gray-500 dark:text-gray-400">
                        {run.repository?.full_name} #{run.run_number}
                      </p>
                    </div>
                    <a
                      href={run.html_url}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-lg hover:bg-gray-100 dark:hover:bg-gray-800"
                      aria-label="Open run on GitHub"
                    >
                      <ExternalLink className="h-4 w-4 text-gray-400" />
                    </a>
                  </div>
                  <div className="flex flex-wrap items-center gap-x-3 gap-y-1 pl-8 text-xs text-gray-500 dark:text-gray-400">
                    <span className={cn(getStatusColor(run.conclusion || run.status))}>
                      {run.conclusion || run.status}
                    </span>
                    <span className="truncate">{run.branch}</span>
                    <span className="badge-neutral">{run.event}</span>
                    <span>{formatDuration(run.duration_seconds)}</span>
                    <span>{formatRelativeTime(run.started_at)}</span>
                  </div>
                </div>
              ))}
            </div>
            <div className="hidden md:block table-container rounded-none border-x-0 border-t-0">
              <table className="table">
                <thead>
                  <tr>
                    <th>Run</th>
                    <th>Status</th>
                    <th>Branch</th>
                    <th className="hidden lg:table-cell">Event</th>
                    <th className="hidden xl:table-cell">Duration</th>
                    <th>Started</th>
                    <th></th>
                  </tr>
                </thead>
                <tbody>
                  {filteredRuns.map((run) => (
                    <tr key={run.id}>
                      <td>
                        <Link
                          to={`/runs/${run.id}`}
                          className="flex min-w-0 items-center gap-3"
                        >
                          <RunStatusIcon status={run.status} conclusion={run.conclusion} />
                          <div className="min-w-0">
                            <p className="font-medium text-gray-900 hover:text-primary-600 dark:text-gray-100 dark:hover:text-primary-400">
                              {run.name}
                            </p>
                            <p className="truncate text-xs text-gray-500 dark:text-gray-400">
                              {run.repository?.full_name} #{run.run_number}
                            </p>
                          </div>
                        </Link>
                      </td>
                      <td>
                        <span className={cn(getStatusColor(run.conclusion || run.status))}>
                          {run.conclusion || run.status}
                        </span>
                      </td>
                      <td>
                        <span className="text-gray-600 dark:text-gray-300">{run.branch}</span>
                      </td>
                      <td className="hidden lg:table-cell">
                        <span className="badge-neutral">{run.event}</span>
                      </td>
                      <td className="hidden xl:table-cell">
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
                        <a
                          href={run.html_url}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="inline-flex rounded-lg p-2 transition-colors hover:bg-gray-100 dark:hover:bg-gray-800"
                          aria-label="Open run on GitHub"
                        >
                          <ExternalLink className="h-4 w-4 text-gray-400" />
                        </a>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </>
        ) : (
          <div className="px-4 py-8 text-center">
            <Play className="mx-auto mb-3 h-12 w-12 text-gray-300 dark:text-gray-600" />
            <p className="text-gray-500 dark:text-gray-400">No runs found</p>
          </div>
        )}
      </div>
    </div>
  );
}

function RunStatusIcon({ status, conclusion }: { status: string; conclusion: string | null }) {
  if (status === 'in_progress' || status === 'queued') {
    return <Loader2 className="h-5 w-5 shrink-0 animate-spin text-blue-500" />;
  }
  if (conclusion === 'success') {
    return <CheckCircle className="h-5 w-5 shrink-0 text-green-500" />;
  }
  if (conclusion === 'failure') {
    return <XCircle className="h-5 w-5 shrink-0 text-red-500" />;
  }
  return <Clock className="h-5 w-5 shrink-0 text-gray-400" />;
}
