import { useQuery } from '@tanstack/react-query';
import { useParams, Link } from 'react-router-dom';
import { ArrowLeft, ExternalLink, Workflow, Lock, Globe, Loader2 } from 'lucide-react';
import { repositoriesApi, workflowsApi } from '../services/api';

export function RepositoryDetail() {
  const { id } = useParams<{ id: string }>();

  const { data: repo, isLoading } = useQuery({
    queryKey: ['repositories', id],
    queryFn: () => repositoriesApi.get(Number(id)),
    enabled: !!id,
  });

  const { data: workflows } = useQuery({
    queryKey: ['workflows', { repo_id: id }],
    queryFn: () => workflowsApi.list(Number(id)),
    enabled: !!id,
  });

  if (isLoading) {
    return (
      <div className="flex items-center justify-center min-h-[400px]">
        <Loader2 className="w-8 h-8 animate-spin text-primary-500" />
      </div>
    );
  }

  if (!repo) {
    return (
      <div className="text-center py-12">
        <p className="text-gray-500 dark:text-gray-400">Repository not found</p>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-start justify-between">
        <div className="flex items-center gap-4">
          <Link
            to="/repositories"
            className="p-2 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-800 transition-colors"
          >
            <ArrowLeft className="w-5 h-5 text-gray-500" />
          </Link>
          <div>
            <div className="flex items-center gap-2">
              <h1 className="text-2xl font-bold text-gray-900 dark:text-gray-100">{repo.name}</h1>
              {repo.is_private ? (
                <Lock className="w-4 h-4 text-gray-400" />
              ) : (
                <Globe className="w-4 h-4 text-gray-400" />
              )}
            </div>
            <p className="text-gray-500 dark:text-gray-400">{repo.full_name}</p>
          </div>
        </div>
        <a
          href={repo.html_url}
          target="_blank"
          rel="noopener noreferrer"
          className="btn-secondary flex items-center gap-2"
        >
          <ExternalLink className="w-4 h-4" />
          View on GitHub
        </a>
      </div>

      {/* Description */}
      {repo.description && (
        <div className="card p-4">
          <p className="text-gray-600 dark:text-gray-300">{repo.description}</p>
        </div>
      )}

      {/* Stats */}
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
        <div className="card p-4">
          <p className="text-sm text-gray-500 dark:text-gray-400">Default Branch</p>
          <p className="text-xl font-bold text-gray-900 dark:text-gray-100">{repo.default_branch}</p>
        </div>
        <div className="card p-4">
          <p className="text-sm text-gray-500 dark:text-gray-400">Workflows</p>
          <p className="text-xl font-bold text-gray-900 dark:text-gray-100">{workflows?.length || 0}</p>
        </div>
        <div className="card p-4">
          <p className="text-sm text-gray-500 dark:text-gray-400">Visibility</p>
          <p className="text-xl font-bold text-gray-900 dark:text-gray-100">
            {repo.is_private ? 'Private' : 'Public'}
          </p>
        </div>
      </div>

      {/* Workflows */}
      <div className="card">
        <div className="px-6 py-4 border-b border-gray-200 dark:border-gray-700">
          <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100">Workflows</h2>
        </div>
        <div className="divide-y divide-gray-100 dark:divide-gray-700">
          {workflows && workflows.length > 0 ? (
            workflows.map((workflow) => (
              <Link
                key={workflow.id}
                to={`/workflows/${workflow.id}`}
                className="flex items-center justify-between px-6 py-4 hover:bg-gray-50 dark:hover:bg-gray-800/50 transition-colors"
              >
                <div className="flex items-center gap-3">
                  <div className="p-2 rounded-lg bg-primary-100 dark:bg-primary-900/30">
                    <Workflow className="w-4 h-4 text-primary-600 dark:text-primary-400" />
                  </div>
                  <div>
                    <p className="font-medium text-gray-900 dark:text-gray-100">{workflow.name}</p>
                    <p className="text-sm text-gray-500 dark:text-gray-400">{workflow.path}</p>
                  </div>
                </div>
                <span className={`badge-${workflow.state === 'active' ? 'success' : 'neutral'}`}>
                  {workflow.state}
                </span>
              </Link>
            ))
          ) : (
            <div className="px-6 py-8 text-center">
              <Workflow className="w-12 h-12 text-gray-300 dark:text-gray-600 mx-auto mb-3" />
              <p className="text-gray-500 dark:text-gray-400">No workflows found</p>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

