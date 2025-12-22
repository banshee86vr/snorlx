import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link, useSearchParams } from 'react-router-dom';
import { Play, CheckCircle, XCircle, Clock, Loader2, ExternalLink, Filter } from 'lucide-react';
import { runsApi } from '../services/api';
import { cn, formatRelativeTime, formatDuration, getStatusColor } from '../lib/utils';
import type { RunFilters } from '../types';

export function Runs() {
  const [searchParams, setSearchParams] = useSearchParams();
  const [filters, setFilters] = useState<RunFilters>({
    status: searchParams.get('status') || '',
    conclusion: searchParams.get('conclusion') || '',
    branch: searchParams.get('branch') || '',
  });
  const page = Number(searchParams.get('page')) || 1;

  const { data, isLoading } = useQuery({
    queryKey: ['runs', filters, page],
    queryFn: () => runsApi.list(filters, page),
  });

  const handleFilterChange = (key: keyof RunFilters, value: string) => {
    const newFilters = { ...filters, [key]: value };
    setFilters(newFilters);
    const params = new URLSearchParams();
    Object.entries(newFilters).forEach(([k, v]) => {
      if (v) params.set(k, String(v));
    });
    params.set('page', '1');
    setSearchParams(params);
  };

  const handlePageChange = (newPage: number) => {
    const params = new URLSearchParams(searchParams);
    params.set('page', String(newPage));
    setSearchParams(params);
  };

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-gray-900 dark:text-gray-100">Workflow Runs</h1>
        <p className="text-gray-500 dark:text-gray-400 mt-1">
          All workflow runs across your repositories
        </p>
      </div>

      {/* Filters */}
      <div className="card p-4">
        <div className="flex items-center gap-4 flex-wrap">
          <div className="flex items-center gap-2">
            <Filter className="w-4 h-4 text-gray-500" />
            <span className="text-sm font-medium text-gray-700 dark:text-gray-300">Filters:</span>
          </div>
          <select
            value={filters.status}
            onChange={(e) => handleFilterChange('status', e.target.value)}
            className="input w-40"
          >
            <option value="">All Status</option>
            <option value="completed">Completed</option>
            <option value="in_progress">In Progress</option>
            <option value="queued">Queued</option>
          </select>
          <select
            value={filters.conclusion}
            onChange={(e) => handleFilterChange('conclusion', e.target.value)}
            className="input w-40"
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
            className="input w-40"
          />
        </div>
      </div>

      {/* Runs Table */}
      <div className="card">
        {isLoading ? (
          <div className="p-8 flex justify-center">
            <Loader2 className="w-8 h-8 animate-spin text-primary-500" />
          </div>
        ) : (
          <>
            <div className="table-container">
              <table className="table">
                <thead>
                  <tr>
                    <th>Run</th>
                    <th>Status</th>
                    <th>Branch</th>
                    <th>Event</th>
                    <th>Duration</th>
                    <th>Started</th>
                    <th></th>
                  </tr>
                </thead>
                <tbody>
                  {data?.data && data.data.length > 0 ? (
                    data.data.map((run) => (
                      <tr key={run.id}>
                        <td>
                          <Link
                            to={`/runs/${run.id}`}
                            className="flex items-center gap-3"
                          >
                            <RunStatusIcon status={run.status} conclusion={run.conclusion} />
                            <div>
                              <p className="font-medium text-gray-900 dark:text-gray-100 hover:text-primary-600 dark:hover:text-primary-400">
                                {run.name}
                              </p>
                              <p className="text-xs text-gray-500 dark:text-gray-400">
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
                        <td>
                          <span className="badge-neutral">{run.event}</span>
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
                          <a
                            href={run.html_url}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="p-2 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-800 transition-colors"
                          >
                            <ExternalLink className="w-4 h-4 text-gray-400" />
                          </a>
                        </td>
                      </tr>
                    ))
                  ) : (
                    <tr>
                      <td colSpan={7} className="text-center py-8">
                        <Play className="w-12 h-12 text-gray-300 dark:text-gray-600 mx-auto mb-3" />
                        <p className="text-gray-500 dark:text-gray-400">No runs found</p>
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>

            {/* Pagination */}
            {data?.pagination && data.pagination.total > data.pagination.page_size && (
              <div className="px-6 py-4 border-t border-gray-200 dark:border-gray-700 flex items-center justify-between">
                <p className="text-sm text-gray-500 dark:text-gray-400">
                  Showing {((page - 1) * data.pagination.page_size) + 1} to{' '}
                  {Math.min(page * data.pagination.page_size, data.pagination.total)} of{' '}
                  {data.pagination.total} runs
                </p>
                <div className="flex gap-2">
                  <button
                    onClick={() => handlePageChange(page - 1)}
                    disabled={page === 1}
                    className="btn-secondary disabled:opacity-50"
                  >
                    Previous
                  </button>
                  <button
                    onClick={() => handlePageChange(page + 1)}
                    disabled={page * data.pagination.page_size >= data.pagination.total}
                    className="btn-secondary disabled:opacity-50"
                  >
                    Next
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

function RunStatusIcon({ status, conclusion }: { status: string; conclusion: string | null }) {
  if (status === 'in_progress' || status === 'queued') {
    return <Loader2 className="w-5 h-5 text-blue-500 animate-spin" />;
  }
  if (conclusion === 'success') {
    return <CheckCircle className="w-5 h-5 text-green-500" />;
  }
  if (conclusion === 'failure') {
    return <XCircle className="w-5 h-5 text-red-500" />;
  }
  return <Clock className="w-5 h-5 text-gray-400" />;
}

