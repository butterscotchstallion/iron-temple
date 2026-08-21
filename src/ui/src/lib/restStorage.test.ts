import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { saveRest, loadRest, clearRest } from "./restStorage";

// A fixed "now" so the assertions read as durations rather than clock times.
const NOW = 1_700_000_000_000;

beforeEach(() => {
  sessionStorage.clear();
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("restStorage", () => {
  it("round-trips a running countdown as seconds remaining", () => {
    saveRest("s1", NOW + 90_000);
    expect(loadRest("s1", NOW)).toBe(90);
  });

  // The whole point of storing a deadline: time that passed with the tab shut
  // is rest that was taken.
  it("counts the time spent away", () => {
    saveRest("s1", NOW + 180_000);
    expect(loadRest("s1", NOW + 40_000)).toBe(140);
  });

  it("has nothing to restore when none was stored", () => {
    expect(loadRest("s1", NOW)).toBeNull();
  });

  // A rest that ran out while the tab was closed is over. Restoring it would
  // bring the timer back at a stopped 0:00 instead of a fresh 3:00.
  it("discards a countdown whose deadline has passed", () => {
    saveRest("s1", NOW);
    expect(loadRest("s1", NOW + 1)).toBeNull();
    // And drops it, so it isn't re-read on every mount.
    expect(sessionStorage.getItem("iron-temple:rest:s1")).toBeNull();
  });

  it("rounds part-seconds up, like the countdown on screen", () => {
    saveRest("s1", NOW + 400);
    expect(loadRest("s1", NOW)).toBe(1);
  });

  it("discards a value that isn't a number", () => {
    sessionStorage.setItem("iron-temple:rest:s1", "not-a-deadline");
    expect(loadRest("s1", NOW)).toBeNull();
    expect(sessionStorage.getItem("iron-temple:rest:s1")).toBeNull();
  });

  // Keys are per-session, so finishing one workout and starting another can't
  // inherit the first one's rest.
  it("keeps sessions apart", () => {
    saveRest("s1", NOW + 90_000);
    expect(loadRest("s2", NOW)).toBeNull();

    clearRest("s1");
    expect(loadRest("s1", NOW)).toBeNull();
  });

  // Safari's private mode throws on write, and a webview can deny storage
  // outright. Losing the countdown is acceptable; throwing at the lifter is not.
  it("degrades quietly when storage is unavailable", () => {
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("QuotaExceededError");
    });
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("SecurityError");
    });

    expect(() => saveRest("s1", NOW + 90_000)).not.toThrow();
    expect(loadRest("s1", NOW)).toBeNull();
  });
});
