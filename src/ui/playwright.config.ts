import { defineConfig, devices } from "@playwright/test";

// The port the preview server binds, overridable via E2E_PORT.
//
// It is a variable rather than a constant because CI runs on a *shared host
// executor*, not a disposable container, and two things follow from that. A run
// cancelled mid-e2e — which `concurrency.cancel-in-progress` in ui.yml does on
// every superseded push — orphans this server, and the orphan holds its port for
// the life of the host. And two branches can run the UI workflow at the same
// time, since the concurrency group is keyed per ref.
//
// On a fixed port both are fatal: one cancelled run breaks every later run with
// "Port 5173 is already in use", on commits that had nothing to do with it, and
// two concurrent branches fight over the same socket. Giving each CI run its own
// port makes a leak inert — nothing ever waits on a port an orphan is holding.
// Local runs keep 5173, so `pnpm test:e2e` on a laptop is unchanged.
const PORT = Number(process.env.E2E_PORT) || 5173;
const BASE_URL = `http://localhost:${PORT}`;

// Run e2e on Firefox (the target browser). Firefox doesn't support Playwright's
// mobile device emulation, so we apply an iPad-landscape viewport to keep the
// design's tablet framing without the unsupported isMobile/touch flags.
export default defineConfig({
  testDir: "./e2e",
  fullyParallel: true,
  // One retry in CI, none locally.
  //
  // This is what makes `trace: "on-first-retry"` below mean anything: retries
  // default to 0, so there was never a first retry, so no trace was ever
  // recorded — the report uploaded by ui.yml's `if: failure()` step held a
  // failure and nothing to debug it with. Either the trace setting or the
  // retries had to move; retries are the cheaper of the two, because a retry
  // only costs time on a run that has already failed.
  //
  // Locally it stays 0, so a flake in front of you is a failure you can see
  // rather than one the second attempt hides.
  retries: process.env.CI ? 1 : 0,
  // `forbidOnly` in CI: a stray test.only() silently shrinks the suite to one
  // test and still reports green. Failing the run is the only way that gets
  // noticed before it is merged.
  forbidOnly: !!process.env.CI,
  use: {
    baseURL: BASE_URL,
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
    //
    // --strictPort stays: if the chosen port is somehow taken, failing loudly is
    // right. Silently drifting to the next free port would leave baseURL pointing
    // at nothing, and every test would fail on a timeout instead of a clear bind
    // error.
    command: `e2e/build-with-changelog.sh && pnpm preview --port ${PORT} --strictPort`,
    url: BASE_URL,
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
