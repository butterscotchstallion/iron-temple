import { describe, it, expect, afterEach, vi } from "vitest";
import { flushSync } from "svelte";
import { render, screen, fireEvent } from "@testing-library/svelte";
import RestTimer from "./RestTimer.svelte";

// Interval-driven countdown, so most tests drive time with fake timers. We
// switch to fake timers *after* render so component mount runs on real timers,
// then flushSync() after each advance to push reactive state into the DOM.
afterEach(() => {
  vi.useRealTimers();
});

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

  it("resets without starting when the parent bumps resetKey", async () => {
    const { rerender } = render(RestTimer, { seconds: 10 });
    vi.useFakeTimers();

    await fireEvent.click(startButton());
    vi.advanceTimersByTime(4000);
    flushSync();
    expect(remaining()).toHaveTextContent("0:06");

    await rerender({ resetKey: 1 });
    expect(remaining()).toHaveTextContent("0:10");
    expect(startButton()).toBeEnabled(); // reset only — not running

    vi.advanceTimersByTime(3000);
    flushSync();
    expect(remaining()).toHaveTextContent("0:10");
  });
});
