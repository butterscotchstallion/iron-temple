import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { currentTime, resetClock, watchClock } from "./now.svelte";

beforeEach(() => {
  resetClock();
});

afterEach(() => {
  resetClock();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("currentTime", () => {
  it("reads the real clock before anything has subscribed", () => {
    // A component may read this during its first render, before the $effect that
    // subscribes has run, so it can never be zero or undefined.
    expect(currentTime()).toBeGreaterThan(0);
  });
});

describe("watchClock", () => {
  it("advances the clock while something is watching", () => {
    vi.useFakeTimers();
    const stop = watchClock();

    const before = currentTime();
    vi.advanceTimersByTime(30_000);
    expect(currentTime()).toBeGreaterThan(before);

    stop();
  });

  it("runs no timer once the last watcher has gone", () => {
    vi.useFakeTimers();
    watchClock()();

    const after = currentTime();
    vi.advanceTimersByTime(120_000);
    // Nothing is displaying a timestamp, so nothing should be waking up to
    // recompute one. A screen with no dates on it is most screens in this app.
    expect(currentTime()).toBe(after);
  });

  it("keeps ticking for the watchers that remain", () => {
    vi.useFakeTimers();
    const first = watchClock();
    const second = watchClock();

    first();
    const before = currentTime();
    vi.advanceTimersByTime(30_000);
    expect(currentTime()).toBeGreaterThan(before);

    second();
  });

  it("ignores a teardown run twice", () => {
    vi.useFakeTimers();
    const first = watchClock();
    const second = watchClock();

    // The refcount is what keeps the timer honest. A teardown counted twice
    // would drop it to zero while `second` is still watching, and stop the clock
    // for a component that is still on screen.
    first();
    first();

    const before = currentTime();
    vi.advanceTimersByTime(30_000);
    expect(currentTime()).toBeGreaterThan(before);

    second();
  });

  it("catches up the moment the tab comes back", () => {
    // A backgrounded tab has its interval throttled hard, so the interval alone
    // cannot be trusted to have fired — this is the phone-in-a-pocket case.
    vi.useFakeTimers();
    const stop = watchClock();

    const before = currentTime();
    vi.setSystemTime(new Date(before + 3_600_000));
    document.dispatchEvent(new Event("visibilitychange"));

    expect(currentTime()).toBeGreaterThanOrEqual(before + 3_600_000);
    stop();
  });

  it("stops listening for visibility once nothing is watching", () => {
    vi.useFakeTimers();
    watchClock()();

    const after = currentTime();
    vi.setSystemTime(new Date(after + 3_600_000));
    document.dispatchEvent(new Event("visibilitychange"));

    expect(currentTime()).toBe(after);
  });
});
