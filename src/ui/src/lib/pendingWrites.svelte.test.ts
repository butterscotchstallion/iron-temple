import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import {
  track,
  whenIdle,
  pendingCount,
  resetPendingWrites,
} from "./pendingWrites.svelte";

/** A promise plus the handles to settle it from the test. */
function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

beforeEach(() => {
  resetPendingWrites();
});

afterEach(() => {
  vi.useRealTimers();
});

describe("track", () => {
  it("counts a write from dispatch to response", async () => {
    const write = deferred<string>();
    const tracked = track(write.promise);
    expect(pendingCount()).toBe(1);

    write.resolve("ok");
    await expect(tracked).resolves.toBe("ok");
    expect(pendingCount()).toBe(0);
  });

  // A failed write is still a finished one; leaving it counted would hang every
  // later whenIdle() for the lifetime of the page.
  it("stops counting a write that failed, and rethrows", async () => {
    const write = deferred<string>();
    const tracked = track(write.promise);

    write.reject(new Error("offline"));
    await expect(tracked).rejects.toThrow("offline");
    expect(pendingCount()).toBe(0);
  });

  it("counts concurrent writes independently", async () => {
    const a = deferred<void>();
    const b = deferred<void>();
    const trackedA = track(a.promise);
    const trackedB = track(b.promise);
    expect(pendingCount()).toBe(2);

    a.resolve();
    await trackedA;
    expect(pendingCount()).toBe(1);

    b.resolve();
    await trackedB;
    expect(pendingCount()).toBe(0);
  });
});

describe("whenIdle", () => {
  it("resolves immediately when nothing is in flight", async () => {
    await expect(whenIdle()).resolves.toBeUndefined();
  });

  it("waits for the last write to land", async () => {
    const write = deferred<void>();
    const tracked = track(write.promise);

    let idle = false;
    const waiting = whenIdle().then(() => (idle = true));

    // Give it a turn of the loop — it must still be waiting.
    await Promise.resolve();
    expect(idle).toBe(false);

    write.resolve();
    await tracked;
    await waiting;
    expect(idle).toBe(true);
  });

  it("waits for every outstanding write, not just the first", async () => {
    const a = deferred<void>();
    const b = deferred<void>();
    track(a.promise);
    track(b.promise);

    let idle = false;
    const waiting = whenIdle().then(() => (idle = true));

    a.resolve();
    await a.promise;
    await Promise.resolve();
    expect(idle).toBe(false);

    b.resolve();
    await b.promise;
    await waiting;
    expect(idle).toBe(true);
  });

  // The ceiling exists so a request that never settles can't strand the user in
  // a dialog whose only button never finishes.
  it("gives up after the timeout rather than hanging", async () => {
    vi.useFakeTimers();
    track(deferred<void>().promise); // never settles

    let idle = false;
    const waiting = whenIdle(5000).then(() => (idle = true));

    await vi.advanceTimersByTimeAsync(4999);
    expect(idle).toBe(false);

    await vi.advanceTimersByTimeAsync(1);
    await waiting;
    expect(idle).toBe(true);
  });

  it("clears its timeout when the writes land first", async () => {
    vi.useFakeTimers();
    const write = deferred<void>();
    const tracked = track(write.promise);
    const waiting = whenIdle(5000);

    write.resolve();
    await tracked;
    await waiting;

    // Nothing left armed — a stray timer here would keep the tab busy.
    expect(vi.getTimerCount()).toBe(0);
  });
});
