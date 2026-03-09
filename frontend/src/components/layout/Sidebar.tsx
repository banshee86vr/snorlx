import { NavLink } from 'react-router-dom';
import {
  LayoutDashboard,
  Play,
  FolderGit2,
  Settings,
  Workflow,
  ChevronLeft,
  ChevronRight,
  GitBranch,
} from 'lucide-react';
import { cn } from '../../lib/utils';
import { useSidebar } from '../../context/SidebarContext';

const navigation = [
  { name: 'Dashboard', href: '/', icon: LayoutDashboard },
  { name: 'Workflows', href: '/workflows', icon: Workflow },
  { name: 'Runs', href: '/runs', icon: Play },
  { name: 'Repositories', href: '/repositories', icon: FolderGit2 },
  { name: 'Settings', href: '/settings', icon: Settings },
];

export function Sidebar() {
  const { isCollapsed, toggleSidebar } = useSidebar();

  return (
    <aside className={cn(
      'fixed inset-y-0 left-0 z-50 hidden bg-white border-r border-gray-200 lg:flex lg:flex-col transition-all duration-300 dark:bg-slate-900/50 dark:border-secondary-500/30 dark:shadow-2xl dark:shadow-secondary-500/5',
      isCollapsed ? 'w-20' : 'w-64'
    )}>
      <div className="flex flex-col h-full">
        {/* Logo */}
        <div className={cn('flex items-center gap-3 px-6 h-16 border-b border-gray-200 dark:border-secondary-500/20', isCollapsed && 'justify-center')}>
          <div className="flex items-center justify-center w-10 h-10 rounded-lg bg-linear-to-br from-primary-500 to-secondary-600 text-white shrink-0 dark:shadow-lg dark:shadow-secondary-500/50">
            <GitBranch className="w-6 h-6" />
          </div>
          {!isCollapsed && (
            <div>
              <h1 className="font-semibold text-gray-900 dark:text-slate-100">Snorlx</h1>
              <p className="text-xs text-gray-500 dark:text-slate-400">CI/CD Dashboard</p>
            </div>
          )}
        </div>

        {/* Navigation */}
        <nav className="flex-1 px-3 py-4 space-y-1 overflow-y-auto">
          {navigation.map((item) => (
            <NavLink
              key={item.name}
              to={item.href}
              title={isCollapsed ? item.name : undefined}
              className={({ isActive }) =>
                cn(
                  'flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm font-medium transition-all duration-200',
                  isCollapsed && 'justify-center',
                  isActive
                    ? 'bg-primary-600/20 text-primary-600 dark:bg-primary-500/20 dark:text-primary-300 dark:border dark:border-primary-500/50 dark:shadow-lg dark:shadow-primary-500/20'
                    : 'text-gray-600 hover:bg-gray-100 hover:text-gray-900 dark:text-slate-400 dark:hover:bg-slate-800/50 dark:hover:text-slate-100 dark:hover:border dark:hover:border-secondary-500/30'
                )
              }
            >
              <item.icon className="w-5 h-5 shrink-0" />
              {!isCollapsed && item.name}
            </NavLink>
          ))}
        </nav>

        {/* Toggle Button */}
        <div className={cn('px-3 py-4 border-t border-gray-200 dark:border-secondary-500/20', isCollapsed && 'flex justify-center')}>
          <button
            type="button"
            onClick={toggleSidebar}
            className={cn(
              'flex items-center gap-2 px-3 py-2 rounded-lg text-sm font-medium text-gray-600 hover:bg-gray-100 hover:text-gray-900 transition-all dark:text-slate-400 dark:hover:bg-slate-800/50 dark:hover:text-slate-100 dark:border dark:border-secondary-500/20 dark:hover:border-secondary-500/50',
              isCollapsed && 'justify-center'
            )}
            title={isCollapsed ? 'Expand sidebar' : 'Collapse sidebar'}
          >
            {isCollapsed ? (
              <ChevronRight className="w-5 h-5" />
            ) : (
              <>
                <ChevronLeft className="w-5 h-5" />
                <span>Collapse</span>
              </>
            )}
          </button>
        </div>

        {/* Footer */}
        {!isCollapsed && (
          <div className="px-6 py-4 border-t border-gray-200 dark:border-secondary-500/20">
            <p className="text-xs text-gray-400 dark:text-slate-500">
              Snorlx CI/CD Dashboard v1.0
            </p>
          </div>
        )}
      </div>
    </aside>
  );
}

