import { render, screen, waitFor } from "@testing-library/svelte";
import { beforeEach, describe, expect, it, vi } from "vitest";
import App from "./App.svelte";
import { auth } from "./lib/auth.svelte";
import { testUser } from "./lib/testFixtures";

// What the shell shows instead of the app.
//
// Three mutually exclusive states hang off one `{#if}` chain in App.svelte —
// signed out, owing a password change, and signed in — and the middle one is a
// security-shaped claim: an account the admin created must not reach a route by
// typing its hash. The API refuses it anyway (403 password_change_required),
// but a client that renders the router behind the prompt would paint a wall of
// failures and invite the question of what else it lets through. So the
// substitution is asserted, not assumed.
//
// Everything the shell starts on mount is stubbed. None of it is what this
// tests, and left real it would put the version poll and the write-queue retry
// on a timer inside the suite.
const loadMe = vi.hoisted(() => vi.fn());
vi.mock("./lib/auth.svelte", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./lib/auth.svelte")>()),
  loadMe,
}));
// Home is the landing route and mounts for real in the signed-in case below.
// It reads this itself, so the stub has to answer in the shape the client does
// — an empty history, which is the quietest thing the screen can be handed.
vi.mock("./lib/homeData", () => ({
  loadHomeSessions: vi.fn().mockResolvedValue({
    status: 200,
    data: { items: [], total: 0, totalVolumeLb: 0, limit: 100, offset: 0 },
  }),
}));
vi.mock("./lib/version.svelte", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./lib/version.svelte")>()),
  startPolling: vi.fn(() => () => {}),
}));
vi.mock("./lib/writeQueue.svelte", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./lib/writeQueue.svelte")>()),
  flush: vi.fn().mockResolvedValue(undefined),
  startQueueRetry: vi.fn(() => () => {}),
}));
vi.mock("./lib/connectivity.svelte", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./lib/connectivity.svelte")>()),
  watchConnectivity: vi.fn(() => () => {}),
}));
// The header and the update prompt are fetched as their own chunks at mount.
// Neither is what this file tests, and a dynamic import started on mount
// resolves after a short test has finished — which lands as an
// EnvironmentTeardownError, not as a failure anyone can act on. Stub the helper
// so they never begin; App renders its own placeholder for a header that has
// not arrived, which is exactly the state being stood in for.
vi.mock("./lib/deferred.svelte", () => ({
  deferred: () => ({ current: undefined, load: () => Promise.resolve() }),
}));

beforeEach(() => {
  vi.clearAllMocks();
  loadMe.mockResolvedValue(undefined);
  window.location.hash = "#/";
});

describe("App", () => {
  it("shows the sign-in form when signed out", async () => {
    auth.me = null;
    auth.loaded = true;

    render(App);

    expect(
      await screen.findByRole("heading", { name: /sign in|claim this install/i }),
    ).toBeInTheDocument();
  });

  // The blocking screen REPLACES the router rather than sitting over it, so
  // there is no route mounted behind it to reach.
  it("shows only the password screen to an account that owes a change", async () => {
    auth.me = testUser({ mustChangePassword: true });
    auth.loaded = true;

    render(App);

    expect(
      await screen.findByRole("heading", { name: /set your password/i }),
    ).toBeInTheDocument();
    // The nav is the way to every other screen, and it is gone too — being
    // unable to act on a tab is not the same as not being offered one.
    expect(screen.queryByRole("navigation")).not.toBeInTheDocument();
  });

  // Typing a hash must not get behind it either: the router is not mounted, so
  // there is nothing for the hash to route to.
  it("keeps the password screen when a route hash is typed", async () => {
    auth.me = testUser({ mustChangePassword: true });
    auth.loaded = true;
    window.location.hash = "#/admin";

    render(App);

    expect(
      await screen.findByRole("heading", { name: /set your password/i }),
    ).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: /accounts/i })).not.toBeInTheDocument();
  });

  it("renders the app for an account that owes nothing", async () => {
    auth.me = testUser();
    auth.loaded = true;

    render(App);

    await waitFor(() => expect(screen.getByRole("navigation")).toBeInTheDocument());
    expect(
      screen.queryByRole("heading", { name: /set your password/i }),
    ).not.toBeInTheDocument();
  });
});
