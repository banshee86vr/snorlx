import { useEffect, useState } from "react";
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

	useEffect(() => {
		const desktop = window.matchMedia("(min-width: 1024px)");
		const closeMenu = () => {
			if (desktop.matches) setShowMobileMenu(false);
		};
		desktop.addEventListener("change", closeMenu);
		return () => desktop.removeEventListener("change", closeMenu);
	}, []);

	return (
		<header className="sticky top-0 z-40 isolate border-b border-gray-200 bg-white pt-[env(safe-area-inset-top)] dark:border-secondary-500/30 dark:bg-slate-900/50 dark:shadow-lg dark:shadow-secondary-500/5 dark:backdrop-blur-xs">
			<div className="relative z-10 flex h-16 items-center justify-between gap-2 px-3 sm:px-4 lg:px-6">
				<div className="flex min-w-0 items-center gap-2">
					<button
						type="button"
						onClick={() => {
							setShowUserMenu(false);
							setShowMobileMenu(!showMobileMenu);
						}}
						className="inline-flex h-11 w-11 items-center justify-center rounded-md text-gray-500 transition-all hover:bg-gray-100 lg:hidden dark:text-slate-400 dark:hover:border dark:hover:border-purple-500/30 dark:hover:bg-slate-800/50"
						aria-label={showMobileMenu ? "Close menu" : "Open menu"}
						aria-expanded={showMobileMenu}
						aria-controls="mobile-navigation"
					>
						{showMobileMenu ? (
							<X className="h-6 w-6" />
						) : (
							<Menu className="h-6 w-6" />
						)}
					</button>

					<div className="flex min-w-0 items-center gap-2">
						<div
							className={cn(
								"h-2 w-2 shrink-0 rounded-full transition-all",
								isConnected
									? "bg-emerald-400 dark:shadow-lg dark:shadow-emerald-500/50"
									: "bg-slate-400",
							)}
						/>
						<span className="hidden truncate text-sm text-gray-500 sm:inline dark:text-slate-400">
							{isConnected ? "Connected" : "Disconnected"}
						</span>
					</div>
				</div>

				<div className="relative z-10 flex shrink-0 items-center gap-1 sm:gap-2">
					<button
						type="button"
						onClick={() => setTheme(isDark ? "light" : "dark")}
						className="inline-flex h-11 w-11 items-center justify-center rounded-lg text-sm font-medium text-gray-900 transition-all hover:bg-gray-100 sm:h-auto sm:w-auto sm:px-3 sm:py-2 dark:border dark:border-secondary-500/20 dark:text-slate-100 dark:hover:border-secondary-500/50 dark:hover:bg-slate-800/50"
						title={`Switch to ${isDark ? "light" : "dark"} mode`}
						aria-label={`Switch to ${isDark ? "light" : "dark"} mode`}
					>
						{isDark ? (
							<Sun className="w-4 h-4" />
						) : (
							<Moon className="w-4 h-4" />
						)}
					</button>

					<button
						type="button"
						onClick={handleSync}
						disabled={sync.isSyncing}
						className={cn(
							"relative inline-flex h-11 items-center gap-2 rounded-lg px-2.5 text-sm font-medium transition-all sm:h-auto sm:px-3 sm:py-2",
							sync.isSyncing
								? "cursor-not-allowed bg-gray-200 text-gray-600 dark:border dark:border-slate-600/50 dark:bg-slate-800 dark:text-slate-500"
								: "bg-gray-100 text-gray-900 hover:bg-gray-200 dark:border dark:border-secondary-500/20 dark:bg-slate-800/50 dark:text-slate-100 dark:hover:border-secondary-500/50 dark:hover:bg-slate-700 dark:hover:shadow-lg dark:hover:shadow-secondary-500/10",
						)}
						aria-label={sync.isSyncing ? "Syncing repositories" : "Sync repositories"}
					>
						<RefreshCw
							className={cn("w-4 h-4", sync.isSyncing && "animate-spin")}
						/>
						<span className="hidden sm:inline">
							{sync.isSyncing ? "Syncing..." : "Sync"}
						</span>
					</button>

					<div className="relative">
						<button
							type="button"
							onClick={() => setShowUserMenu(!showUserMenu)}
							className="relative inline-flex h-11 max-w-[42vw] items-center gap-2 rounded-lg px-2 text-sm font-medium text-gray-900 transition-all hover:bg-gray-100 sm:h-auto sm:max-w-xs sm:px-3 sm:py-2 dark:border dark:border-secondary-500/20 dark:text-slate-100 dark:hover:border-secondary-500/50 dark:hover:bg-slate-800/50"
							aria-expanded={showUserMenu}
							aria-haspopup="menu"
							aria-label={user?.name || user?.login || "Account menu"}
						>
							{user?.avatar_url ? (
								<img
									src={user.avatar_url}
									alt=""
									className="h-6 w-6 shrink-0 rounded-full"
								/>
							) : (
								<User className="h-5 w-5 shrink-0" />
							)}
							<span className="hidden truncate md:inline">
								{user?.name || user?.login}
							</span>
						</button>

						{showUserMenu && (
							<div className="absolute right-0 mt-2 w-64 max-w-[calc(100vw-1.5rem)] animate-fadeIn rounded-lg border border-gray-200 bg-white shadow-lg dark:border-secondary-500/30 dark:bg-slate-800 dark:shadow-2xl dark:shadow-secondary-500/10">
								<div className="border-b border-gray-200 px-4 py-3 dark:border-secondary-500/20">
									<p className="truncate text-sm font-medium text-gray-900 dark:text-slate-100">
										{user?.name}
									</p>
									<p className="truncate text-xs text-gray-500 dark:text-slate-400">
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
										className="flex w-full items-center gap-2 px-4 py-2 text-left text-sm text-gray-700 transition-colors hover:bg-gray-100 dark:text-slate-300 dark:hover:bg-slate-700/50"
									>
										<LogOut className="h-4 w-4" />
										Logout
									</button>
								</div>
							</div>
						)}
					</div>
				</div>
			</div>

			{showMobileMenu && (
				<div
					id="mobile-navigation"
					className="max-h-[calc(100dvh-4rem-env(safe-area-inset-top))] animate-slideIn overflow-y-auto border-t border-gray-200 bg-white lg:hidden dark:border-secondary-500/20 dark:bg-slate-800/50"
				>
					<div className="flex items-center gap-3 border-b border-gray-100 px-4 py-3 dark:border-secondary-500/20">
						<div className="flex h-8 w-8 items-center justify-center rounded-lg bg-linear-to-br from-primary-500 to-secondary-600 text-white dark:shadow-lg dark:shadow-secondary-500/50">
							<GitBranch className="h-5 w-5" />
						</div>
						<div className="min-w-0">
							<p className="font-semibold text-gray-900 dark:text-slate-100">
								Snorlx Dashboard
							</p>
							<p className="text-xs text-gray-500 dark:text-slate-400">
								{isConnected ? "Connected" : "Disconnected"}
							</p>
						</div>
					</div>

					<nav className="px-2 py-2">
						{mobileNavigation.map((item) => (
							<NavLink
								key={item.name}
								to={item.href}
								onClick={() => setShowMobileMenu(false)}
								className={({ isActive }) =>
									cn(
										"flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm font-medium transition-all",
										isActive
											? "bg-primary-600/20 text-primary-600 dark:border dark:border-primary-500/50 dark:bg-primary-500/20 dark:text-primary-300"
											: "text-gray-600 hover:bg-gray-100 dark:text-slate-400 dark:hover:border dark:hover:border-secondary-500/30 dark:hover:bg-slate-700/50",
									)
								}
							>
								<item.icon className="h-5 w-5" />
								{item.name}
							</NavLink>
						))}
					</nav>
				</div>
			)}
		</header>
	);
}
