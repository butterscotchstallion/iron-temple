import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { flushSync } from "svelte";
import { render, screen, fireEvent } from "@testing-library/svelte";
import RestTimer from "./RestTimer.svelte";

// Interval-driven countdown, so most tests drive time with fake timers. We
// switch to fake timers *after* render so component mount runs on real timers,
// then flushSync() after each advance to push reactive state into the DOM.
beforeEach(() => {
  sessionStorage.clear();
});

afterEach(() => {
  vi.useRealTimers();
});

const KEY = "iron-temple:rest:s1";

const remaining = () => screen.getByTestId("rest-remaining");
const startButton = () => screen.getByRole("button", { name: "Start" });
const resetButton = () => screen.getByRole("button", { name: "Reset" });

describe("RestTimer", () => {
  it("defaults to a 3-minute rest", () => {
    render(RestTimer);
    expect(remaining()).toHaveTextContent("3:00");
    expect(startButton()).toBeEnabled();
  });

  it("formats the initial time from the seconds prop", () => {
    render(RestTimer, { seconds: 65 });
    expect(remaining()).toHaveTextContent("1:05");
  });

  it("counts down once started and disables Start while running", async () => {
    render(RestTimer, { seconds: 5 });
    vi.useFakeTimers();

    await fireEvent.click(startButton());
    expect(startButton()).toBeDisabled();

    vi.advanceTimersByTime(1000);
    flushSync();
    expect(remaining()).toHaveTextContent("0:04");

    vi.advanceTimersByTime(2000);
    flushSync();
    expect(remaining()).toHaveTextContent("0:02");
  });

  it("ignores a second Start (no double interval)", async () => {
    render(RestTimer, { seconds: 5 });
    vi.useFakeTimers();

    await fireEvent.click(startButton());
    await fireEvent.click(startButton()); // guarded: running === true

    vi.advanceTimersByTime(1000);
    flushSync();
    // A doubled interval would have decremented twice (0:03).
    expect(remaining()).toHaveTextContent("0:04");
  });

  it("stops at zero and leaves Start disabled", async () => {
    render(RestTimer, { seconds: 2 });
    vi.useFakeTimers();

    await fireEvent.click(startButton());
    vi.advanceTimersByTime(2000);
    flushSync();

    expect(remaining()).toHaveTextContent("0:00");
    expect(startButton()).toBeDisabled();

    // Never goes negative even if the interval somehow fired again.
    vi.advanceTimersByTime(5000);
    flushSync();
    expect(remaining()).toHaveTextContent("0:00");
  });

  it("Reset restores the initial time and halts the countdown", async () => {
    render(RestTimer, { seconds: 30 });
    vi.useFakeTimers();

    await fireEvent.click(startButton());
    vi.advanceTimersByTime(3000);
    flushSync();
    expect(remaining()).toHaveTextContent("0:27");

    await fireEvent.click(resetButton());
    expect(remaining()).toHaveTextContent("0:30");
    expect(startButton()).toBeEnabled();

    // Reset stopped the interval, so time no longer advances.
    vi.advanceTimersByTime(5000);
    flushSync();
    expect(remaining()).toHaveTextContent("0:30");
  });

  it("auto-starts when the parent bumps autoStartKey", async () => {
    const { rerender } = render(RestTimer, { seconds: 10, autoStartKey: 0 });
    vi.useFakeTimers();

    await rerender({ autoStartKey: 1 });
    expect(startButton()).toBeDisabled(); // running after auto-start

    vi.advanceTimersByTime(2000);
    flushSync();
    expect(remaining()).toHaveTextContent("0:08");
  });

  it("restarts from the top on each autoStartKey bump", async () => {
    const { rerender } = render(RestTimer, { seconds: 10, autoStartKey: 0 });
    vi.useFakeTimers();

    await rerender({ autoStartKey: 1 });
    vi.advanceTimersByTime(4000);
    flushSync();
    expect(remaining()).toHaveTextContent("0:06");

    // A rep mid-rest starts the clock over rather than resuming it.
    await rerender({ autoStartKey: 2 });
    vi.advanceTimersByTime(1000);
    flushSync();
    expect(remaining()).toHaveTextContent("0:09");
  });

  // The countdown is a floating overlay, not a card in the page flow — the
  // active session unmounts it to stop it, so nothing may outlive the node.
  it("clears the interval when unmounted mid-countdown", async () => {
    const { unmount } = render(RestTimer, { seconds: 10 });
    vi.useFakeTimers();
    const clear = vi.spyOn(globalThis, "clearInterval");

    await fireEvent.click(startButton());
    unmount();

    expect(clear).toHaveBeenCalled();
    expect(vi.getTimerCount()).toBe(0);
  });
});

// Taking an update mid-workout reloads the page. Everything else in the session
// is already on the server; the countdown is the one thing that only exists
// here, so it's the one thing that has to be written down.
describe("RestTimer persistence", () => {
  it("stores a deadline while it runs, and drops it when stopped", async () => {
    render(RestTimer, { seconds: 60, storageKey: "s1" });
    expect(sessionStorage.getItem(KEY)).toBeNull(); // nothing to store yet

    await fireEvent.click(startButton());
    const endsAt = Number(sessionStorage.getItem(KEY));
    expect(endsAt).toBeGreaterThan(Date.now());

    await fireEvent.click(resetButton());
    expect(sessionStorage.getItem(KEY)).toBeNull();
  });

  it("resumes a countdown left running by the previous page load", () => {
    // 45 seconds still to go when the page came back.
    sessionStorage.setItem(KEY, String(Date.now() + 45_000));
    render(RestTimer, { seconds: 180, storageKey: "s1" });

    expect(remaining()).toHaveTextContent("0:45");
    // Already ticking — Start is the disabled control while running.
    expect(startButton()).toBeDisabled();
  });

  it("comes up fresh when the stored rest ran out while away", () => {
    sessionStorage.setItem(KEY, String(Date.now() - 1000));
    render(RestTimer, { seconds: 180, storageKey: "s1" });

    expect(remaining()).toHaveTextContent("3:00");
    expect(startButton()).toBeEnabled();
  });

  it("forgets the countdown once it reaches zero", async () => {
    render(RestTimer, { seconds: 2, storageKey: "s1" });
    vi.useFakeTimers();

    await fireEvent.click(startButton());
    expect(sessionStorage.getItem(KEY)).not.toBeNull();

    vi.advanceTimersByTime(2000);
    flushSync();
    expect(remaining()).toHaveTextContent("0:00");
    expect(sessionStorage.getItem(KEY)).toBeNull();
  });

  // Unmounting is navigation, not the lifter ending their rest — leaving the
  // session screen and coming back should find the clock still running. It is
  // also the guarantee that a reload can't erase the snapshot it restores from.
  it("keeps the stored countdown when unmounted mid-rest", async () => {
    const { unmount } = render(RestTimer, { seconds: 60, storageKey: "s1" });

    await fireEvent.click(startButton());
    unmount();

    expect(sessionStorage.getItem(KEY)).not.toBeNull();
  });

  // Without a key there is no session to belong to, so nothing is written.
  it("stays in memory when given no storage key", async () => {
    render(RestTimer, { seconds: 60 });
    await fireEvent.click(startButton());

    expect(sessionStorage.length).toBe(0);
  });
});
