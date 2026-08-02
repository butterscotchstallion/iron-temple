import { defineConfig } from "@hey-api/openapi-ts";

// Generates the typed API client from the backend's OpenAPI spec.
// Run with: pnpm generate:api
export default defineConfig({
  input: "../api/openapi.yaml",
  output: "src/lib/api",
  plugins: ["@hey-api/client-fetch"],
});
