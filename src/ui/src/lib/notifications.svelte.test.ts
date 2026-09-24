import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  notifications,
  poll,
  resetNotifications,
  setLiveBacked,
  startPolling,
} from "./notifications.svelte";

// The notification poller, which is still the mechanism when the live socket is
// not — and the safety net when it is.
//
// Two things here are new and both are easy to get wrong: backing off while a
// socket is connected must not turn into "stopped", and the in-flight guard
// must not silently swallow the last request of a burst.

const listNotifications = vi.hoisted(() => vi.fn());
vi.mock("./api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./api")>()),
  listNotifications,
}));

function served(unreadCount = 0) {
  listNotifications.mockResolvedValue({
    status: 200,
    data: { items: [], limit: 20, offset: 0, unreadCount },
  });
}

function visible(state: "visible" | "hidden") {
  vi.spyOn(document, "visibilityState", "get").mockReturnValue(state);
}

let teardown: (() => void) | null = null;

beforeEach(() => {
  vi.useFakeTimers();
  vi.clearAllMocks();
  visible("visible");
  served();
  resetNotifications();
});

afterEach(() => {
  teardown?.();
  teardown = null;
  resetNotifications();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("backing off for a live socket", () => {
  // The poller does NOT stop when a socket connects. A proxy that eats
  // upgrades, or a socket the browser has quietly given up on, has to leave a
  // working badge behind.
  it("keeps its ordinary minute while nothing is watching", async () => {
    teardown = startPolling();
    await vi.advanceTimersByTimeAsync(0);
    expect(listNotifications).toHaveBeenCalledTimes(1);

    await vi.advanceTimersByTimeAsync(60_000);
    expect(listNotifications).toHaveBeenCalledTimes(2);
  });

  it("skips its ticks while a socket is connected", async () => {
    teardown = startPolling();
    await vi.advanceTimersByTimeAsync(0);
    listNotifications.mockClear();

    setLiveBacked(true);
    // Four minutes of ticks, all skipped: the socket is the mechanism now.
    await vi.advanceTimersByTimeAsync(4 * 60_000);
    expect(listNotifications).not.toHaveBeenCalled();
  });

  // The safety net actually fires. This is the assertion that separates
  // "backed off" from "stopped".
  it("still polls once the safety-net interval has passed", async () => {
    teardown = startPolling();
    await vi.advanceTimersByTimeAsync(0);
    listNotifications.mockClear();

    setLiveBacked(true);
    await vi.advanceTimersByTimeAsync(6 * 60_000);
    expect(listNotifications).toHaveBeenCalled();
  });

  it("returns to its ordinary minute when the socket goes", async () => {
    teardown = startPolling();
    await vi.advanceTimersByTimeAsync(0);
    setLiveBacked(true);
    await vi.advanceTimersByTimeAsync(2 * 60_000);
    listNotifications.mockClear();

    setLiveBacked(false);
    await vi.advanceTimersByTimeAsync(60_000);
    expect(listNotifications).toHaveBeenCalledTimes(1);
  });
});

describe("coalescing a burst", () => {
  // The in-flight guard used to DROP a concurrent request. Harmless while the
  // only caller was a once-a-minute timer; not harmless now that a burst of
  // pushes can arrive inside one request, because the last of them would be
  // the one swallowed and the badge would stay stale.
  it("polls again once for requests made while one was running", async () => {
    let finish!: (value: unknown) => void;
    listNotifications.mockReturnValueOnce(
      new Promise((resolve) => {
        finish = resolve;
      }),
    );

    const first = poll();
    // Three more while the first is still out.
    void poll();
    void poll();
    void poll();
    expect(listNotifications).toHaveBeenCalledTimes(1);

    served(3);
    finish({ status: 200, data: { items: [], limit: 20, offset: 0, unreadCount: 1 } });
    await first;
    await vi.advanceTimersByTimeAsync(0);

    // Exactly one catch-up for the three that were coalesced: they all asked
    // the same question.
    expect(listNotifications).toHaveBeenCalledTimes(2);
    expect(notifications.unread).toBe(3);
  });

  it("does not poll again when nothing asked during the request", async () => {
    await poll();
    await vi.advanceTimersByTimeAsync(0);
    expect(listNotifications).toHaveBeenCalledTimes(1);
  });
});
