import { defineConfig, devices } from "@playwright/test";

// Mobile-first, targeting iPad per the design.
export default defineConfig({
  testDir: "./e2e",
  fullyParallel: true,
  use: {
    baseURL: "http://localhost:5173",
    trace: "on-first-retry",
  },
  webServer: {
    command: "npm run dev",
    url: "http://localhost:5173",
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
  },
  projects: [
    {
      name: "ipad",
      use: { ...devices["iPad (gen 7) landscape"] },
    },
  ],
});
