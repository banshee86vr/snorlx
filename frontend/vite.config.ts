import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// https://vitejs.dev/config/
export default defineConfig({
	plugins: [react()],
	resolve: {
		alias: {
			"@": "/src",
		},
	},
	// @ts-expect-error - Vitest extends Vite config with test (see vitest/config)
	test: {
		globals: true,
		environment: "jsdom",
		setupFiles: ["./src/test/setup.ts"],
		coverage: {
			provider: "v8",
			reporter: ["text", "lcov"],
			include: ["src/**/*.{ts,tsx}"],
			exclude: ["src/main.tsx", "src/vite-env.d.ts", "src/test/**"],
		},
	},
	server: {
		port: 5173,
		proxy: {
			// REST API proxy (no WebSocket)
			"/api": {
				target: "http://localhost:8080",
				changeOrigin: true,
				configure: (proxy) => {
					// Silence proxy errors for long-running requests
					proxy.on("error", (err, _req, res) => {
						if (err.message.includes("socket hang up") || err.message.includes("ECONNRESET")) {
							return;
						}
						console.error("[vite proxy error]", err.message);
						if (res && "writeHead" in res) {
							res.writeHead(502, { "Content-Type": "text/plain" });
							res.end("Proxy error");
						}
					});
				},
			},
			// WebSocket proxy (separate entry like krabbx)
			"/ws": {
				target: "http://localhost:8080",
				ws: true,
			},
		},
	},
});
