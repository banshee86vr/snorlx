import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Settings as SettingsIcon, Moon, Sun, Monitor, Shield, Database, Key, Copy, Trash2 } from 'lucide-react';
import { useTheme } from '../context/ThemeContext';
import { cn } from '../lib/utils';
import { tokensApi } from '../services/api';
import { AboutCard } from '../components/settings/AboutCard';

export function Settings() {
  const { theme, setTheme } = useTheme();
  const queryClient = useQueryClient();
  const [tokenName, setTokenName] = useState('');
  const [readOnly, setReadOnly] = useState(false);
  const [createdToken, setCreatedToken] = useState<string | null>(null);
  const [copyDone, setCopyDone] = useState(false);

  const { data: tokensData, isLoading: tokensLoading } = useQuery({
    queryKey: ['api-tokens'],
    queryFn: tokensApi.list,
  });

  const createMutation = useMutation({
    mutationFn: () =>
      tokensApi.create(tokenName.trim(), readOnly ? ['read'] : ['read', 'write']),
    onSuccess: (created) => {
      setCreatedToken(created.token);
      setTokenName('');
      setReadOnly(false);
      queryClient.invalidateQueries({ queryKey: ['api-tokens'] });
    },
  });

  const revokeMutation = useMutation({
    mutationFn: (id: number) => tokensApi.revoke(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['api-tokens'] });
    },
  });

  const copyToken = async () => {
    if (!createdToken) return;
    await navigator.clipboard.writeText(createdToken);
    setCopyDone(true);
    setTimeout(() => setCopyDone(false), 2000);
  };

  return (
    <div className="min-w-0 space-y-6">
      <div>
        <h1 className="text-xl font-bold text-gray-900 sm:text-2xl dark:text-gray-100">Settings</h1>
        <p className="text-gray-500 dark:text-gray-400 mt-1">
          Manage your dashboard preferences
        </p>
      </div>

      {/* Appearance */}
      <div className="card">
        <div className="border-b border-gray-200 px-4 py-4 sm:px-6 dark:border-gray-700">
          <div className="flex items-center gap-2">
            <SettingsIcon className="w-5 h-5 text-gray-500" />
            <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100">Appearance</h2>
          </div>
        </div>
        <div className="p-4 sm:p-6">
          <div className="space-y-4">
            <div>
              <label className="text-sm font-medium text-gray-700 dark:text-gray-300 mb-3 block">
                Theme
              </label>
              <div className="grid grid-cols-3 gap-2 sm:flex sm:flex-wrap sm:gap-3">
                {[
                  { value: 'light', label: 'Light', icon: Sun },
                  { value: 'dark', label: 'Dark', icon: Moon },
                  { value: 'system', label: 'System', icon: Monitor },
                ].map((option) => (
                  <button
                    key={option.value}
                    onClick={() => setTheme(option.value as 'light' | 'dark' | 'system')}
                    className={cn(
                      'flex flex-col items-center justify-center gap-1 rounded-lg border px-2 py-2 text-xs transition-colors sm:flex-row sm:gap-2 sm:px-4 sm:text-sm',
                      theme === option.value
                        ? 'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300'
                        : 'border-gray-200 dark:border-gray-700 hover:bg-gray-50 dark:hover:bg-gray-800'
                    )}
                  >
                    <option.icon className="w-4 h-4" />
                    {option.label}
                  </button>
                ))}
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* API tokens */}
      <div className="card">
        <div className="border-b border-gray-200 px-4 py-4 sm:px-6 dark:border-gray-700">
          <div className="flex items-center gap-2">
            <Key className="w-5 h-5 text-gray-500" />
            <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100">API tokens</h2>
          </div>
        </div>
        <div className="space-y-4 p-4 sm:p-6">
          <p className="text-sm text-gray-500 dark:text-gray-400">
            Create a personal token for MCP clients and automation. The full token is shown only once.
          </p>

          {createdToken && (
            <div className="rounded-lg border border-amber-300 bg-amber-50 dark:bg-amber-950/40 dark:border-amber-700 p-4 space-y-2">
              <p className="text-sm font-medium text-amber-900 dark:text-amber-200">
                Copy this token now. You will not see it again.
              </p>
              <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
                <code className="min-w-0 flex-1 break-all rounded border border-amber-200 bg-white px-3 py-2 text-xs dark:border-amber-800 dark:bg-gray-900">
                  {createdToken}
                </code>
                <button
                  type="button"
                  onClick={copyToken}
                  className="inline-flex shrink-0 items-center justify-center gap-1 rounded-lg bg-amber-600 px-3 py-2 text-sm text-white hover:bg-amber-700"
                >
                  <Copy className="w-4 h-4" />
                  {copyDone ? 'Copied' : 'Copy'}
                </button>
              </div>
              <button
                type="button"
                className="text-xs text-amber-800 dark:text-amber-300 underline"
                onClick={() => setCreatedToken(null)}
              >
                Dismiss
              </button>
            </div>
          )}

          <div className="flex flex-col sm:flex-row gap-3 sm:items-end">
            <div className="flex-1">
              <label className="text-sm font-medium text-gray-700 dark:text-gray-300 mb-1 block">
                Token name
              </label>
              <input
                type="text"
                value={tokenName}
                onChange={(e) => setTokenName(e.target.value)}
                placeholder="e.g. Cursor MCP"
                className="w-full px-3 py-2 rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900 text-sm"
              />
            </div>
            <label className="flex items-center gap-2 text-sm text-gray-600 dark:text-gray-300 pb-2">
              <input
                type="checkbox"
                checked={readOnly}
                onChange={(e) => setReadOnly(e.target.checked)}
              />
              Read-only
            </label>
            <button
              type="button"
              disabled={!tokenName.trim() || createMutation.isPending}
              onClick={() => createMutation.mutate()}
              className="px-4 py-2 rounded-lg bg-primary-600 text-white text-sm font-medium hover:bg-primary-700 disabled:opacity-50"
            >
              {createMutation.isPending ? 'Creating…' : 'Create token'}
            </button>
          </div>

          {createMutation.isError && (
            <p className="text-sm text-red-600">{(createMutation.error as Error).message}</p>
          )}

          <div className="divide-y divide-gray-200 dark:divide-gray-700 border border-gray-200 dark:border-gray-700 rounded-lg">
            {tokensLoading && (
              <p className="p-4 text-sm text-gray-500">Loading tokens…</p>
            )}
            {!tokensLoading && (tokensData?.data?.length ?? 0) === 0 && (
              <p className="p-4 text-sm text-gray-500">No API tokens yet.</p>
            )}
            {tokensData?.data?.map((token) => (
              <div key={token.id} className="flex flex-col gap-3 p-4 sm:flex-row sm:items-center sm:justify-between">
                <div className="min-w-0">
                  <p className="break-words font-medium text-gray-900 dark:text-gray-100">{token.name}</p>
                  <p className="mt-1 break-words text-xs text-gray-500 dark:text-gray-400">
                    {token.token_prefix}… · scopes: {token.scopes.join(', ')} · created{' '}
                    {new Date(token.created_at).toLocaleString()}
                  </p>
                </div>
                <button
                  type="button"
                  onClick={() => {
                    if (confirm(`Revoke token "${token.name}"?`)) {
                      revokeMutation.mutate(token.id);
                    }
                  }}
                  className="inline-flex w-fit shrink-0 items-center gap-1 self-start rounded-lg border border-red-200 px-3 py-1.5 text-sm text-red-600 hover:bg-red-50 dark:border-red-900 dark:hover:bg-red-950"
                >
                  <Trash2 className="w-4 h-4" />
                  Revoke
                </button>
              </div>
            ))}
          </div>
        </div>
      </div>

      {/* GitHub App */}
      <div className="card">
        <div className="border-b border-gray-200 px-4 py-4 sm:px-6 dark:border-gray-700">
          <div className="flex items-center gap-2">
            <Shield className="w-5 h-5 text-gray-500" />
            <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100">GitHub App</h2>
          </div>
        </div>
        <div className="p-4 sm:p-6">
          <div className="space-y-4">
            <div className="flex flex-col gap-2 rounded-lg bg-gray-50 p-4 sm:flex-row sm:items-center sm:justify-between dark:bg-gray-800">
              <div>
                <p className="font-medium text-gray-900 dark:text-gray-100">Connected</p>
                <p className="text-sm text-gray-500 dark:text-gray-400">
                  Your GitHub App is connected and receiving webhooks
                </p>
              </div>
              <span className="flex items-center gap-2">
                <span className="w-2 h-2 rounded-full bg-green-500"></span>
                <span className="text-sm text-green-600 dark:text-green-400">Active</span>
              </span>
            </div>
            <div>
              <p className="text-sm text-gray-500 dark:text-gray-400 mb-2">Permissions</p>
              <ul className="text-sm space-y-1 text-gray-600 dark:text-gray-300">
                <li>• actions: read</li>
                <li>• checks: read</li>
                <li>• metadata: read</li>
                <li>• deployments: read</li>
              </ul>
            </div>
          </div>
        </div>
      </div>

      {/* Data */}
      <div className="card">
        <div className="border-b border-gray-200 px-4 py-4 sm:px-6 dark:border-gray-700">
          <div className="flex items-center gap-2">
            <Database className="w-5 h-5 text-gray-500" />
            <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100">Data Management</h2>
          </div>
        </div>
        <div className="p-4 sm:p-6">
          <div className="space-y-4">
            <div className="flex flex-col gap-2 rounded-lg bg-gray-50 p-4 sm:flex-row sm:items-center sm:justify-between dark:bg-gray-800">
              <div>
                <p className="font-medium text-gray-900 dark:text-gray-100">Data Retention</p>
                <p className="text-sm text-gray-500 dark:text-gray-400">
                  Workflow run data is retained for 1 year
                </p>
              </div>
              <span className="text-sm text-gray-600 dark:text-gray-300">365 days</span>
            </div>
            <p className="text-sm text-gray-500 dark:text-gray-400">
              Data older than the retention period is automatically cleaned up by TimescaleDB retention policies.
            </p>
          </div>
        </div>
      </div>

      <AboutCard />
    </div>
  );
}
