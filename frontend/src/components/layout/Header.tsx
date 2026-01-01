import { useState } from "react";
import {
	RefreshCw,
	Menu,
	X,
	Moon,
	Sun,
	LogOut,
	User,
	GitBranch,
	LayoutDashboard,
	Play,
	FolderGit2,
	Settings,
	Workflow,
} from "lucide-react";
import { useSocket } from "../../context/SocketContext";
import { useSync } from "../../context/SyncContext";
import { useTheme } from "../../context/ThemeContext";
import { useAuth } from "../../context/AuthContext";
import { cn } from "../../lib/utils";
import { NavLink, useNavigate } from "react-router-dom";
import { api } from "../../services/api";

const mobileNavigation = [
	{ name: "Dashboard", href: "/", icon: LayoutDashboard },
	{ name: "Workflows", href: "/workflows", icon: Workflow },
	{ name: "Runs", href: "/runs", icon: Play },
	{ name: "Repositories", href: "/repositories", icon: FolderGit2 },
	{ name: "Settings", href: "/settings", icon: Settings },
];

export function Header() {
	const [showUserMenu, setShowUserMenu] = useState(false);
	const [showMobileMenu, setShowMobileMenu] = useState(false);
	const { isConnected } = useSocket();
	const { sync, startSync } = useSync();
	const { isDark, setTheme } = useTheme();
	const { user, logout } = useAuth();
	const navigate = useNavigate();

	const handleLogout = async () => {
		await logout();
		navigate("/login");
	};

	const handleSync = () => {
		if (sync.isSyncing) return;

		startSync();

		// Fire and forget - progress and completion come via WebSocket
		api.syncRepositories().catch(() => {
			// Silent error handling - sync status is tracked via WebSocket
		});
	};

	return (
		<header className="sticky top-0 z-40 bg-white border-b border-gray-200 isolate dark:bg-slate-900/50 dark:border-secondary-500/30 dark:shadow-lg dark:shadow-secondary-500/5 dark:backdrop-blur-sm">
			<div className="relative z-10 flex items-center justify-between h-16 px-4 lg:px-6">
				{/* Mobile menu button */}
				<button
					type="button"
					onClick={() => setShowMobileMenu(!showMobileMenu)}
					className="lg:hidden p-2 rounded-md text-gray-500 hover:bg-gray-100 dark:text-slate-400 dark:hover:bg-slate-800/50 dark:hover:border dark:hover:border-purple-500/30 transition-all"
				>
					{showMobileMenu ? (
						<X className="w-6 h-6" />
					) : (
						<Menu className="w-6 h-6" />
					)}
				</button>

				{/* Left side - Connection status */}
				<div className="hidden lg:flex items-center gap-2">
					<div
						className={cn(
							"w-2 h-2 rounded-full transition-all",
							isConnected
								? "bg-emerald-400 dark:shadow-lg dark:shadow-emerald-500/50"
								: "bg-slate-400",
						)}
					/>
					<span className="text-sm text-gray-500 dark:text-slate-400">
						{isConnected ? "Connected" : "Disconnected"}
					</span>
				</div>

				{/* Right side - Actions */}
				<div className="relative z-10 flex items-center gap-2">
					{/* Theme toggle */}
					<button
						type="button"
						onClick={() => setTheme(isDark ? "light" : "dark")}
						className="inline-flex items-center gap-2 px-3 py-2 rounded-lg font-medium text-sm transition-all text-gray-900 hover:bg-gray-100 dark:text-slate-100 dark:hover:bg-slate-800/50 dark:border dark:border-secondary-500/20 dark:hover:border-secondary-500/50"
						title={`Switch to ${isDark ? "light" : "dark"} mode`}
					>
						{isDark ? (
							<Sun className="w-4 h-4" />
						) : (
							<Moon className="w-4 h-4" />
						)}
					</button>

					{/* Sync button */}
					<button
						type="button"
						onClick={handleSync}
						disabled={sync.isSyncing}
						className={cn(
							"relative inline-flex items-center gap-2 px-3 py-2 rounded-lg font-medium text-sm transition-all",
							sync.isSyncing
								? "bg-gray-200 text-gray-600 cursor-not-allowed dark:bg-slate-800 dark:text-slate-500 dark:border dark:border-slate-600/50"
								: "bg-gray-100 text-gray-900 hover:bg-gray-200 dark:bg-slate-800/50 dark:text-slate-100 dark:hover:bg-slate-700 dark:border dark:border-secondary-500/20 dark:hover:border-secondary-500/50 dark:hover:shadow-lg dark:hover:shadow-secondary-500/10",
						)}
					>
						<RefreshCw
							className={cn("w-4 h-4", sync.isSyncing && "animate-spin")}
						/>
						<span className="hidden sm:inline">
							{sync.isSyncing ? "Syncing..." : "Sync"}
						</span>
					</button>

					{/* User menu */}
					<div className="relative">
						<button
							type="button"
							onClick={() => setShowUserMenu(!showUserMenu)}
							className="relative inline-flex items-center gap-2 px-3 py-2 rounded-lg font-medium text-sm transition-all text-gray-900 hover:bg-gray-100 dark:text-slate-100 dark:hover:bg-slate-800/50 dark:border dark:border-secondary-500/20 dark:hover:border-secondary-500/50"
						>
							{user?.avatar_url ? (
								<img
									src={user.avatar_url}
									alt={user.name || user.login}
									className="w-6 h-6 rounded-full"
								/>
							) : (
								<User className="w-5 h-5" />
							)}
							<span className="hidden md:inline">
								{user?.name || user?.login}
							</span>
						</button>

						{/* User dropdown */}
						{showUserMenu && (
							<div className="absolute right-0 mt-2 w-64 bg-white rounded-lg shadow-lg border border-gray-200 animate-fadeIn dark:bg-slate-800 dark:border-secondary-500/30 dark:shadow-2xl dark:shadow-secondary-500/10">
								<div className="px-4 py-3 border-b border-gray-200 dark:border-secondary-500/20">
									<p className="text-sm font-medium text-gray-900 dark:text-slate-100">
										{user?.name}
									</p>
									<p className="text-xs text-gray-500 dark:text-slate-400">
										{user?.email}
									</p>
								</div>
								<div className="py-2">
									<button
										type="button"
										onClick={() => {
											setShowUserMenu(false);
											handleLogout();
										}}
										className="w-full flex items-center gap-2 px-4 py-2 text-sm text-left text-gray-700 hover:bg-gray-100 dark:text-slate-300 dark:hover:bg-slate-700/50 transition-colors"
									>
										<LogOut className="w-4 h-4" />
										Logout
									</button>
								</div>
							</div>
						)}
					</div>
				</div>
			</div>

			{/* Mobile navigation */}
			{showMobileMenu && (
				<div className="lg:hidden border-t border-gray-200 bg-white animate-slideIn dark:border-secondary-500/20 dark:bg-slate-800/50">
					<div className="flex items-center gap-3 px-4 py-3 border-b border-gray-100 dark:border-secondary-500/20">
						<div className="flex items-center justify-center w-8 h-8 rounded-lg bg-gradient-to-br from-primary-500 to-secondary-600 text-white dark:shadow-lg dark:shadow-secondary-500/50">
							<GitBranch className="w-5 h-5" />
						</div>
						<span className="font-semibold text-gray-900 dark:text-slate-100">
							Snorlx Dashboard
						</span>
					</div>

					<nav className="px-2 py-2">
						{mobileNavigation.map((item) => (
							<NavLink
								key={item.name}
								to={item.href}
								onClick={() => setShowMobileMenu(false)}
								className={({ isActive }) =>
									cn(
										"flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm font-medium transition-all",
										isActive
											? "bg-primary-600/20 text-primary-600 dark:bg-primary-500/20 dark:text-primary-300 dark:border dark:border-primary-500/50"
											: "text-gray-600 hover:bg-gray-100 dark:text-slate-400 dark:hover:bg-slate-700/50 dark:hover:border dark:hover:border-secondary-500/30",
									)
								}
							>
								<item.icon className="w-5 h-5" />
								{item.name}
							</NavLink>
						))}
					</nav>
				</div>
			)}
		</header>
	);
}
