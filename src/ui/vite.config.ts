import { execFile } from "node:child_process";
import { createHash } from "node:crypto";
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname } from "node:path";
import { fileURLToPath } from "node:url";
import type { Plugin } from "vite";
import { defineConfig } from "vitest/config";
import { svelte } from "@sveltejs/vite-plugin-svelte";
import tailwindcss from "@tailwindcss/vite";

const UI_ROOT = fileURLToPath(new URL(".", import.meta.url));
const OPENAPI_SPEC = fileURLToPath(new URL("../api/openapi.yaml", import.meta.url));
const GENERATED_CLIENT = fileURLToPath(new URL("./src/lib/api", import.meta.url));

// Shared with dev/regen-api.sh — same path, same digest, so the hook and the dev
// server never disagree about whether the client is current. The shell side hashes
// via `sha256sum < spec`, which keeps the filename out of the digest and makes the
// two implementations produce identical hex.
const SPEC_STAMP = fileURLToPath(
  new URL("./node_modules/.cache/iron-temple/openapi-spec.sha256", import.meta.url),
);

function hashSpec(): string | null {
  try {
    return createHash("sha256").update(readFileSync(OPENAPI_SPEC)).digest("hex");
  } catch {
    return null;
  }
}

function readStamp(): string | null {
  try {
    return readFileSync(SPEC_STAMP, "utf8").trim();
  } catch {
    return null;
  }
}

function writeStamp(hash: string): void {
  try {
    mkdirSync(dirname(SPEC_STAMP), { recursive: true });
    writeFileSync(SPEC_STAMP, `${hash}\n`);
  } catch {
    // A missing stamp only costs a redundant regeneration, so this is not worth
    // interrupting the dev server over.
  }
}

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
 * That reload is also why this compares content hashes rather than regenerating on
 * every save: the generated client is plain .ts with no HMR accept handler, so any
 * rewrite costs a FULL PAGE RELOAD and the in-page state that goes with it. A save
 * that leaves the spec byte-identical — the reflexive ctrl-s, a formatter that
 * changed nothing — must not cost you that.
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

        const hash = hashSpec();
        // Unreadable mid-write: the editor's next event will bring us back.
        if (hash === null) return;
        // Byte-identical to what the client was last generated from, and that client
        // is still on disk. Nothing to do, and no page reload inflicted.
        if (hash === readStamp() && existsSync(GENERATED_CLIENT)) return;

        running = true;
        execFile("pnpm", ["generate:api"], { cwd: UI_ROOT }, (error) => {
          running = false;
          if (error) {
            // Usually a half-saved spec that isn't valid YAML yet. Report it and
            // wait for the next save rather than killing the dev server. The stamp
            // is deliberately left alone, so the next valid save regenerates.
            logger.error(
              `[regenerate-api] openapi.yaml changed but generation failed:\n${error.message}`,
              { timestamp: true },
            );
          } else {
            writeStamp(hash);
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
