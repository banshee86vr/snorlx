import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
const pkg = JSON.parse(readFileSync(new URL("./package.json", import.meta.url), "utf8"));
// Dev only: the app is reached under BASE_PATH (shell or repo-root .env, like the backend).
// The proxy drops the prefix, as nginx does in the container.
const rootDir = fileURLToPath(new URL("..", import.meta.url));
const configured = process.env.BASE_PATH ?? loadEnv("development", rootDir, "").BASE_PATH ?? "";
const trimmed = configured.replace(/^\/+|\/+$/g, "");
const basePath = trimmed && `/${trimmed}`;
const dropBasePath = (path) => path.slice(basePath.length);
// https://vitejs.dev/config/
export default defineConfig({
    // Relative asset URLs: the <base> tag in index.html sets the base path at runtime
    base: "./",
    define: {
        __APP_VERSION__: JSON.stringify(pkg.version),
    },
    plugins: [
        tailwindcss(),
        react(),
        {
            // In the container nginx sets the base path in the <base> tag
            name: "dev-base-tag",
            apply: "serve",
            transformIndexHtml: (html) => html.replace('<base href="/" />', `<base href="${basePath}/" />`),
        },
    ],
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
            coverageFilesDirectory: "./node_modules/.vite/coverage",
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
            [`${basePath}/api`]: {
                target: "http://localhost:8080",
                changeOrigin: true,
                rewrite: dropBasePath,
                configure: (proxy) => {
                    // Silence proxy errors for long-running requests (proxy is http-proxy at runtime)
                    // eslint-disable-next-line @typescript-eslint/no-explicit-any
                    proxy.on("error", (err, _req, res) => {
                        if (err.message.includes("socket hang up") || err.message.includes("ECONNRESET")) {
                            return;
                        }
                        console.error("[vite proxy error]", err.message);
                        const resObj = res;
                        if (resObj?.writeHead && resObj?.end) {
                            resObj.writeHead(502, { "Content-Type": "text/plain" });
                            resObj.end("Proxy error");
                        }
                    });
                },
            },
            // WebSocket proxy (separate entry like krabbx)
            [`${basePath}/ws`]: {
                target: "http://localhost:8080",
                ws: true,
                rewrite: dropBasePath,
            },
        },
    },
});
