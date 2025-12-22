import { Routes, Route } from "react-router-dom";
import { Layout } from "./components/layout/Layout";
import { ProtectedRoute } from "./components/ProtectedRoute";
import { Dashboard } from "./pages/Dashboard";
import { Workflows } from "./pages/Workflows";
import { WorkflowDetail } from "./pages/WorkflowDetail";
import { Runs } from "./pages/Runs";
import { RunDetail } from "./pages/RunDetail";
import { Repositories } from "./pages/Repositories";
import { RepositoryDetail } from "./pages/RepositoryDetail";
import { Settings } from "./pages/Settings";
import { Login } from "./pages/Login";
import { AuthProvider } from "./context/AuthContext";
import { SocketProvider } from "./context/SocketContext";
import { SidebarProvider } from "./context/SidebarContext";
import { SyncProvider } from "./context/SyncContext";
import { ThemeProvider } from "./context/ThemeContext";

function App() {
	return (
		<ThemeProvider>
			<AuthProvider>
				<Routes>
					{/* Public routes */}
					<Route path="/login" element={<Login />} />

					{/* Protected routes */}
					<Route
						path="/*"
						element={
							<ProtectedRoute>
								<SocketProvider>
									<SyncProvider>
										<SidebarProvider>
											<Layout>
												<Routes>
													<Route path="/" element={<Dashboard />} />
													<Route path="/workflows" element={<Workflows />} />
													<Route
														path="/workflows/:id"
														element={<WorkflowDetail />}
													/>
													<Route path="/runs" element={<Runs />} />
													<Route path="/runs/:id" element={<RunDetail />} />
													<Route
														path="/repositories"
														element={<Repositories />}
													/>
													<Route
														path="/repositories/:id"
														element={<RepositoryDetail />}
													/>
													<Route path="/settings" element={<Settings />} />
												</Routes>
											</Layout>
										</SidebarProvider>
									</SyncProvider>
								</SocketProvider>
							</ProtectedRoute>
						}
					/>
				</Routes>
			</AuthProvider>
		</ThemeProvider>
	);
}

export default App;
