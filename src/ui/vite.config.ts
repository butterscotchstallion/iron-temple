import { fileURLToPath } from "node:url";
import { defineConfig } from "vitest/config";
import { svelte } from "@sveltejs/vite-plugin-svelte";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
  plugins: [svelte(), tailwindcss()],
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
        // changeOrigin would rewrite Host to localhost:8080 while the browser
        // still sends Origin: http://localhost:5173, and the API's CSRF check
        // rejects a mutation whose Origin and Host disagree — so every POST in
        // development would 403. Leaving it off makes dev genuinely same-origin
        // from the API's point of view, which is what production is too
        // (Traefik path-routes /api and preserves Host). The target is a plain
        // Go server that ignores Host, so nothing needs the rewrite.
        changeOrigin: false,
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
