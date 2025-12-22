import { useQuery } from '@tanstack/react-query';
import { Link, useSearchParams } from 'react-router-dom';
import { FolderGit2, Workflow, Lock, Globe } from 'lucide-react';
import { repositoriesApi } from '../services/api';

export function Repositories() {
  const [searchParams, setSearchParams] = useSearchParams();
  const page = Number(searchParams.get('page')) || 1;

  const { data, isLoading } = useQuery({
    queryKey: ['repositories', page],
    queryFn: () => repositoriesApi.list(page),
  });

  const handlePageChange = (newPage: number) => {
    const params = new URLSearchParams(searchParams);
    params.set('page', String(newPage));
    setSearchParams(params);
  };

  if (isLoading) {
    return <RepositoriesSkeleton />;
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-gray-900 dark:text-gray-100">Repositories</h1>
        <p className="text-gray-500 dark:text-gray-400 mt-1">
          All repositories with GitHub Actions workflows
        </p>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {data?.data && data.data.length > 0 ? (
          data.data.map((repo) => (
            <Link
              key={repo.id}
              to={`/repositories/${repo.id}`}
              className="card p-6 hover:shadow-lg transition-shadow"
            >
              <div className="flex items-start justify-between">
                <div className="flex items-center gap-3">
                  <div className="p-2 rounded-lg bg-primary-100 dark:bg-primary-900/30">
                    <FolderGit2 className="w-5 h-5 text-primary-600 dark:text-primary-400" />
                  </div>
                  <div>
                    <h3 className="font-semibold text-gray-900 dark:text-gray-100 group-hover:text-primary-600">
                      {repo.name}
                    </h3>
                    <p className="text-sm text-gray-500 dark:text-gray-400">{repo.full_name}</p>
                  </div>
                </div>
                {repo.is_private ? (
                  <Lock className="w-4 h-4 text-gray-400" />
                ) : (
                  <Globe className="w-4 h-4 text-gray-400" />
                )}
              </div>

              {repo.description && (
                <p className="mt-3 text-sm text-gray-600 dark:text-gray-300 line-clamp-2">
                  {repo.description}
                </p>
              )}

              <div className="mt-4 flex items-center justify-between">
                <div className="flex items-center gap-2 text-sm text-gray-500 dark:text-gray-400">
                  <Workflow className="w-4 h-4" />
                  <span>{repo.workflow_count || 0} workflows</span>
                </div>
                <span className="text-xs text-gray-400 dark:text-gray-500">
                  {repo.default_branch}
                </span>
              </div>
            </Link>
          ))
        ) : (
          <div className="col-span-full text-center py-12">
            <FolderGit2 className="w-12 h-12 text-gray-300 dark:text-gray-600 mx-auto mb-3" />
            <p className="text-gray-500 dark:text-gray-400">No repositories found</p>
            <p className="text-sm text-gray-400 dark:text-gray-500 mt-1">
              Sync your repositories to get started
            </p>
          </div>
        )}
      </div>

      {/* Pagination */}
      {data?.pagination && data.pagination.total > data.pagination.page_size && (
        <div className="flex items-center justify-between">
          <p className="text-sm text-gray-500 dark:text-gray-400">
            Showing {((page - 1) * data.pagination.page_size) + 1} to{' '}
            {Math.min(page * data.pagination.page_size, data.pagination.total)} of{' '}
            {data.pagination.total} repositories
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
    </div>
  );
}

function RepositoriesSkeleton() {
  return (
    <div className="space-y-6 animate-pulse">
      <div className="h-8 bg-gray-200 dark:bg-gray-700 rounded w-48" />
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {[...Array(6)].map((_, i) => (
          <div key={i} className="card p-6 h-40" />
        ))}
      </div>
    </div>
  );
}

