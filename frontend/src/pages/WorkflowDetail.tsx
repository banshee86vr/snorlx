import { useQuery } from '@tanstack/react-query';
import { useParams, Link } from 'react-router-dom';
import { ArrowLeft, ExternalLink, CheckCircle, XCircle, Clock, Loader2 } from 'lucide-react';
import { workflowsApi } from '../services/api';
import { formatRelativeTime, formatDuration } from '../lib/utils';

export function WorkflowDetail() {
  const { id } = useParams<{ id: string }>();

  const { data: workflow, isLoading } = useQuery({
    queryKey: ['workflows', id],
    queryFn: () => workflowsApi.get(Number(id)),
    enabled: !!id,
  });

  const { data: runsData } = useQuery({
    queryKey: ['workflows', id, 'runs'],
    queryFn: () => workflowsApi.getRuns(Number(id)),
    enabled: !!id,
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
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center gap-4">
        <Link
          to="/workflows"
          className="p-2 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-800 transition-colors"
        >
          <ArrowLeft className="w-5 h-5 text-gray-500" />
        </Link>
        <div className="flex-1">
          <h1 className="text-2xl font-bold text-gray-900 dark:text-gray-100">{workflow.name}</h1>
          <p className="text-gray-500 dark:text-gray-400">{workflow.path}</p>
        </div>
        {workflow.html_url && (
          <a
            href={workflow.html_url}
            target="_blank"
            rel="noopener noreferrer"
            className="btn-secondary flex items-center gap-2"
          >
            <ExternalLink className="w-4 h-4" />
            View on GitHub
          </a>
        )}
      </div>

      {/* Stats */}
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
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

      {/* Recent Runs */}
      <div className="card">
        <div className="px-6 py-4 border-b border-gray-200 dark:border-gray-700">
          <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100">Recent Runs</h2>
        </div>
        <div className="divide-y divide-gray-100 dark:divide-gray-700">
          {runsData?.data && runsData.data.length > 0 ? (
            runsData.data.map((run) => (
              <Link
                key={run.id}
                to={`/runs/${run.id}`}
                className="flex items-center justify-between px-6 py-4 hover:bg-gray-50 dark:hover:bg-gray-800/50 transition-colors"
              >
                <div className="flex items-center gap-4">
                  <RunStatusIcon status={run.status} conclusion={run.conclusion} />
                  <div>
                    <p className="font-medium text-gray-900 dark:text-gray-100">
                      #{run.run_number}
                    </p>
                    <p className="text-sm text-gray-500 dark:text-gray-400">
                      {run.branch} • {run.event}
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
            <div className="px-6 py-8 text-center">
              <Clock className="w-12 h-12 text-gray-300 dark:text-gray-600 mx-auto mb-3" />
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
      <div className="w-8 h-8 rounded-full bg-blue-100 dark:bg-blue-900 flex items-center justify-center">
        <Loader2 className="w-4 h-4 text-blue-600 dark:text-blue-400 animate-spin" />
      </div>
    );
  }

  if (conclusion === 'success') {
    return (
      <div className="w-8 h-8 rounded-full bg-green-100 dark:bg-green-900 flex items-center justify-center">
        <CheckCircle className="w-4 h-4 text-green-600 dark:text-green-400" />
      </div>
    );
  }

  if (conclusion === 'failure') {
    return (
      <div className="w-8 h-8 rounded-full bg-red-100 dark:bg-red-900 flex items-center justify-center">
        <XCircle className="w-4 h-4 text-red-600 dark:text-red-400" />
      </div>
    );
  }

  return (
    <div className="w-8 h-8 rounded-full bg-gray-100 dark:bg-gray-800 flex items-center justify-center">
      <Clock className="w-4 h-4 text-gray-600 dark:text-gray-400" />
    </div>
  );
}

