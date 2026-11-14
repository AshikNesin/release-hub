import { writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { defineConfig, type Plugin } from "vite";

const root = path.dirname(fileURLToPath(import.meta.url));
const outDir = path.resolve(root, "dist");
// The Go server (make run) the dev server proxies API and device routes to.
const backend = process.env.RELEASE_HUB_URL ?? "http://localhost:9100";

/** Keep dist/.gitkeep after a build: go:embed (web/embed.go) needs a match even before the UI is built. */
function keepGitkeep(): Plugin {
  return {
    name: "keep-gitkeep",
    closeBundle() {
      writeFileSync(path.join(outDir, ".gitkeep"), "");
    },
  };
}

export default defineConfig({
  plugins: [react(), tailwindcss(), keepGitkeep()],
  resolve: {
    // Same aliases as groot, pointed at the vendored copies (scripts/sync-groot.sh).
    alias: [
      { find: "@groot/ui", replacement: path.resolve(root, "vendor/groot/ui/src") },
      { find: "@groot/shell", replacement: path.resolve(root, "vendor/groot/shell/src") },
      { find: "@", replacement: path.resolve(root, "src") },
    ],
  },
  build: {
    outDir,
    emptyOutDir: true,
    // The main chunk is mostly the groot kit (Radix, motion, react-query, zod),
    // the same as in groot itself; pages are split per route.
    chunkSizeWarningLimit: 700,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (
            /node_modules\/(react|react-dom|react-router|react-router-dom|scheduler)\//.test(id)
          ) {
            return "react-vendor";
          }
        },
      },
    },
  },
  server: {
    port: 5173,
    // Everything that is not the SPA goes to Go. changeOrigin stays false so
    // the Go side sees the browser's Host and its same-origin check passes.
    proxy: Object.fromEntries(
      ["/api", "/artifacts", "/health", "^/apps/[^/]+/(download|get)"].map((p) => [
        p,
        { target: backend },
      ]),
    ),
  },
});
