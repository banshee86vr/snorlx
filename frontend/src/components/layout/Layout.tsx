import { type ReactNode } from 'react';
import { RefreshCw } from 'lucide-react';
import { Sidebar } from './Sidebar';
import { Header } from './Header';
import { useSidebar } from '../../context/SidebarContext';
import { useSync } from '../../context/SyncContext';
import { cn } from '../../lib/utils';

interface LayoutProps {
  children: ReactNode;
}

export function Layout({ children }: LayoutProps) {
  const { isCollapsed } = useSidebar();
  const { sync } = useSync();

  return (
    <div className="min-h-screen bg-gray-50 dark:bg-slate-950 dark:bg-linear-to-br dark:from-slate-950 dark:via-slate-900 dark:to-primary-900/20">
      <Sidebar />
      <div className={cn(
        'transition-all duration-300',
        isCollapsed ? 'lg:pl-20' : 'lg:pl-64'
      )}>
        <Header />
        <main className="p-6">
          {children}
        </main>
      </div>

      {/* Global Live Sync Progress - Visible across all pages */}
      {sync.isSyncing && (
        <>
          {/* Overlay */}
          <div className="fixed inset-0 bg-black/30 backdrop-blur-xs z-40" />

          {/* Progress Box - Perfectly centered in viewport */}
          <div
            className="fixed z-50"
            style={{
              top: '50vh',
              left: '50vw',
              transform: 'translate(-50%, -50%)',
              width: 'min(28rem, calc(100vw - 2rem))',
            }}
          >
            <div className="bg-primary-50 dark:bg-primary-900/95 border border-primary-200 dark:border-primary-500/50 rounded-lg p-4 dark:shadow-lg dark:shadow-primary-500/10">
              <div className="flex items-center gap-3 mb-3">
                <RefreshCw className="w-5 h-5 text-primary-600 dark:text-primary-400 animate-spin" />
                <h3 className="font-semibold text-primary-900 dark:text-primary-100">
                  Syncing Repositories
                </h3>
                <span className="ml-auto text-sm text-primary-700 dark:text-primary-300">
                  {sync.syncedCount} of {sync.totalToSync}
                </span>
              </div>
              {sync.currentRepo && (
                <p className="text-sm text-primary-600 dark:text-primary-400 mb-3 truncate">
                  {sync.currentRepo}
                </p>
              )}
              {!sync.currentRepo && (
                <p className="text-sm text-primary-600 dark:text-primary-400 mb-3">
                  Fetching repositories from GitHub...
                </p>
              )}
              <div className="w-full h-2 bg-primary-200 dark:bg-primary-800/50 rounded-full overflow-hidden">
                <div
                  className="h-full bg-linear-to-r from-primary-500 to-primary-600 dark:from-primary-500 dark:to-primary-400 transition-all duration-300 ease-out"
                  style={{ width: `${sync.progress}%` }}
                />
              </div>
              <p className="text-xs text-primary-700 dark:text-primary-300 mt-2">
                {sync.progress}% complete
              </p>
            </div>
          </div>
        </>
      )}
    </div>
  );
}

