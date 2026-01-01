import { useState, useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { FolderGit2, Workflow, Lock, Globe, Search, X } from 'lucide-react';
import { repositoriesApi } from '../services/api';

export function Repositories() {
  const [search, setSearch] = useState('');

  // Fetch all repositories (first page with large limit via server-side search if needed)
  const { data, isLoading } = useQuery({
    queryKey: ['repositories'],
    queryFn: () => repositoriesApi.list(1),
  });

  const filteredRepositories = useMemo(() => {
    if (!data?.data || !search.trim()) return data?.data;
    const searchLower = search.toLowerCase();
    return data.data.filter(
      (repo) =>
        repo.name.toLowerCase().includes(searchLower) ||
        repo.full_name.toLowerCase().includes(searchLower)
    );
  }, [data?.data, search]);

  if (isLoading) {
    return <RepositoriesSkeleton />;
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold text-gray-900 dark:text-gray-100">Repositories</h1>
          <p className="text-gray-500 dark:text-gray-400 mt-1">
            All repositories with GitHub Actions workflows
          </p>
        </div>
        <div className="relative w-full sm:w-72">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-400" />
          <input
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search repositories..."
            className="w-full pl-10 pr-10 py-2 text-sm rounded-lg border border-gray-300 bg-white focus:border-primary-500 focus:ring-2 focus:ring-primary-500/20 focus:outline-none dark:bg-gray-800 dark:border-gray-600 dark:text-gray-100"
          />
          {search && (
            <button
              type="button"
              onClick={() => setSearch('')}
              className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
            >
              <X className="w-4 h-4" />
            </button>
          )}
        </div>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {filteredRepositories && filteredRepositories.length > 0 ? (
          filteredRepositories.map((repo) => (
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

