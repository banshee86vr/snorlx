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
    server: {
        port: 5173,
        proxy: {
            // REST API proxy (no WebSocket)
            "/api": {
                target: "http://localhost:8080",
                changeOrigin: true,
                configure: function (proxy) {
                    // Silence proxy errors for long-running requests
                    proxy.on("error", function (err, _req, res) {
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
