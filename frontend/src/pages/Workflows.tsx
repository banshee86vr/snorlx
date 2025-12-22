import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { Workflow, CheckCircle, XCircle, Clock, ExternalLink } from 'lucide-react';
import { workflowsApi } from '../services/api';
import { cn, formatRelativeTime } from '../lib/utils';

export function Workflows() {
  const { data: workflows, isLoading } = useQuery({
    queryKey: ['workflows'],
    queryFn: () => workflowsApi.list(),
  });

  if (isLoading) {
    return <WorkflowsSkeleton />;
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900 dark:text-gray-100">Workflows</h1>
          <p className="text-gray-500 dark:text-gray-400 mt-1">
            All GitHub Actions workflows across your repositories
          </p>
        </div>
      </div>

      <div className="card">
        <div className="table-container">
          <table className="table">
            <thead>
              <tr>
                <th>Workflow</th>
                <th>Repository</th>
                <th>Last Run</th>
                <th>Status</th>
                <th>Success Rate</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {workflows && workflows.length > 0 ? (
                workflows.map((workflow) => (
                  <tr key={workflow.id}>
                    <td>
                      <Link
                        to={`/workflows/${workflow.id}`}
                        className="flex items-center gap-3"
                      >
                        <div className="p-2 rounded-lg bg-primary-100 dark:bg-primary-900/30">
                          <Workflow className="w-4 h-4 text-primary-600 dark:text-primary-400" />
                        </div>
                        <div>
                          <p className="font-medium text-gray-900 dark:text-gray-100 hover:text-primary-600 dark:hover:text-primary-400">
                            {workflow.name}
                          </p>
                          <p className="text-xs text-gray-500 dark:text-gray-400">{workflow.path}</p>
                        </div>
                      </Link>
                    </td>
                    <td>
                      <span className="text-gray-600 dark:text-gray-300">
                        {workflow.repository?.full_name || 'Unknown'}
                      </span>
                    </td>
                    <td>
                      {workflow.last_run ? (
                        <span className="text-gray-600 dark:text-gray-300">
                          {formatRelativeTime(workflow.last_run.started_at)}
                        </span>
                      ) : (
                        <span className="text-gray-400">Never</span>
                      )}
                    </td>
                    <td>
                      <WorkflowStatusBadge state={workflow.state} lastRun={workflow.last_run} />
                    </td>
                    <td>
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
                          className="p-2 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-800 transition-colors"
                        >
                          <ExternalLink className="w-4 h-4 text-gray-400" />
                        </a>
                      )}
                    </td>
                  </tr>
                ))
              ) : (
                <tr>
                  <td colSpan={6} className="text-center py-8">
                    <Workflow className="w-12 h-12 text-gray-300 dark:text-gray-600 mx-auto mb-3" />
                    <p className="text-gray-500 dark:text-gray-400">No workflows found</p>
                    <p className="text-sm text-gray-400 dark:text-gray-500 mt-1">
                      Sync your repositories to see workflows
                    </p>
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}

function WorkflowStatusBadge({ state, lastRun }: { state: string; lastRun?: { conclusion: string | null } }) {
  if (state === 'disabled') {
    return <span className="badge-neutral">Disabled</span>;
  }

  if (!lastRun) {
    return <span className="badge-neutral">No runs</span>;
  }

  if (lastRun.conclusion === 'success') {
    return (
      <span className="badge-success flex items-center gap-1">
        <CheckCircle className="w-3 h-3" />
        Success
      </span>
    );
  }

  if (lastRun.conclusion === 'failure') {
    return (
      <span className="badge-danger flex items-center gap-1">
        <XCircle className="w-3 h-3" />
        Failed
      </span>
    );
  }

  return (
    <span className="badge-info flex items-center gap-1">
      <Clock className="w-3 h-3" />
      Running
    </span>
  );
}

function WorkflowsSkeleton() {
  return (
    <div className="space-y-6 animate-pulse">
      <div className="h-8 bg-gray-200 dark:bg-gray-700 rounded w-48" />
      <div className="card p-6 space-y-4">
        {[...Array(5)].map((_, i) => (
          <div key={i} className="h-16 bg-gray-100 dark:bg-gray-800 rounded" />
        ))}
      </div>
    </div>
  );
}

