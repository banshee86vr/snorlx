import { useState, useEffect } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { FolderGit2, Workflow, Lock, Globe, Search, X, ChevronLeft, ChevronRight, Loader2 } from 'lucide-react';
import { repositoriesApi } from '../services/api';

const PAGE_SIZE_OPTIONS = [10, 20, 50, 100];

export function Repositories() {
  const [page, setPage] = useState(1);
  const [perPage, setPerPage] = useState(20);
  const [search, setSearch] = useState('');
  const [searchQuery, setSearchQuery] = useState('');

  // Debounce search so we query the API after user stops typing (search runs across all pages server-side)
  useEffect(() => {
    const t = setTimeout(() => setSearchQuery(search.trim()), 300);
    return () => clearTimeout(t);
  }, [search]);

  // Reset to first page when search changes (defer to satisfy react-hooks/set-state-in-effect)
  useEffect(() => {
    queueMicrotask(() => setPage(1));
  }, [searchQuery]);

  const { data, isLoading, isFetching } = useQuery({
    queryKey: ['repositories', page, searchQuery, perPage],
    queryFn: () => repositoriesApi.list(page, searchQuery || undefined, perPage),
  });

  const { data: scoresData } = useQuery({
    queryKey: ['repositories', 'scores'],
    queryFn: () => repositoriesApi.listScores(),
  });
  const scoreByRepoId = new Map(
    (scoresData?.data ?? []).map((s) => [s.repo_id, s]),
  );

  const handlePerPageChange = (newPerPage: number) => {
    setPerPage(newPerPage);
    setPage(1);
  };

  const repos = data?.data ?? [];
  const pagination = data?.pagination;
  const total = pagination?.total ?? 0;
  const totalPages = pagination ? Math.max(1, Math.ceil(pagination.total / pagination.page_size)) : 1;
  const currentPage = pagination?.page ?? 1;

  if (isLoading && !data) {
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
            className="w-full pl-10 pr-10 py-2 text-sm rounded-lg border border-gray-300 bg-white focus:border-primary-500 focus:ring-2 focus:ring-primary-500/20 focus:outline-hidden dark:bg-gray-800 dark:border-gray-600 dark:text-gray-100"
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

      <div className="relative">
        {isFetching && (
          <div className="absolute right-0 top-0 z-10 flex items-center gap-1.5 text-sm text-gray-500 dark:text-gray-400">
            <Loader2 className="w-4 h-4 animate-spin" />
            Updating…
          </div>
        )}
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {repos.length > 0 ? (
          repos.map((repo) => (
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

              <div className="mt-4 flex items-center justify-between flex-wrap gap-2">
                <div className="flex items-center gap-2 text-sm text-gray-500 dark:text-gray-400">
                  <Workflow className="w-4 h-4" />
                  <span>{repo.workflow_count || 0} workflows</span>
                </div>
                <div className="flex items-center gap-2">
                  {scoreByRepoId.has(repo.id) && (() => {
                    const score = scoreByRepoId.get(repo.id)!;
                    const tier = score.tier === "none" ? "bronze" : score.tier;
                    const tierClass =
                      tier === "gold"
                        ? "badge-gold"
                        : tier === "silver"
                          ? "badge-silver"
                          : "badge-bronze";
                    return (
                      <>
                        <span
                          className={`badge ${tierClass}`}
                          title="Repository score tier"
                        >
                          {tier}
                        </span>
                        <span className="text-xs text-gray-500 dark:text-gray-400">
                          {Math.round(score.overall_score)}%
                        </span>
                      </>
                    );
                  })()}
                  <span className="text-xs text-gray-400 dark:text-gray-500">
                    {repo.default_branch}
                  </span>
                </div>
              </div>
            </Link>
          ))
        ) : (
          <div className="col-span-full text-center py-12">
            <FolderGit2 className="w-12 h-12 text-gray-300 dark:text-gray-600 mx-auto mb-3" />
            <p className="text-gray-500 dark:text-gray-400">
              {searchQuery ? 'No repositories match your search' : 'No repositories found'}
            </p>
            <p className="text-sm text-gray-400 dark:text-gray-500 mt-1">
              {searchQuery ? 'Try a different search term' : 'Sync your repositories to get started'}
            </p>
          </div>
        )}
        </div>
      </div>

      {total > 0 && (
        <div className="flex flex-col sm:flex-row items-center justify-between gap-4 pt-2 border-t border-gray-200 dark:border-gray-700">
          <div className="flex flex-wrap items-center gap-4">
            <p className="text-sm text-gray-500 dark:text-gray-400">
              Showing {(currentPage - 1) * (pagination?.page_size ?? perPage) + 1}–
              {Math.min(currentPage * (pagination?.page_size ?? perPage), total)} of {total}
              {searchQuery ? ' matching' : ''} repositories
            </p>
            <label className="flex items-center gap-2 text-sm text-gray-500 dark:text-gray-400">
              <span>Per page</span>
              <select
                value={perPage}
                onChange={(e) => handlePerPageChange(Number(e.target.value))}
                className="rounded-lg border border-gray-300 bg-white px-2 py-1.5 text-sm focus:border-primary-500 focus:ring-2 focus:ring-primary-500/20 focus:outline-hidden dark:bg-gray-800 dark:border-gray-600 dark:text-gray-100"
              >
                {PAGE_SIZE_OPTIONS.map((n) => (
                  <option key={n} value={n}>
                    {n}
                  </option>
                ))}
              </select>
            </label>
          </div>
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => setPage((p) => Math.max(1, p - 1))}
              disabled={currentPage <= 1}
              className="inline-flex items-center gap-1 px-3 py-2 text-sm font-medium rounded-lg border border-gray-300 bg-white text-gray-700 hover:bg-gray-50 disabled:opacity-50 disabled:pointer-events-none dark:bg-gray-800 dark:border-gray-600 dark:text-gray-300 dark:hover:bg-gray-700"
            >
              <ChevronLeft className="w-4 h-4" />
              Previous
            </button>
            <span className="text-sm text-gray-500 dark:text-gray-400 px-2">
              Page {currentPage} of {totalPages}
            </span>
            <button
              type="button"
              onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
              disabled={currentPage >= totalPages}
              className="inline-flex items-center gap-1 px-3 py-2 text-sm font-medium rounded-lg border border-gray-300 bg-white text-gray-700 hover:bg-gray-50 disabled:opacity-50 disabled:pointer-events-none dark:bg-gray-800 dark:border-gray-600 dark:text-gray-300 dark:hover:bg-gray-700"
            >
              Next
              <ChevronRight className="w-4 h-4" />
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
      <div className="h-8 bg-gray-200 dark:bg-gray-700 rounded-sm w-48" />
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {[...Array(6)].map((_, i) => (
          <div key={i} className="card p-6 h-40" />
        ))}
      </div>
    </div>
  );
}

