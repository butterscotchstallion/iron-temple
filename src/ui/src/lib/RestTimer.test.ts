import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { flushSync } from "svelte";
import { render, screen, fireEvent } from "@testing-library/svelte";
import RestTimer from "./RestTimer.svelte";

// The alert is stubbed rather than exercised: what matters here is *when* the
// timer decides the rest is over, not what noise that makes. restAlert.test.ts
// covers the noise.
const alert = vi.hoisted(() => ({
  fire: vi.fn(),
  prime: vi.fn(),
  isMuted: vi.fn(() => false),
  setMuted: vi.fn(),
}));
vi.mock("./restAlert", () => alert);

// The countdown is driven off a deadline and painted by an interval, so most
// tests drive time with fake timers. We switch to fake timers *after* render so
// component mount runs on real timers, then flushSync() after each advance to
// push reactive state into the DOM.
beforeEach(() => {
  sessionStorage.clear();
  alert.fire.mockClear();
  alert.prime.mockClear();
  alert.setMuted.mockClear();
  alert.isMuted.mockReturnValue(false);
});

afterEach(() => {
  vi.useRealTimers();
});

const KEY = "iron-temple:rest:s1";

const remaining = () => screen.getByTestId("rest-remaining");
const startButton = () => screen.getByRole("button", { name: "Start" });
const resetButton = () => screen.getByRole("button", { name: "Reset" });
const skipButton = () => screen.getByRole("button", { name: "Skip" });
const plusButton = () => screen.getByRole("button", { name: "Add 30 seconds" });
const minusButton = () =>
  screen.getByRole("button", { name: "Subtract 30 seconds" });

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

  // The guard used to matter because a second interval meant a second
  // decrement. It still matters, for a different reason: a second start() would
  // stamp a fresh deadline and quietly hand back the seconds already rested.
  it("ignores a second Start (the deadline does not move)", async () => {
    render(RestTimer, { seconds: 5, storageKey: "s1" });
    vi.useFakeTimers();

    await fireEvent.click(startButton());
    const deadline = sessionStorage.getItem(KEY);

    vi.advanceTimersByTime(1000);
    flushSync();
    await fireEvent.click(startButton()); // guarded: running === true

    expect(sessionStorage.getItem(KEY)).toBe(deadline);
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
    expect(alert.fire).toHaveBeenCalledOnce();

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

  // A rest is prescribed per lift (migration 0011), so the prop changes as the
  // session moves from the squat to the accessory that follows it. The bump has
  // to pick up the new number, not restart the previous exercise's.
  it("counts down the new prescription when the lift changes", async () => {
    const { rerender } = render(RestTimer, { seconds: 300, autoStartKey: 0 });
    vi.useFakeTimers();

    await rerender({ seconds: 300, autoStartKey: 1 });
    expect(remaining()).toHaveTextContent("5:00");

    await rerender({ seconds: 90, autoStartKey: 2 });
    expect(remaining()).toHaveTextContent("1:30");
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

// A rest you have to hold your phone through is a rest you stop taking. These
// are the controls that make one usable one-handed, mid-set.
describe("RestTimer controls", () => {
  it("+30 buys thirty more seconds of real time", async () => {
    render(RestTimer, { seconds: 60 });
    vi.useFakeTimers();

    await fireEvent.click(startButton());
    vi.advanceTimersByTime(5000);
    flushSync();
    expect(remaining()).toHaveTextContent("0:55");

    await fireEvent.click(plusButton());
    flushSync();
    expect(remaining()).toHaveTextContent("1:25");

    // And it is genuinely on the clock, not just on the display.
    vi.advanceTimersByTime(5000);
    flushSync();
    expect(remaining()).toHaveTextContent("1:20");
  });

  it("−30 takes thirty off, and the stored deadline follows", async () => {
    render(RestTimer, { seconds: 120, storageKey: "s1" });
    vi.useFakeTimers();

    await fireEvent.click(startButton());
    const before = Number(sessionStorage.getItem(KEY));

    await fireEvent.click(minusButton());
    flushSync();
    expect(remaining()).toHaveTextContent("1:30");
    expect(Number(sessionStorage.getItem(KEY))).toBe(before - 30_000);
  });

  // Trimming the rest to nothing is the lifter saying they're done, not the
  // clock running out — so it ends the countdown without announcing it.
  it("−30 past zero ends the rest without a chime", async () => {
    render(RestTimer, { seconds: 20, storageKey: "s1" });
    vi.useFakeTimers();

    await fireEvent.click(startButton());
    await fireEvent.click(minusButton());
    flushSync();

    expect(remaining()).toHaveTextContent("0:00");
    expect(alert.fire).not.toHaveBeenCalled();
    expect(sessionStorage.getItem(KEY)).toBeNull();

    // And it stays quiet: a countdown left ticking at zero would reach the
    // chime on its very next pass.
    vi.advanceTimersByTime(5000);
    flushSync();
    expect(alert.fire).not.toHaveBeenCalled();
  });

  it("adjusts a stopped countdown without starting it", async () => {
    render(RestTimer, { seconds: 60 });
    vi.useFakeTimers();

    await fireEvent.click(plusButton());
    flushSync();

    expect(remaining()).toHaveTextContent("1:30");
    expect(startButton()).toBeEnabled(); // still stopped
    expect(vi.getTimerCount()).toBe(0);
  });

  // Skip is not Reset: the rest is over, so the clock reads zero rather than
  // going back to the top. Only the next logged rep restarts it.
  it("Skip ends the rest at zero, quietly", async () => {
    render(RestTimer, { seconds: 60, storageKey: "s1" });
    vi.useFakeTimers();

    await fireEvent.click(startButton());
    vi.advanceTimersByTime(5000);
    flushSync();

    await fireEvent.click(skipButton());
    flushSync();

    expect(remaining()).toHaveTextContent("0:00");
    expect(startButton()).toBeDisabled();
    expect(skipButton()).toBeDisabled();
    expect(alert.fire).not.toHaveBeenCalled();
    expect(sessionStorage.getItem(KEY)).toBeNull();
  });

  it("mutes and unmutes, remembering which", async () => {
    render(RestTimer, { seconds: 60 });
    const mute = () => screen.getByRole("button", { name: /the rest alert$/ });

    expect(mute()).toHaveAttribute("aria-pressed", "false");
    await fireEvent.click(mute());

    expect(alert.setMuted).toHaveBeenCalledWith(true);
    expect(mute()).toHaveAttribute("aria-pressed", "true");

    await fireEvent.click(mute());
    expect(alert.setMuted).toHaveBeenLastCalledWith(false);
    // Unmuting is a gesture, so it is also the moment to open the audio device.
    expect(alert.prime).toHaveBeenCalled();
  });
});

// A phone goes in a pocket the moment a set starts, and a backgrounded tab has
// its interval throttled — or suspended outright on a locked screen. The
// deadline is what survives that; the interval only paints.
describe("RestTimer while the tab is away", () => {
  function returnToTab() {
    document.dispatchEvent(new Event("visibilitychange"));
    flushSync();
  }

  it("recomputes from the deadline when the tab comes back", async () => {
    render(RestTimer, { seconds: 180 });
    vi.useFakeTimers();

    await fireEvent.click(startButton());
    // Wall-clock time passes with no interval firing at all — the worst case a
    // throttled tab produces.
    vi.setSystemTime(Date.now() + 45_000);
    returnToTab();

    expect(remaining()).toHaveTextContent("2:15");
  });

  it("finds a rest that ran out while away, and announces it", async () => {
    render(RestTimer, { seconds: 60, storageKey: "s1" });
    vi.useFakeTimers();

    await fireEvent.click(startButton());
    vi.setSystemTime(Date.now() + 90_000);
    returnToTab();

    expect(remaining()).toHaveTextContent("0:00");
    expect(alert.fire).toHaveBeenCalledOnce();
    expect(sessionStorage.getItem(KEY)).toBeNull();
  });

  // Nothing to resync when no rest is running, and a stray chime on a tab
  // switch would be worse than a stale one.
  it("stays quiet on a tab switch with the clock stopped", async () => {
    render(RestTimer, { seconds: 60 });
    vi.useFakeTimers();

    vi.setSystemTime(Date.now() + 90_000);
    returnToTab();

    expect(remaining()).toHaveTextContent("1:00");
    expect(alert.fire).not.toHaveBeenCalled();
  });
});
