import { useState, useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { Workflow, CheckCircle, XCircle, Clock, ExternalLink, Search, X } from 'lucide-react';
import { workflowsApi } from '../services/api';
import { cn, formatRelativeTime } from '../lib/utils';
import { useFallbackRefetchInterval } from '../hooks/useFallbackRefetchInterval';

export function Workflows() {
  const [search, setSearch] = useState('');
  // The last-run column changes with live run events; polling only covers a lost WebSocket.
  const fallbackInterval = useFallbackRefetchInterval(30_000);

  const { data: workflows, isLoading } = useQuery({
    queryKey: ['workflows'],
    queryFn: () => workflowsApi.list(),
    refetchInterval: fallbackInterval,
  });

  const filteredWorkflows = useMemo(() => {
    if (!workflows || !search.trim()) return workflows;
    const searchLower = search.toLowerCase();
    return workflows.filter(
      (wf) =>
        wf.name.toLowerCase().includes(searchLower) ||
        wf.path.toLowerCase().includes(searchLower) ||
        wf.repository?.full_name?.toLowerCase().includes(searchLower)
    );
  }, [workflows, search]);

  if (isLoading) {
    return <WorkflowsSkeleton />;
  }

  return (
    <div className="min-w-0 space-y-6">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="min-w-0">
          <h1 className="text-xl font-bold text-gray-900 sm:text-2xl dark:text-gray-100">Workflows</h1>
          <p className="mt-1 text-gray-500 dark:text-gray-400">
            All GitHub Actions workflows across your repositories
          </p>
        </div>
        <div className="relative w-full sm:w-72 sm:shrink-0">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400" />
          <input
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search workflows..."
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

      <div className="card overflow-hidden">
        {filteredWorkflows && filteredWorkflows.length > 0 ? (
          <>
            <div className="divide-y divide-gray-100 md:hidden dark:divide-gray-700">
              {filteredWorkflows.map((workflow) => (
                <div key={workflow.id} className="space-y-2 p-4">
                  <div className="flex items-start justify-between gap-3">
                    <Link to={`/workflows/${workflow.id}`} className="min-w-0 flex-1">
                      <p className="truncate font-medium text-gray-900 hover:text-primary-600 dark:text-gray-100 dark:hover:text-primary-400">
                        {workflow.name}
                      </p>
                      <p className="truncate text-xs text-gray-500 dark:text-gray-400">
                        {workflow.repository?.full_name || 'Unknown'}
                      </p>
                    </Link>
                    <WorkflowStatusBadge state={workflow.state} lastRun={workflow.last_run} />
                  </div>
                  <p className="break-all text-xs text-gray-500 dark:text-gray-400">{workflow.path}</p>
                  <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-gray-500 dark:text-gray-400">
                    <span>
                      {workflow.last_run ? formatRelativeTime(workflow.last_run.started_at) : 'Never run'}
                    </span>
                    <span>
                      {workflow.success_rate !== undefined
                        ? `${workflow.success_rate.toFixed(0)}% success`
                        : 'No success rate'}
                    </span>
                    {workflow.html_url && (
                      <a
                        href={workflow.html_url}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="inline-flex items-center gap-1 text-primary-600 dark:text-primary-400"
                      >
                        GitHub
                        <ExternalLink className="h-3 w-3" />
                      </a>
                    )}
                  </div>
                </div>
              ))}
            </div>
            <div className="hidden md:block table-container rounded-none border-x-0 border-t-0">
              <table className="table">
                <thead>
                  <tr>
                    <th>Workflow</th>
                    <th>Repository</th>
                    <th className="hidden lg:table-cell">Last Run</th>
                    <th className="text-center">Status</th>
                    <th className="hidden xl:table-cell">Success Rate</th>
                    <th></th>
                  </tr>
                </thead>
                <tbody>
                  {filteredWorkflows.map((workflow) => (
                    <tr key={workflow.id}>
                      <td>
                        <Link
                          to={`/workflows/${workflow.id}`}
                          className="flex min-w-0 items-center gap-3"
                        >
                          <div className="shrink-0 rounded-lg bg-primary-100 p-2 dark:bg-primary-900/30">
                            <Workflow className="h-4 w-4 text-primary-600 dark:text-primary-400" />
                          </div>
                          <div className="min-w-0">
                            <p className="font-medium text-gray-900 hover:text-primary-600 dark:text-gray-100 dark:hover:text-primary-400">
                              {workflow.name}
                            </p>
                            <p className="max-w-xs truncate text-xs text-gray-500 xl:max-w-sm dark:text-gray-400">{workflow.path}</p>
                          </div>
                        </Link>
                      </td>
                      <td>
                        <span className="text-gray-600 dark:text-gray-300">
                          {workflow.repository?.full_name || 'Unknown'}
                        </span>
                      </td>
                      <td className="hidden lg:table-cell">
                        {workflow.last_run ? (
                          <span className="text-gray-600 dark:text-gray-300">
                            {formatRelativeTime(workflow.last_run.started_at)}
                          </span>
                        ) : (
                          <span className="text-gray-400">Never</span>
                        )}
                      </td>
                      <td className="text-center">
                        <WorkflowStatusBadge state={workflow.state} lastRun={workflow.last_run} />
                      </td>
                      <td className="hidden xl:table-cell">
                        {workflow.success_rate !== undefined ? (
                          <span className={cn(
                            'font-medium',
                            workflow.success_rate >= 80 ? 'text-green-600 dark:text-green-400' :
                            workflow.success_rate >= 50 ? 'text-amber-600 dark:text-amber-400' :
                            'text-red-600 dark:text-red-400'
                          )}>
                            {workflow.success_rate.toFixed(0)}%
                          </span>
                        ) : (
                          <span className="text-gray-400">-</span>
                        )}
                      </td>
                      <td>
                        {workflow.html_url && (
                          <a
                            href={workflow.html_url}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="inline-flex h-8 w-8 items-center justify-center rounded-lg transition-colors hover:bg-gray-100 dark:hover:bg-gray-700"
                            aria-label="Open workflow on GitHub"
                          >
                            <ExternalLink className="h-4 w-4 text-gray-400" />
                          </a>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </>
        ) : (
          <div className="px-4 py-8 text-center">
            <Workflow className="mx-auto mb-3 h-12 w-12 text-gray-300 dark:text-gray-600" />
            <p className="text-gray-500 dark:text-gray-400">No workflows found</p>
            <p className="mt-1 text-sm text-gray-400 dark:text-gray-500">
              Sync your repositories to see workflows
            </p>
          </div>
        )}
      </div>
    </div>
  );
}

function WorkflowStatusBadge({ state, lastRun }: { state: string; lastRun?: { conclusion: string | null } }) {
  const badgeBase = "w-24 justify-center";
  
  if (state === 'disabled') {
    return <span className={`badge-neutral ${badgeBase}`}>Disabled</span>;
  }

  if (!lastRun) {
    return <span className={`badge-neutral ${badgeBase}`}>No runs</span>;
  }

  if (lastRun.conclusion === 'success') {
    return (
      <span className={`badge-success ${badgeBase} gap-1`}>
        <CheckCircle className="w-3 h-3" />
        Success
      </span>
    );
  }

  if (lastRun.conclusion === 'failure') {
    return (
      <span className={`badge-danger ${badgeBase} gap-1`}>
        <XCircle className="w-3 h-3" />
        Failed
      </span>
    );
  }

  return (
    <span className={`badge-info ${badgeBase} gap-1`}>
      <Clock className="w-3 h-3" />
      Running
    </span>
  );
}

function WorkflowsSkeleton() {
  return (
    <div className="space-y-6 animate-pulse">
      <div className="h-8 bg-gray-200 dark:bg-gray-700 rounded-sm w-48" />
      <div className="card p-6 space-y-4">
        {[...Array(5)].map((_, i) => (
          <div key={i} className="h-16 bg-gray-100 dark:bg-gray-800 rounded-sm" />
        ))}
      </div>
    </div>
  );
}
