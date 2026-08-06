import { defineConfig, devices } from "@playwright/test";

// Run e2e on Firefox (the target browser). Firefox doesn't support Playwright's
// mobile device emulation, so we apply an iPad-landscape viewport to keep the
// design's tablet framing without the unsupported isMobile/touch flags.
export default defineConfig({
  testDir: "./e2e",
  fullyParallel: true,
  use: {
    baseURL: "http://localhost:5173",
    trace: "on-first-retry",
  },
  webServer: {
    command: "pnpm dev",
    url: "http://localhost:5173",
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
  },
  projects: [
    {
      name: "firefox",
      use: {
        ...devices["Desktop Firefox"],
        viewport: { width: 1080, height: 810 },
      },
    },
  ],
});
