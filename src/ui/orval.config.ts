import { defineConfig } from "orval";

// Generates the typed API client from the backend's OpenAPI spec.
// Run with: pnpm generate:api
//
// Output lands in src/lib/api/generated/, which is git-ignored and regenerated
// — see src/ui/.gitignore and dev/regen-api.sh for why it is not tracked. The
// barrel one level up (src/lib/api/index.ts) IS tracked, and is what the app
// imports; see the comment there.
export default defineConfig({
  ironTemple: {
    input: "../api/openapi.yaml",
    output: {
      target: "src/lib/api/generated/ironTemple.ts",
      schemas: "src/lib/api/generated/model",
      client: "fetch",
      mode: "split",
      // Every request goes through our own fetch rather than orval's, so that
      // the base URL and — more importantly — the offline case are defined in
      // exactly one place. See the file for what it guarantees.
      override: {
        mutator: {
          path: "src/lib/apiFetch.ts",
          name: "apiFetch",
        },
      },
    },
  },
});
