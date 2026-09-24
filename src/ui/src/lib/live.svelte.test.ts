import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { FakeWebSocket } from "../../test-support/fakeWebSocket";
import { live, resetLive, startLive } from "./live.svelte";
import { notifications } from "./notifications.svelte";

// The live socket's client half.
//
// The things worth asserting here are the ones that are wrong in the obvious
// implementation: that "connected" means the WELCOME frame and not `onopen`,
// that every connect does a full refetch, that a reconnect loop does not run in
// a background tab, and that the teardown leaves no timer behind.

const listNotifications = vi.hoisted(() => vi.fn());
vi.mock("./api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./api")>()),
  listNotifications,
}));

/** An empty panel, which is all these tests need the endpoint to say. */
function served() {
  listNotifications.mockResolvedValue({
    status: 200,
    data: { items: [], limit: 20, offset: 0, unreadCount: 0 },
  });
}

function visible(state: "visible" | "hidden") {
  vi.spyOn(document, "visibilityState", "get").mockReturnValue(state);
}

let teardown: (() => void) | null = null;

beforeEach(() => {
  vi.useFakeTimers();
  vi.clearAllMocks();
  FakeWebSocket.reset();
  vi.stubGlobal("WebSocket", FakeWebSocket);
  visible("visible");
  served();
  // Module state outlives a test, which is the point of it — so each one starts
  // from a known socket rather than the previous one's.
  resetLive();
  notifications.unread = 0;
});

afterEach(() => {
  teardown?.();
  teardown = null;
  resetLive();
  vi.unstubAllGlobals();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

function start() {
  teardown = startLive();
}

describe("connecting", () => {
  it("opens a same-origin socket at the API's own path", () => {
    start();
    expect(FakeWebSocket.last.url).toBe(`ws://${window.location.host}/api/v1/live`);
  });

  // The wss branch is not covered here: jsdom refuses to redefine
  // window.location.protocol, and the alternatives — replacing the whole
  // location object, or exporting socketURL purely so a test can pass it a
  // scheme — cost more in contortion than the one-line conditional is worth.
  // It is `protocol === "https:" ? "wss:" : "ws:"` and nothing else.

  // A socket whose transport opened but whose server side never said anything
  // is not a working connection, and treating it as one would quiet the poller
  // while nothing was arriving.
  it("is not connected until the welcome frame", () => {
    start();
    FakeWebSocket.last.open();
    expect(live.connected).toBe(false);

    FakeWebSocket.last.emit({ type: "welcome", protocol: 1 });
    expect(live.connected).toBe(true);
  });

  // The refetch that makes a connect a complete repair: anything that happened
  // while this tab was away is picked up rather than waited for.
  it("refetches the panel on every connect", async () => {
    start();
    FakeWebSocket.last.welcome();
    await vi.advanceTimersByTimeAsync(0);
    expect(listNotifications).toHaveBeenCalledTimes(1);
  });
});

describe("receiving", () => {
  beforeEach(async () => {
    start();
    FakeWebSocket.last.welcome();
    await vi.advanceTimersByTimeAsync(0);
    listNotifications.mockClear();
  });

  it("refetches when told the panel changed", async () => {
    FakeWebSocket.last.emit({ type: "notification" });
    await vi.advanceTimersByTimeAsync(0);
    expect(listNotifications).toHaveBeenCalledTimes(1);
  });

  // The server may be newer than this tab — a deploy does not close every open
  // socket — so an unrecognised frame is a thing to skip, not a failure.
  it("ignores a frame it does not recognise", async () => {
    FakeWebSocket.last.emit({ type: "something-from-the-future" });
    await vi.advanceTimersByTimeAsync(0);
    expect(listNotifications).not.toHaveBeenCalled();
  });

  it("survives a frame that is not JSON", async () => {
    FakeWebSocket.last.emitRaw("<html>a proxy error page</html>");
    await vi.advanceTimersByTimeAsync(0);
    expect(listNotifications).not.toHaveBeenCalled();
    // Still working.
    FakeWebSocket.last.emit({ type: "notification" });
    await vi.advanceTimersByTimeAsync(0);
    expect(listNotifications).toHaveBeenCalledTimes(1);
  });
});

describe("reconnecting", () => {
  it("backs off and eventually reconnects", async () => {
    start();
    FakeWebSocket.last.welcome();
    await vi.advanceTimersByTimeAsync(0);
    expect(FakeWebSocket.instances).toHaveLength(1);

    FakeWebSocket.last.fail();
    expect(live.connected).toBe(false);

    // Nothing immediately: the whole point of a backoff.
    await vi.advanceTimersByTimeAsync(100);
    expect(FakeWebSocket.instances).toHaveLength(1);

    // The first step is a second, plus jitter.
    await vi.advanceTimersByTimeAsync(2000);
    expect(FakeWebSocket.instances).toHaveLength(2);
  });

  it("climbs the backoff when reconnects keep failing", async () => {
    start();
    FakeWebSocket.last.welcome();
    await vi.advanceTimersByTimeAsync(0);

    FakeWebSocket.last.fail();
    await vi.advanceTimersByTimeAsync(2000);
    expect(FakeWebSocket.instances).toHaveLength(2);

    // Second failure: the next attempt is further out, so the same short
    // advance is no longer enough to produce one.
    FakeWebSocket.last.fail();
    await vi.advanceTimersByTimeAsync(1000);
    expect(FakeWebSocket.instances).toHaveLength(2);
    await vi.advanceTimersByTimeAsync(2000);
    expect(FakeWebSocket.instances).toHaveLength(3);
  });

  it("resets the backoff once a connection welcomes it", async () => {
    start();
    FakeWebSocket.last.welcome();
    await vi.advanceTimersByTimeAsync(0);

    FakeWebSocket.last.fail();
    await vi.advanceTimersByTimeAsync(2000);
    FakeWebSocket.last.fail();
    await vi.advanceTimersByTimeAsync(4000);
    FakeWebSocket.last.welcome();
    await vi.advanceTimersByTimeAsync(0);

    // Back to the first step.
    FakeWebSocket.last.fail();
    await vi.advanceTimersByTimeAsync(2000);
    expect(FakeWebSocket.instances).toHaveLength(4);
  });

  // A reconnect loop in a background tab is waste: nobody is looking at the
  // badge, and the poller is covering it anyway.
  it("does not reconnect while the tab is hidden", async () => {
    start();
    FakeWebSocket.last.welcome();
    await vi.advanceTimersByTimeAsync(0);

    visible("hidden");
    FakeWebSocket.last.fail();
    await vi.advanceTimersByTimeAsync(30_000);
    expect(FakeWebSocket.instances).toHaveLength(1);
  });

  // Coming back to a tab is when a socket is most likely to have died
  // unnoticed — a laptop that slept has a connection the browser gave up on.
  it("reconnects as soon as the tab comes back", async () => {
    start();
    FakeWebSocket.last.welcome();
    await vi.advanceTimersByTimeAsync(0);

    visible("hidden");
    FakeWebSocket.last.fail();
    await vi.advanceTimersByTimeAsync(30_000);
    expect(FakeWebSocket.instances).toHaveLength(1);

    visible("visible");
    document.dispatchEvent(new Event("visibilitychange"));
    expect(FakeWebSocket.instances).toHaveLength(2);
  });
});

describe("tearing down", () => {
  it("closes the socket and leaves no reconnect behind", async () => {
    start();
    FakeWebSocket.last.welcome();
    await vi.advanceTimersByTimeAsync(0);
    const ws = FakeWebSocket.last;

    teardown?.();
    teardown = null;

    expect(ws.closed).toBe(1);
    expect(live.connected).toBe(false);

    // The leak test: a close after teardown must not schedule anything.
    ws.fail();
    await vi.advanceTimersByTimeAsync(60_000);
    expect(FakeWebSocket.instances).toHaveLength(1);
  });

  it("stops a pending reconnect", async () => {
    start();
    FakeWebSocket.last.welcome();
    await vi.advanceTimersByTimeAsync(0);
    FakeWebSocket.last.fail();

    teardown?.();
    teardown = null;

    await vi.advanceTimersByTimeAsync(60_000);
    expect(FakeWebSocket.instances).toHaveLength(1);
  });

  // Sign-out. The socket was opened as the account that is leaving.
  it("resetLive closes whatever is open", async () => {
    start();
    FakeWebSocket.last.welcome();
    await vi.advanceTimersByTimeAsync(0);
    const ws = FakeWebSocket.last;

    resetLive();

    expect(ws.closed).toBe(1);
    expect(live.connected).toBe(false);
  });
});
