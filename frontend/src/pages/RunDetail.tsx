import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useParams, Link } from 'react-router-dom';
import {
  ArrowLeft,
  ExternalLink,
  CheckCircle,
  XCircle,
  Clock,
  Loader2,
  RefreshCw,
  StopCircle,
  GitBranch,
  GitCommit,
  User,
} from 'lucide-react';
import { runsApi } from '../services/api';
import { cn, formatDuration, formatDateTime, getStatusColor } from '../lib/utils';

export function RunDetail() {
  const { id } = useParams<{ id: string }>();
  const queryClient = useQueryClient();

  const { data: run, isLoading } = useQuery({
    queryKey: ['runs', id],
    queryFn: () => runsApi.get(Number(id)),
    enabled: !!id,
    refetchInterval: (query) => {
      const data = query.state.data;
      return data?.status === 'in_progress' || data?.status === 'queued' ? 5000 : false;
    },
  });

  const { data: jobs } = useQuery({
    queryKey: ['runs', id, 'jobs'],
    queryFn: () => runsApi.getJobs(Number(id)),
    enabled: !!id,
  });

  const rerunMutation = useMutation({
    mutationFn: () => runsApi.rerun(Number(id)),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['runs', id] });
    },
  });

  const cancelMutation = useMutation({
    mutationFn: () => runsApi.cancel(Number(id)),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['runs', id] });
    },
  });

  if (isLoading) {
    return (
      <div className="flex items-center justify-center min-h-[400px]">
        <Loader2 className="w-8 h-8 animate-spin text-primary-500" />
      </div>
    );
  }

  if (!run) {
    return (
      <div className="text-center py-12">
        <p className="text-gray-500 dark:text-gray-400">Run not found</p>
      </div>
    );
  }

  const isRunning = run.status === 'in_progress' || run.status === 'queued';

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-start justify-between">
        <div className="flex items-center gap-4">
          <Link
            to="/runs"
            className="p-2 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-800 transition-colors"
          >
            <ArrowLeft className="w-5 h-5 text-gray-500" />
          </Link>
          <div>
            <div className="flex items-center gap-3">
              <RunStatusIcon status={run.status} conclusion={run.conclusion} size="lg" />
              <h1 className="text-2xl font-bold text-gray-900 dark:text-gray-100">{run.name}</h1>
            </div>
            <p className="text-gray-500 dark:text-gray-400 mt-1">
              {run.repository?.full_name} • Run #{run.run_number}
            </p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          {isRunning && (
            <button
              onClick={() => cancelMutation.mutate()}
              disabled={cancelMutation.isPending}
              className="btn-danger flex items-center gap-2"
            >
              <StopCircle className="w-4 h-4" />
              Cancel
            </button>
          )}
          <button
            onClick={() => rerunMutation.mutate()}
            disabled={rerunMutation.isPending || isRunning}
            className="btn-secondary flex items-center gap-2"
          >
            <RefreshCw className={cn('w-4 h-4', rerunMutation.isPending && 'animate-spin')} />
            Re-run
          </button>
          <a
            href={run.html_url}
            target="_blank"
            rel="noopener noreferrer"
            className="btn-secondary flex items-center gap-2"
          >
            <ExternalLink className="w-4 h-4" />
            View on GitHub
          </a>
        </div>
      </div>

      {/* Info Cards */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
        <div className="card p-4">
          <div className="flex items-center gap-3">
            <GitBranch className="w-5 h-5 text-gray-400" />
            <div>
              <p className="text-sm text-gray-500 dark:text-gray-400">Branch</p>
              <p className="font-medium text-gray-900 dark:text-gray-100">{run.branch}</p>
            </div>
          </div>
        </div>
        <div className="card p-4">
          <div className="flex items-center gap-3">
            <GitCommit className="w-5 h-5 text-gray-400" />
            <div>
              <p className="text-sm text-gray-500 dark:text-gray-400">Commit</p>
              <p className="font-medium text-gray-900 dark:text-gray-100 font-mono text-sm">
                {run.commit_sha.substring(0, 7)}
              </p>
            </div>
          </div>
        </div>
        <div className="card p-4">
          <div className="flex items-center gap-3">
            <User className="w-5 h-5 text-gray-400" />
            <div>
              <p className="text-sm text-gray-500 dark:text-gray-400">Triggered by</p>
              <p className="font-medium text-gray-900 dark:text-gray-100">{run.actor_login}</p>
            </div>
          </div>
        </div>
        <div className="card p-4">
          <div className="flex items-center gap-3">
            <Clock className="w-5 h-5 text-gray-400" />
            <div>
              <p className="text-sm text-gray-500 dark:text-gray-400">Duration</p>
              <p className="font-medium text-gray-900 dark:text-gray-100">
                {formatDuration(run.duration_seconds)}
              </p>
            </div>
          </div>
        </div>
      </div>

      {/* Timeline */}
      <div className="card p-6">
        <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100 mb-4">Timeline</h2>
        <div className="space-y-2 text-sm">
          <div className="flex items-center gap-3">
            <span className="text-gray-500 dark:text-gray-400 w-24">Started</span>
            <span className="text-gray-900 dark:text-gray-100">{formatDateTime(run.started_at)}</span>
          </div>
          {run.completed_at && (
            <div className="flex items-center gap-3">
              <span className="text-gray-500 dark:text-gray-400 w-24">Completed</span>
              <span className="text-gray-900 dark:text-gray-100">{formatDateTime(run.completed_at)}</span>
            </div>
          )}
        </div>
      </div>

      {/* Jobs */}
      <div className="card">
        <div className="px-6 py-4 border-b border-gray-200 dark:border-gray-700">
          <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100">Jobs</h2>
        </div>
        <div className="divide-y divide-gray-100 dark:divide-gray-700">
          {jobs && jobs.length > 0 ? (
            jobs.map((job) => (
              <div key={job.id} className="px-6 py-4">
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-3">
                    <JobStatusIcon status={job.status} conclusion={job.conclusion} />
                    <div>
                      <p className="font-medium text-gray-900 dark:text-gray-100">{job.name}</p>
                      {job.runner_name && (
                        <p className="text-sm text-gray-500 dark:text-gray-400">
                          Runner: {job.runner_name}
                        </p>
                      )}
                    </div>
                  </div>
                  <div className="text-right">
                    <span className={cn(getStatusColor(job.conclusion || job.status))}>
                      {job.conclusion || job.status}
                    </span>
                    <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
                      {formatDuration(job.duration_seconds)}
                    </p>
                  </div>
                </div>

                {/* Steps */}
                {job.steps && job.steps.length > 0 && (
                  <div className="mt-4 ml-8 space-y-2">
                    {(job.steps as Array<{ name: string; conclusion: string | null; number: number }>).map((step) => (
                      <div key={step.number} className="flex items-center gap-2 text-sm">
                        <StepStatusIcon conclusion={step.conclusion} />
                        <span className="text-gray-600 dark:text-gray-300">{step.name}</span>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            ))
          ) : (
            <div className="px-6 py-8 text-center">
              <Clock className="w-12 h-12 text-gray-300 dark:text-gray-600 mx-auto mb-3" />
              <p className="text-gray-500 dark:text-gray-400">No jobs found</p>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

function RunStatusIcon({ status, conclusion, size = 'md' }: { status: string; conclusion: string | null; size?: 'md' | 'lg' }) {
  const sizeClass = size === 'lg' ? 'w-10 h-10' : 'w-8 h-8';
  const iconSize = size === 'lg' ? 'w-5 h-5' : 'w-4 h-4';

  if (status === 'in_progress' || status === 'queued') {
    return (
      <div className={cn(sizeClass, 'rounded-full bg-blue-100 dark:bg-blue-900 flex items-center justify-center')}>
        <Loader2 className={cn(iconSize, 'text-blue-600 dark:text-blue-400 animate-spin')} />
      </div>
    );
  }

  if (conclusion === 'success') {
    return (
      <div className={cn(sizeClass, 'rounded-full bg-green-100 dark:bg-green-900 flex items-center justify-center')}>
        <CheckCircle className={cn(iconSize, 'text-green-600 dark:text-green-400')} />
      </div>
    );
  }

  if (conclusion === 'failure') {
    return (
      <div className={cn(sizeClass, 'rounded-full bg-red-100 dark:bg-red-900 flex items-center justify-center')}>
        <XCircle className={cn(iconSize, 'text-red-600 dark:text-red-400')} />
      </div>
    );
  }

  return (
    <div className={cn(sizeClass, 'rounded-full bg-gray-100 dark:bg-gray-800 flex items-center justify-center')}>
      <Clock className={cn(iconSize, 'text-gray-600 dark:text-gray-400')} />
    </div>
  );
}

function JobStatusIcon({ status, conclusion }: { status: string; conclusion: string | null }) {
  if (status === 'in_progress') {
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

function StepStatusIcon({ conclusion }: { conclusion: string | null }) {
  if (conclusion === 'success') {
    return <CheckCircle className="w-4 h-4 text-green-500" />;
  }
  if (conclusion === 'failure') {
    return <XCircle className="w-4 h-4 text-red-500" />;
  }
  if (conclusion === 'skipped') {
    return <Clock className="w-4 h-4 text-gray-400" />;
  }
  return <Clock className="w-4 h-4 text-gray-400" />;
}

