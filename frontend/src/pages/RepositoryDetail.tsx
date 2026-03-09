import React, { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useParams, Link } from 'react-router-dom';
import { ArrowLeft, ExternalLink, Workflow, Lock, Globe, Loader2, ChevronDown, ChevronRight, RefreshCw } from 'lucide-react';
import {
  Radar,
  RadarChart,
  PolarGrid,
  PolarAngleAxis,
  PolarRadiusAxis,
  ResponsiveContainer,
  Tooltip,
} from 'recharts';
import { repositoriesApi, workflowsApi } from '../services/api';
import { useTheme } from '../context/ThemeContext';
import { cn } from '../lib/utils';

const SCORE_CATEGORIES = [
  { key: 'security_score', label: 'Security' },
  { key: 'testing_score', label: 'Testing' },
  { key: 'cicd_score', label: 'CI/CD' },
  { key: 'documentation_score', label: 'Documentation' },
  { key: 'code_quality_score', label: 'Code Quality' },
  { key: 'maintenance_score', label: 'Maintenance' },
  { key: 'community_score', label: 'Community' },
] as const;

const CHECK_TO_CATEGORY: Record<string, string> = {
  branch_protection: 'Security',
  requires_reviews: 'Security',
  requires_status_checks: 'Security',
  enforce_admins: 'Security',
  no_force_pushes: 'Security',
  dependabot_enabled: 'Security',
  code_scanning_enabled: 'Security',
  security_md: 'Security',
  test_directory: 'Testing',
  test_config: 'Testing',
  ci_workflow_tests: 'Testing',
  has_workflows: 'CI/CD',
  status_checks_required: 'CI/CD',
  readme_exists: 'Documentation',
  readme_size: 'Documentation',
  license_exists: 'Documentation',
  contributing: 'Documentation',
  changelog: 'Documentation',
  description_set: 'Documentation',
  linter_config: 'Code Quality',
  editorconfig: 'Code Quality',
  type_config: 'Code Quality',
  dockerfile: 'Code Quality',
  pre_commit: 'Code Quality',
  pushed_recently_30: 'Maintenance',
  pushed_recently_90: 'Maintenance',
  not_archived: 'Maintenance',
  topics_set: 'Community',
  issue_templates: 'Community',
  pr_template: 'Community',
  codeowners: 'Community',
  code_of_conduct: 'Community',
};

const CHECK_META: Record<string, { description: string; remediation: string }> = {
  branch_protection: {
    description: 'Default branch has protection rules to prevent force pushes and require reviews.',
    remediation: 'Go to Settings → Branches → Add branch protection rule for the default branch. Enable "Require a pull request before merging" and restrict force pushes.',
  },
  requires_reviews: {
    description: 'Branch protection requires at least one approving review before merging.',
    remediation: 'In the branch protection rule, enable "Require approvals" and set required approving review count to 1 or more.',
  },
  requires_status_checks: {
    description: 'Branch protection requires CI status checks to pass before merging.',
    remediation: 'In the branch protection rule, enable "Require status checks to pass" and add the relevant workflow or check names.',
  },
  enforce_admins: {
    description: 'Branch protection rules apply to administrators as well.',
    remediation: 'In the branch protection rule, enable "Do not allow bypassing the above settings" (enforce for admins).',
  },
  no_force_pushes: {
    description: 'Force pushes to the branch are disabled.',
    remediation: 'In the branch protection rule, set "Allow force pushes" to "Do not allow force pushes".',
  },
  dependabot_enabled: {
    description: 'Dependabot alerts are enabled to track vulnerable dependencies.',
    remediation: 'Go to Settings → Security → Code security and analysis → Enable "Dependabot alerts".',
  },
  code_scanning_enabled: {
    description: 'Code scanning (e.g. CodeQL) is configured to find security issues.',
    remediation: 'Go to Settings → Security → Code security and analysis → Set up Code scanning (e.g. default CodeQL).',
  },
  security_md: {
    description: 'A SECURITY.md file describes how to report vulnerabilities.',
    remediation: 'Add a SECURITY.md file in the repo root or in .github/ describing your security policy and how to report issues.',
  },
  test_directory: {
    description: 'Repository has a dedicated directory for tests (e.g. test/, tests/, spec/).',
    remediation: 'Add a test directory (test, tests, spec, or __tests__) and place tests there.',
  },
  test_config: {
    description: 'A test framework config file is present (e.g. jest, pytest, vitest).',
    remediation: 'Add a config file for your test framework (e.g. jest.config.js, pytest.ini, vitest.config.ts).',
  },
  ci_workflow_tests: {
    description: 'At least one CI workflow runs tests (name contains test, ci, or build).',
    remediation: 'Add or rename a workflow under .github/workflows/ so it runs tests and its name suggests testing (e.g. "CI" or "Tests").',
  },
  has_workflows: {
    description: 'Repository has at least one GitHub Actions workflow.',
    remediation: 'Add one or more workflow files under .github/workflows/ (e.g. ci.yml) to automate build or tests.',
  },
  status_checks_required: {
    description: 'Branch protection requires status checks, ensuring CI runs before merge.',
    remediation: 'In the branch protection rule, enable "Require status checks" and list the workflow or check names.',
  },
  readme_exists: {
    description: 'A README file exists to describe the project.',
    remediation: 'Add a README.md file in the repository root with project overview, setup, and usage.',
  },
  readme_size: {
    description: 'README has sufficient content (e.g. at least ~500 bytes).',
    remediation: 'Expand README.md with a clear description, installation steps, and usage examples.',
  },
  license_exists: {
    description: 'A LICENSE file is present with a valid SPDX identifier.',
    remediation: 'Add a LICENSE file in the root (e.g. MIT, Apache-2.0) so reuse terms are clear.',
  },
  contributing: {
    description: 'A CONTRIBUTING file explains how to contribute.',
    remediation: 'Add CONTRIBUTING.md or CONTRIBUTING in the root with contribution guidelines and process.',
  },
  changelog: {
    description: 'A CHANGELOG file documents version history.',
    remediation: 'Add CHANGELOG.md in the root and keep it updated with notable changes per release.',
  },
  description_set: {
    description: 'Repository has a short description set on GitHub.',
    remediation: 'Edit the repository description on the main GitHub repo page (under the name).',
  },
  linter_config: {
    description: 'A linter or formatter config is present (e.g. ESLint, Prettier, Rubocop).',
    remediation: 'Add a config file for your language (e.g. .eslintrc.json, .prettierrc, .rubocop.yml).',
  },
  editorconfig: {
    description: 'An .editorconfig file enforces consistent editor settings.',
    remediation: 'Add .editorconfig in the root to define indentation, charset, and line endings.',
  },
  type_config: {
    description: 'Type-checking config is present (e.g. tsconfig for TypeScript).',
    remediation: 'Add tsconfig.json (or equivalent) if using TypeScript or other typed languages.',
  },
  dockerfile: {
    description: 'A Dockerfile is present for containerized build or run.',
    remediation: 'Add a Dockerfile in the root (or a dedicated directory) for container builds.',
  },
  pre_commit: {
    description: 'Pre-commit hooks are configured (e.g. .pre-commit-config.yaml).',
    remediation: 'Add .pre-commit-config.yaml and install pre-commit to run checks before commits.',
  },
  pushed_recently_30: {
    description: 'Repository had at least one push in the last 30 days.',
    remediation: 'Make a commit and push to the default branch to show recent activity.',
  },
  pushed_recently_90: {
    description: 'Repository had at least one push in the last 90 days.',
    remediation: 'Make a commit and push to the default branch to show recent activity.',
  },
  not_archived: {
    description: 'Repository is not archived and is actively maintained.',
    remediation: 'Unarchive the repository from Settings → Danger Zone if it should be active again.',
  },
  topics_set: {
    description: 'Repository has one or more topics (labels) set on GitHub.',
    remediation: 'Add topics on the main repo page (click the gear next to "About") to improve discoverability.',
  },
  issue_templates: {
    description: 'Issue templates are configured for structured bug reports or feature requests.',
    remediation: 'Add issue templates under .github/ISSUE_TEMPLATE/ or use the single .github/ISSUE_TEMPLATE.md file.',
  },
  pr_template: {
    description: 'A pull request template is configured.',
    remediation: 'Add .github/PULL_REQUEST_TEMPLATE.md or a file in .github/PULL_REQUEST_TEMPLATE/ to guide PR descriptions.',
  },
  codeowners: {
    description: 'A CODEOWNERS file defines owners for paths in the repo.',
    remediation: 'Add CODEOWNERS in the root or .github/ to assign owners (e.g. * @team/backend).',
  },
  code_of_conduct: {
    description: 'A code of conduct file sets community behavior expectations.',
    remediation: 'Add CODE_OF_CONDUCT.md in the root or use GitHub\'s template from Insights → Community.',
  },
};

function CheckBox({ check, pass }: { check: string; pass: boolean }) {
  const meta = CHECK_META[check];
  const label = check.replace(/_/g, ' ');
  return (
    <div className="relative group">
      <div
        className={cn(
          'min-h-[2.25rem] rounded-md px-2 py-1.5 text-xs font-medium flex items-center justify-center text-center break-words border-2 cursor-help',
          pass
            ? 'bg-green-500/15 border-green-500/70 text-green-700 dark:bg-green-500/20 dark:border-green-400/80 dark:text-green-300'
            : 'bg-red-500/15 border-red-500/70 text-red-700 dark:bg-red-500/20 dark:border-red-400/80 dark:text-red-300',
        )}
      >
        <span className="truncate">{label}</span>
      </div>
      {(meta || label) && (
        <div
          className="absolute top-full left-1/2 -translate-x-1/2 mt-1.5 px-3 py-2 w-72 rounded-lg border border-gray-200 dark:border-gray-600 bg-white dark:bg-gray-800 shadow-lg text-left opacity-0 invisible group-hover:opacity-100 group-hover:visible transition-opacity z-50 pointer-events-none"
          role="tooltip"
        >
          <p className="text-xs font-semibold text-gray-900 dark:text-gray-100 mb-1">{label}</p>
          {meta?.description && (
            <p className="text-xs text-gray-600 dark:text-gray-400 mb-1.5">{meta.description}</p>
          )}
          {!pass && meta?.remediation && (
            <p className="text-xs text-amber-700 dark:text-amber-300 border-t border-gray-200 dark:border-gray-600 pt-1.5 mt-1.5">
              <span className="font-medium">Remediation:</span> {meta.remediation}
            </p>
          )}
        </div>
      )}
    </div>
  );
}

export function RepositoryDetail() {
  const { id } = useParams<{ id: string }>();
  const { isDark } = useTheme();
  const [checksExpanded, setChecksExpanded] = useState(false);

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

  const queryClient = useQueryClient();
  const { data: score } = useQuery({
    queryKey: ['repositories', id, 'score'],
    queryFn: () => repositoriesApi.getScore(Number(id)),
    enabled: !!id,
  });

  const refreshGradeMutation = useMutation({
    mutationFn: () => repositoriesApi.syncRepo(Number(id)),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['repositories', id, 'score'] });
      queryClient.invalidateQueries({ queryKey: ['repositories', id] });
      queryClient.invalidateQueries({ queryKey: ['workflows', { repo_id: id }] });
    },
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

      {/* Repository Score */}
      {(score || repo) && (
        <div className="card">
          <div className="px-6 py-4 border-b border-gray-200 dark:border-gray-700 flex flex-wrap items-center justify-between gap-2">
            <div>
              <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100">Repository Score</h2>
              <p className="text-sm text-gray-500 dark:text-gray-400 mt-0.5">
                {score
                  ? `Last scanned: ${new Date(score.scanned_at).toLocaleString()}`
                  : 'No score yet. Sync to grade this repository.'}
              </p>
            </div>
            <button
              type="button"
              onClick={() => refreshGradeMutation.mutate()}
              disabled={refreshGradeMutation.isPending}
              className="btn btn-secondary flex items-center gap-2"
              title="Re-check on GitHub and update grading"
            >
              <RefreshCw className={cn('w-4 h-4', refreshGradeMutation.isPending && 'animate-spin')} />
              {refreshGradeMutation.isPending ? 'Checking…' : 'Refresh grade'}
            </button>
          </div>
          <div className="p-6 space-y-6">
            {!score ? (
              <p className="text-sm text-gray-500 dark:text-gray-400">
                Click &quot;Refresh grade&quot; to check this repository on GitHub and compute its score.
              </p>
            ) : (
              <>
            <div className="flex flex-wrap items-center gap-4">
              <span className="text-3xl font-bold text-gray-900 dark:text-gray-100">
                {Math.round(score.overall_score)}%
              </span>
              <span
                className={
                  score.tier === 'gold'
                    ? 'badge badge-gold'
                    : score.tier === 'silver'
                      ? 'badge badge-silver'
                      : 'badge badge-bronze'
                }
              >
                {score.tier === 'none' ? 'bronze' : score.tier}
              </span>
            </div>
            <div className="h-[280px]">
              <ResponsiveContainer width="100%" height="100%">
                <RadarChart
                  data={SCORE_CATEGORIES.map(({ key, label }) => ({
                    subject: label,
                    score:
                      key === 'security_score'
                        ? score.security_score
                        : key === 'testing_score'
                          ? score.testing_score
                          : key === 'cicd_score'
                            ? score.cicd_score
                            : key === 'documentation_score'
                              ? score.documentation_score
                              : key === 'code_quality_score'
                                ? score.code_quality_score
                                : key === 'maintenance_score'
                                  ? score.maintenance_score
                                  : score.community_score,
                    fullMark: 100,
                  }))}
                >
                  <PolarGrid stroke={isDark ? '#374151' : '#d1d5db'} />
                  <PolarAngleAxis
                    dataKey="subject"
                    tick={{ fill: isDark ? '#9ca3af' : '#6b7280', fontSize: 12 }}
                  />
                  <PolarRadiusAxis
                    angle={90}
                    domain={[0, 100]}
                    tick={{ fill: isDark ? '#9ca3af' : '#6b7280', fontSize: 10 }}
                  />
                  <Radar
                    name="Score"
                    dataKey="score"
                    stroke={isDark ? '#34cb6f' : '#2db865'}
                    fill={isDark ? '#34cb6f' : '#2db865'}
                    fillOpacity={0.3}
                  />
                  <Tooltip
                    contentStyle={{
                      backgroundColor: isDark ? '#1f2937' : '#fff',
                      border: `1px solid ${isDark ? '#374151' : '#e5e7eb'}`,
                      borderRadius: '8px',
                    }}
                    labelStyle={{ color: isDark ? '#e5e7eb' : '#111827' }}
                    formatter={(value: unknown) => [typeof value === 'number' ? `${value.toFixed(0)}%` : String(value), 'Score'] as [React.ReactNode, string]}
                  />
                </RadarChart>
              </ResponsiveContainer>
            </div>
            {score.check_results && Object.keys(score.check_results).length > 0 && (
              <div>
                <button
                  type="button"
                  onClick={() => setChecksExpanded((e) => !e)}
                  className="flex items-center gap-2 text-sm font-medium text-gray-700 dark:text-gray-300 hover:text-primary-600 dark:hover:text-primary-400"
                >
                  {checksExpanded ? (
                    <ChevronDown className="w-4 h-4" />
                  ) : (
                    <ChevronRight className="w-4 h-4" />
                  )}
                  Check details
                </button>
                {checksExpanded && (() => {
                  const order = ['Security', 'Testing', 'CI/CD', 'Documentation', 'Code Quality', 'Maintenance', 'Community'];
                  const byCategory = Object.entries(score.check_results).reduce<Record<string, [string, boolean][]>>((acc, [check, pass]) => {
                    const cat = CHECK_TO_CATEGORY[check] ?? 'Other';
                    if (!acc[cat]) acc[cat] = [];
                    acc[cat].push([check, pass]);
                    return acc;
                  }, {});
                  return (
                    <div className="mt-3 space-y-4">
                      {order.filter((cat) => byCategory[cat]?.length).map((category) => (
                        <div key={category}>
                          <p className="text-xs font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wider mb-2">
                            {category}
                          </p>
                          <div className="grid grid-cols-3 sm:grid-cols-4 md:grid-cols-5 gap-2">
                            {byCategory[category].map(([check, pass]) => (
                              <CheckBox key={check} check={check} pass={pass} />
                            ))}
                          </div>
                        </div>
                      ))}
                      {byCategory['Other']?.length > 0 && (
                        <div>
                          <p className="text-xs font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wider mb-2">
                            Other
                          </p>
                          <div className="grid grid-cols-3 sm:grid-cols-4 md:grid-cols-5 gap-2">
                            {byCategory['Other'].map(([check, pass]) => (
                              <CheckBox key={check} check={check} pass={pass} />
                            ))}
                          </div>
                        </div>
                      )}
                    </div>
                  );
                })()}
              </div>
            )}
              </>
            )}
          </div>
        </div>
      )}

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
                <span className={`badge ${workflow.state === 'active' ? 'badge-success' : 'badge-neutral'}`}>
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

