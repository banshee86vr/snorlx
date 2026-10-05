import { useQuery } from '@tanstack/react-query';
import { useParams, Link } from 'react-router-dom';
import { ArrowLeft, ExternalLink, CheckCircle, XCircle, Clock, Loader2 } from 'lucide-react';
import { workflowsApi } from '../services/api';
import { formatRelativeTime, formatDuration } from '../lib/utils';
import { useFallbackRefetchInterval } from '../hooks/useFallbackRefetchInterval';

export function WorkflowDetail() {
  const { id } = useParams<{ id: string }>();
  const workflowId = id ? Number(id) : 0;
  // Live run events invalidate these queries; polling only covers a lost WebSocket.
  const fallbackInterval = useFallbackRefetchInterval(30_000);

  const { data: workflow, isLoading } = useQuery({
    queryKey: ['workflows', id],
    queryFn: () => workflowsApi.get(workflowId),
    enabled: !!id,
    refetchInterval: fallbackInterval,
  });

  const { data: runsData } = useQuery({
    queryKey: ['workflows', id, 'runs'],
    queryFn: () => workflowsApi.getRuns(Number(id)),
    enabled: !!id,
    refetchInterval: fallbackInterval,
  });

  if (isLoading) {
    return (
      <div className="flex items-center justify-center min-h-[400px]">
        <Loader2 className="w-8 h-8 animate-spin text-primary-500" />
      </div>
    );
  }

  if (!workflow) {
    return (
      <div className="text-center py-12">
        <p className="text-gray-500 dark:text-gray-400">Workflow not found</p>
      </div>
    );
  }

  return (
    <div className="min-w-0 space-y-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start">
        <div className="flex min-w-0 flex-1 items-start gap-3 sm:gap-4">
          <Link
            to="/workflows"
            className="shrink-0 rounded-lg p-2 transition-colors hover:bg-gray-100 dark:hover:bg-gray-800"
            aria-label="Back to workflows"
          >
            <ArrowLeft className="h-5 w-5 text-gray-500" />
          </Link>
          <div className="min-w-0">
            <h1 className="break-words text-xl font-bold text-gray-900 sm:text-2xl dark:text-gray-100">{workflow.name}</h1>
            <p className="break-all text-gray-500 dark:text-gray-400">{workflow.path}</p>
          </div>
        </div>
        {workflow.html_url && (
          <a
            href={workflow.html_url}
            target="_blank"
            rel="noopener noreferrer"
            className="btn-secondary inline-flex w-fit shrink-0 items-center gap-2"
          >
            <ExternalLink className="h-4 w-4" />
            View on GitHub
          </a>
        )}
      </div>

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <div className="card p-4">
          <p className="text-sm text-gray-500 dark:text-gray-400">Total Runs</p>
          <p className="text-2xl font-bold text-gray-900 dark:text-gray-100">
            {workflow.total_runs || 0}
          </p>
        </div>
        <div className="card p-4">
          <p className="text-sm text-gray-500 dark:text-gray-400">Success Rate</p>
          <p className="text-2xl font-bold text-gray-900 dark:text-gray-100">
            {workflow.success_rate?.toFixed(1) || 0}%
          </p>
        </div>
        <div className="card p-4">
          <p className="text-sm text-gray-500 dark:text-gray-400">Avg Duration</p>
          <p className="text-2xl font-bold text-gray-900 dark:text-gray-100">
            {formatDuration(workflow.avg_duration_seconds)}
          </p>
        </div>
      </div>

      <div className="card">
        <div className="border-b border-gray-200 px-4 py-4 sm:px-6 dark:border-gray-700">
          <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100">Recent Runs</h2>
        </div>
        <div className="divide-y divide-gray-100 dark:divide-gray-700">
          {runsData?.data && runsData.data.length > 0 ? (
            runsData.data.map((run) => (
              <Link
                key={run.id}
                to={`/runs/${run.id}`}
                className="flex flex-col gap-2 px-4 py-4 transition-colors hover:bg-gray-50 sm:flex-row sm:items-center sm:justify-between sm:px-6 dark:hover:bg-gray-800/50"
              >
                <div className="flex min-w-0 items-center gap-3 sm:gap-4">
                  <RunStatusIcon status={run.status} conclusion={run.conclusion} />
                  <div className="min-w-0">
                    <p className="font-medium text-gray-900 dark:text-gray-100">
                      #{run.run_number}
                    </p>
                    <p className="truncate text-sm text-gray-500 dark:text-gray-400">
                      {run.branch} • {run.event}
                    </p>
                  </div>
                </div>
                <div className="shrink-0 pl-11 text-left sm:pl-0 sm:text-right">
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
            <div className="px-6 py-8 text-center">
              <Clock className="mx-auto mb-3 h-12 w-12 text-gray-300 dark:text-gray-600" />
              <p className="text-gray-500 dark:text-gray-400">No runs yet</p>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

function RunStatusIcon({ status, conclusion }: { status: string; conclusion: string | null }) {
  if (status === 'in_progress' || status === 'queued') {
    return (
      <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-blue-100 dark:bg-blue-900">
        <Loader2 className="h-4 w-4 animate-spin text-blue-600 dark:text-blue-400" />
      </div>
    );
  }

  if (conclusion === 'success') {
    return (
      <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-green-100 dark:bg-green-900">
        <CheckCircle className="h-4 w-4 text-green-600 dark:text-green-400" />
      </div>
    );
  }

  if (conclusion === 'failure') {
    return (
      <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-red-100 dark:bg-red-900">
        <XCircle className="h-4 w-4 text-red-600 dark:text-red-400" />
      </div>
    );
  }

  return (
    <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-gray-100 dark:bg-gray-800">
      <Clock className="h-4 w-4 text-gray-600 dark:text-gray-400" />
    </div>
  );
}
