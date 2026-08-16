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
