import { useQuery } from '@tanstack/react-query';
import {
  GitBranch,
  AlertTriangle,
  CheckCircle,
  Clock,
  Loader2,
  Play,
  XCircle,
  Timer,
} from 'lucide-react';
import {
  AreaChart,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  PieChart,
  Pie,
  Cell,
} from 'recharts';
import { dashboardApi } from '../services/api';
import { cn, formatRelativeTime, formatDuration } from '../lib/utils';
import { Link } from 'react-router-dom';
import { useTheme } from '../context/ThemeContext';

export function Dashboard() {
  const { isDark } = useTheme();

  // Theme-aware chart colors
  const chartColors = {
    success: isDark ? '#10b981' : '#059669',
    failed: isDark ? '#f43f5e' : '#dc2626',
    gridStroke: isDark ? '#374151' : '#d1d5db',
    axisStroke: isDark ? '#6b7280' : '#9ca3af',
    tooltipBg: isDark ? '#1f2937' : '#ffffff',
    tooltipBorder: isDark ? '#374151' : '#e5e7eb',
    tooltipText: isDark ? '#f3f4f6' : '#111827',
    pieSuccess: isDark ? '#10b981' : '#059669',
    pieFailed: isDark ? '#f43f5e' : '#dc2626',
    pieOther: isDark ? '#4b5563' : '#e5e7eb',
  };

  const { data: summary, isLoading: summaryLoading } = useQuery({
    queryKey: ['dashboard', 'summary'],
    queryFn: dashboardApi.getSummary,
    refetchInterval: 30000,
  });

  const { data: trendsData, isLoading: trendsLoading } = useQuery({
    queryKey: ['dashboard', 'trends'],
    queryFn: () => dashboardApi.getTrends(30),
  });

  if (summaryLoading) {
    return <DashboardSkeleton />;
  }

  const runData = summary ? [
    { name: 'Success', value: summary.runs.success, color: chartColors.pieSuccess },
    { name: 'Failed', value: summary.runs.failed, color: chartColors.pieFailed },
    { name: 'Other', value: summary.runs.cancelled + summary.runs.in_progress + summary.runs.queued, color: chartColors.pieOther },
  ] : [];

  // Labels for the time periods
  const currentPeriod = 'Last 30 days';
  const previousPeriod = 'Previous 30 days';

  // Format total duration in hours and minutes
  const formatTotalTime = (seconds: number): string => {
    if (seconds === 0) return '0m';
    const hours = Math.floor(seconds / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);
    if (hours > 0) {
      return `${hours}h ${minutes}m`;
    }
    return `${minutes}m`;
  };

  const totalDuration = summary?.runs.total_duration_seconds || 0;
  const previousTotalDuration = summary?.previous_runs?.total_duration_seconds || 0;

  return (
    <div className="space-y-6">
      {/* Page Header */}
      <div>
        <h1 className="text-2xl font-bold text-gray-900 dark:text-gray-100">Dashboard</h1>
        <p className="text-gray-500 dark:text-gray-400 mt-1">
          Monitor your CI/CD pipelines
        </p>
      </div>

      {/* Stats Grid - 4 boxes */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <StatCard
          title="Workflows"
          value={summary?.workflows.total || 0}
          icon={GitBranch}
          color="primary"
          subtitle={`${summary?.workflows.active || 0} active`}
        />
        <StatCard
          title={`Runs (${currentPeriod})`}
          value={summary?.runs.total || 0}
          icon={Play}
          color="info"
          subtitle={`${summary?.runs.success || 0} successful, ${summary?.runs.failed || 0} failed`}
          previousLabel={previousPeriod}
          previousValue={summary?.previous_runs?.total || 0}
          previousSubtitle={`${summary?.previous_runs?.success || 0} successful, ${summary?.previous_runs?.failed || 0} failed`}
        />
        <StatCard
          title="Pending Runs"
          value={(summary?.runs.in_progress || 0) + (summary?.runs.queued || 0)}
          icon={Clock}
          color="warning"
          subtitle={`${summary?.runs.in_progress || 0} in progress, ${summary?.runs.queued || 0} queued`}
        />
        <StatCard
          title={`Pipeline Time (${currentPeriod})`}
          value={formatTotalTime(totalDuration)}
          icon={Timer}
          color="success"
          subtitle={`${summary?.runs.total || 0} runs completed`}
          previousLabel={previousPeriod}
          previousValue={formatTotalTime(previousTotalDuration)}
          previousSubtitle={`${summary?.previous_runs?.total || 0} runs completed`}
        />
      </div>

      {/* Charts Row */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Trend Chart */}
        <div className="lg:col-span-2 card p-6">
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100">Run Trends</h2>
            {trendsLoading && <Loader2 className="w-4 h-4 animate-spin text-primary-500" />}
          </div>
          <div className="h-72">
            {trendsData?.trends && trendsData.trends.length > 0 ? (
              <ResponsiveContainer width="100%" height="100%">
                <AreaChart data={trendsData.trends}>
                  <defs>
                    <linearGradient id="successGradient" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="5%" stopColor={chartColors.success} stopOpacity={0.3}/>
                      <stop offset="95%" stopColor={chartColors.success} stopOpacity={0}/>
                    </linearGradient>
                    <linearGradient id="failedGradient" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="5%" stopColor={chartColors.failed} stopOpacity={0.3}/>
                      <stop offset="95%" stopColor={chartColors.failed} stopOpacity={0}/>
                    </linearGradient>
                  </defs>
                  <CartesianGrid strokeDasharray="3 3" stroke={chartColors.gridStroke} />
                  <XAxis 
                    dataKey="date" 
                    stroke={chartColors.axisStroke}
                    tickFormatter={(value) => new Date(value).toLocaleDateString('en-US', { month: 'short', day: 'numeric' })}
                  />
                  <YAxis stroke={chartColors.axisStroke} />
                  <Tooltip
                    contentStyle={{
                      backgroundColor: chartColors.tooltipBg,
                      border: `1px solid ${chartColors.tooltipBorder}`,
                      borderRadius: '8px',
                      color: chartColors.tooltipText,
                    }}
                  />
                  <Area 
                    type="monotone" 
                    dataKey="successful_runs" 
                    name="Successful"
                    stroke={chartColors.success} 
                    fill="url(#successGradient)"
                  />
                  <Area 
                    type="monotone" 
                    dataKey="failed_runs" 
                    name="Failed"
                    stroke={chartColors.failed} 
                    fill="url(#failedGradient)"
                  />
                </AreaChart>
              </ResponsiveContainer>
            ) : (
              <EmptyState icon={Clock} message="No trend data available" />
            )}
          </div>
        </div>

        {/* Run Distribution Pie Chart */}
        <div className="card p-6">
          <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100 mb-4">Run Distribution</h2>
          <div className="h-48">
            {summary && summary.runs.total > 0 ? (
              <ResponsiveContainer width="100%" height="100%">
                <PieChart>
                  <Pie
                    data={runData}
                    cx="50%"
                    cy="50%"
                    innerRadius={50}
                    outerRadius={70}
                    paddingAngle={2}
                    dataKey="value"
                  >
                    {runData.map((entry, index) => (
                      <Cell key={`cell-${index}`} fill={entry.color} />
                    ))}
                  </Pie>
                  <Tooltip
                    contentStyle={{
                      backgroundColor: chartColors.tooltipBg,
                      border: `1px solid ${chartColors.tooltipBorder}`,
                      borderRadius: '8px',
                      color: chartColors.tooltipText,
                    }}
                  />
                </PieChart>
              </ResponsiveContainer>
            ) : (
              <EmptyState icon={Play} message="No runs yet" />
            )}
          </div>
          <div className="flex justify-center gap-4 mt-4">
            {runData.map((item) => (
              <div key={item.name} className="flex items-center gap-2">
                <div className="w-3 h-3 rounded-full" style={{ backgroundColor: item.color }} />
                <span className="text-sm text-gray-600 dark:text-gray-400">{item.name}</span>
              </div>
            ))}
          </div>
        </div>
      </div>

      {/* Recent Runs */}
      <div className="card">
        <div className="px-6 py-4 border-b border-gray-200 dark:border-gray-700 flex items-center justify-between">
          <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100">Recent Runs</h2>
          <Link to="/runs" className="text-sm text-primary-600 hover:text-primary-700 dark:text-primary-400">
            View all
          </Link>
        </div>
        <div className="divide-y divide-gray-100 dark:divide-gray-700">
          {summary?.recent_runs && summary.recent_runs.length > 0 ? (
            summary.recent_runs.slice(0, 5).map((run) => (
              <Link
                key={run.id}
                to={`/runs/${run.id}`}
                className="flex items-center justify-between px-6 py-4 hover:bg-gray-50 dark:hover:bg-gray-800/50 transition-colors"
              >
                <div className="flex items-center gap-4">
                  <RunStatusIcon status={run.status} conclusion={run.conclusion} />
                  <div>
                    <p className="font-medium text-gray-900 dark:text-gray-100">{run.name}</p>
                    <p className="text-sm text-gray-500 dark:text-gray-400">
                      {run.repository?.full_name || 'Unknown'} • {run.branch}
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
            <div className="px-6 py-8">
              <EmptyState icon={Play} message="No recent runs" />
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

interface StatCardProps {
  title: string;
  value: number | string;
  icon: React.ElementType;
  color: 'primary' | 'success' | 'warning' | 'info' | 'danger';
  subtitle?: string;
  previousLabel?: string;
  previousValue?: number | string;
  previousSubtitle?: string;
}

function StatCard({ title, value, icon: Icon, color, subtitle, previousLabel, previousValue, previousSubtitle }: StatCardProps) {
  const colorClasses = {
    primary: 'bg-primary-100 text-primary-600 dark:bg-primary-900 dark:text-primary-200',
    success: 'bg-green-100 text-green-600 dark:bg-green-900 dark:text-green-200',
    warning: 'bg-amber-100 text-amber-600 dark:bg-amber-900 dark:text-amber-200',
    info: 'bg-blue-100 text-blue-600 dark:bg-blue-900 dark:text-blue-200',
    danger: 'bg-red-100 text-red-600 dark:bg-red-900 dark:text-red-200',
  };

  return (
    <div className="card p-6">
      <div className="flex items-start justify-between">
        <div className="flex-1">
          <p className="text-sm font-medium text-gray-500 dark:text-gray-400">{title}</p>
          <p className="text-3xl font-bold text-gray-900 dark:text-gray-100 mt-1">{value}</p>
          {subtitle && (
            <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">{subtitle}</p>
          )}
          {previousLabel && previousValue !== undefined && (
            <div className="mt-3 pt-3 border-t border-gray-200 dark:border-gray-700">
              <p className="text-xs font-medium text-gray-400 dark:text-gray-500">{previousLabel}</p>
              <p className="text-lg font-semibold text-gray-600 dark:text-gray-300 mt-0.5">{previousValue}</p>
              {previousSubtitle && (
                <p className="text-xs text-gray-400 dark:text-gray-500 mt-0.5">{previousSubtitle}</p>
              )}
            </div>
          )}
        </div>
        <div className={cn('p-3 rounded-lg', colorClasses[color])}>
          <Icon className="w-6 h-6" />
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
      <AlertTriangle className="w-4 h-4 text-gray-600 dark:text-gray-400" />
    </div>
  );
}

function EmptyState({ icon: Icon, message }: { icon: React.ElementType; message: string }) {
  return (
    <div className="flex flex-col items-center justify-center h-full">
      <Icon className="w-12 h-12 text-gray-300 dark:text-gray-600 mb-3" />
      <p className="text-gray-500 dark:text-gray-400">{message}</p>
    </div>
  );
}

function DashboardSkeleton() {
  return (
    <div className="space-y-6 animate-pulse">
      <div className="h-8 bg-gray-200 dark:bg-gray-700 rounded w-48" />
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        {[...Array(4)].map((_, i) => (
          <div key={i} className="card p-6 h-32" />
        ))}
      </div>
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <div className="lg:col-span-2 card h-96" />
        <div className="card h-96" />
      </div>
    </div>
  );
}
