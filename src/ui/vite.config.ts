import { execFile } from "node:child_process";
import { fileURLToPath } from "node:url";
import type { Plugin } from "vite";
import { defineConfig } from "vitest/config";
import { svelte } from "@sveltejs/vite-plugin-svelte";
import tailwindcss from "@tailwindcss/vite";

const UI_ROOT = fileURLToPath(new URL(".", import.meta.url));
const OPENAPI_SPEC = fileURLToPath(new URL("../api/openapi.yaml", import.meta.url));

/**
 * Regenerate the hey-api client when the OpenAPI contract changes, mid-session.
 *
 * `pnpm dev` generates the client once at startup, but the spec lives outside the
 * Vite root and no module imports it, so it is not in the module graph and Vite
 * never watches it. Editing the contract therefore did nothing until you restarted
 * the dev server.
 *
 * Only the regeneration is missing — Vite handles the rest on its own. Writing to
 * src/lib/api/ triggers a reload of the client modules and an HMR update of every
 * route that imports them, because those files ARE in the graph.
 *
 * Shells out to `pnpm generate:api` rather than calling openapi-ts directly so
 * there is one definition of how the client is generated (openapi-ts.config.ts),
 * shared with CI, the pre-commit hook and dev/regen-api.sh.
 */
function regenerateApiOnSpecChange(): Plugin {
  let running = false;
  let queued = false;

  return {
    name: "iron-temple:regenerate-api",
    // Dev server only: `vite build` gets its client from the build script's chain.
    apply: "serve",
    configureServer(server) {
      const { logger } = server.config;

      const regenerate = () => {
        // An editor can emit several change events for one save, and generation is
        // not instant. Collapse concurrent requests into one trailing re-run so the
        // final state always reflects the last write.
        if (running) {
          queued = true;
          return;
        }
        running = true;

        execFile("pnpm", ["generate:api"], { cwd: UI_ROOT }, (error) => {
          running = false;
          if (error) {
            // Usually a half-saved spec that isn't valid YAML yet. Report it and
            // wait for the next save rather than killing the dev server.
            logger.error(
              `[regenerate-api] openapi.yaml changed but generation failed:\n${error.message}`,
              { timestamp: true },
            );
          } else {
            logger.info("[regenerate-api] openapi.yaml changed - client regenerated", {
              timestamp: true,
            });
          }
          if (queued) {
            queued = false;
            regenerate();
          }
        });
      };

      server.watcher.add(OPENAPI_SPEC);
      server.watcher.on("change", (file) => {
        if (file === OPENAPI_SPEC) regenerate();
      });
    },
  };
}

export default defineConfig({
  plugins: [svelte(), tailwindcss(), regenerateApiOnSpecChange()],
  resolve: {
    // $lib alias for shadcn-svelte's generated components (Vite, not SvelteKit).
    alias: {
      $lib: fileURLToPath(new URL("./src/lib", import.meta.url)),
    },
    // Under Vitest, resolve Svelte's *browser* entry so component rendering
    // (mount/runes) works in jsdom. Guarded so the real dev/build stays untouched.
    ...(process.env.VITEST ? { conditions: ["browser"] } : {}),
  },
  server: {
    port: 5173,
    // Proxy API calls to the Go backend during development.
    proxy: {
      "/api": {
        target: "http://localhost:8080",
        changeOrigin: true,
      },
    },
  },
  test: {
    // jsdom so we can render components; globals for testing-library auto-cleanup.
    environment: "jsdom",
    globals: true,
    setupFiles: ["./vitest-setup.ts"],
    include: ["src/**/*.{test,spec}.ts"],
    coverage: {
      provider: "v8",
      reporter: ["text", "html", "lcov"],
      // First-party code only: the generated API client and the shadcn-svelte
      // UI primitives are vendored/generated, so they'd only dilute the signal.
      include: ["src/**/*.{ts,svelte}"],
      exclude: [
        "src/**/*.{test,spec}.ts",
        "src/lib/api/**",
        "src/lib/components/ui/**",
        "src/main.ts",
        "src/vite-env.d.ts",
        "**/*.d.ts",
      ],
    },
  },
});
