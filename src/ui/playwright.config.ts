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
    // Serve the production build (vite preview), not the dev server. Preview serves
    // pre-built static assets, so pages load fast and deterministically — no on-demand
    // vite module transforms. On the CPU-limited CI runner, `vite dev` compiled the app
    // on first request slower than page.goto's 30s timeout, hanging the "load" event.
    // The build here also doubles as the production-build check.
    //
    // It goes through build-with-changelog.sh rather than calling `pnpm build`
    // directly because the header's changelog panel is fed at build time and so can't
    // be mocked per-test; see that script for why.
    command: "e2e/build-with-changelog.sh && pnpm preview --port 5173 --strictPort",
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
