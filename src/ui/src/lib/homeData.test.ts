import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { SessionList, SessionSummary } from "./api";
import { clearCache } from "./cache.svelte";
import { resetHomeWatch, watchHomeSessions } from "./homeData";

// The poller behind the main screen staying current across devices.
//
// What is worth pinning here is not that an interval fires — it is the four
// guards around it, each of which is a request somebody would otherwise pay for
// and none of which is visible from a component test: one request however many
// screens are watching, silence while the tab is hidden, a floor under refocus,
// and an interval that actually stops when the last watcher goes.

const listSessions = vi.hoisted(() => vi.fn());
vi.mock("./api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./api")>()),
  listSessions,
}));

function summary(over: Partial<SessionSummary> = {}): SessionSummary {
  return {
    id: 1,
    programId: 1,
    programName: "StrongLifts 5x5",
    programDayId: 7,
    programDayName: "Workout A",
    performedOn: "2026-09-23",
    setCount: 15,
    completedSetCount: 15,
    volumeLb: 8450,
    isOver: true,
    exercises: [],
    ...over,
  };
}

function list(items: SessionSummary[] = [summary()]): SessionList {
  return { items, total: items.length, totalVolumeLb: 0, limit: 100, offset: 0 };
}

const ok = (data: unknown, status = 200) => ({ status, data, headers: new Headers() });

/** jsdom reports "visible" and has no way to set it, so the getter is replaced. */
function setVisibility(state: DocumentVisibilityState) {
  Object.defineProperty(document, "visibilityState", {
    configurable: true,
    get: () => state,
  });
}

beforeEach(() => {
  vi.useFakeTimers();
  listSessions.mockReset();
  listSessions.mockResolvedValue(ok(list()));
  clearCache();
  resetHomeWatch();
  setVisibility("visible");
});

afterEach(() => {
  resetHomeWatch();
  vi.useRealTimers();
});

describe("watchHomeSessions", () => {
  it("hands every watcher the same list off one request", async () => {
    const home = vi.fn();
    const program = vi.fn();
    watchHomeSessions(home);
    watchHomeSessions(program);

    await vi.advanceTimersByTimeAsync(31_000);

    // The point of the shared interval: Home and ProgramDetail are on screen
    // together, and two pollers would be two requests asking one question.
    expect(listSessions).toHaveBeenCalledTimes(1);
    expect(home).toHaveBeenCalledWith(list());
    expect(program).toHaveBeenCalledWith(list());
  });

  it("asks for nothing while the tab is hidden", async () => {
    watchHomeSessions(vi.fn());
    setVisibility("hidden");

    await vi.advanceTimersByTimeAsync(91_000);

    expect(listSessions).not.toHaveBeenCalled();
  });

  // Walking from the rack back to the desk. This is the case the whole thing is
  // for, and waiting out the interval would be the wrong answer to it.
  it("reads at once when the tab is brought back", async () => {
    const watcher = vi.fn();
    watchHomeSessions(watcher);

    setVisibility("hidden");
    document.dispatchEvent(new Event("visibilitychange"));
    setVisibility("visible");
    document.dispatchEvent(new Event("visibilitychange"));
    await vi.advanceTimersByTimeAsync(0);

    expect(listSessions).toHaveBeenCalledTimes(1);
    expect(watcher).toHaveBeenCalledWith(list());
  });

  it("does not read again for a flick between tabs", async () => {
    watchHomeSessions(vi.fn());

    document.dispatchEvent(new Event("visibilitychange"));
    await vi.advanceTimersByTimeAsync(0);
    expect(listSessions).toHaveBeenCalledTimes(1);

    // Inside REFOCUS_MIN_GAP_MS: a second flick is not a second request.
    await vi.advanceTimersByTimeAsync(5_000);
    document.dispatchEvent(new Event("visibilitychange"));
    await vi.advanceTimersByTimeAsync(0);
    expect(listSessions).toHaveBeenCalledTimes(1);

    // Past it, and it asks again.
    await vi.advanceTimersByTimeAsync(11_000);
    document.dispatchEvent(new Event("visibilitychange"));
    await vi.advanceTimersByTimeAsync(0);
    expect(listSessions).toHaveBeenCalledTimes(2);
  });

  // Navigating off the main screen has to actually stop it, or every route the
  // lifter visits afterwards keeps paying for a badge nobody is looking at.
  it("stops once the last watcher goes, and not before", async () => {
    const home = vi.fn();
    const program = vi.fn();
    const stopHome = watchHomeSessions(home);
    const stopProgram = watchHomeSessions(program);

    stopHome();
    await vi.advanceTimersByTimeAsync(31_000);
    expect(listSessions).toHaveBeenCalledTimes(1);
    expect(home).not.toHaveBeenCalled();
    expect(program).toHaveBeenCalledTimes(1);

    stopProgram();
    await vi.advanceTimersByTimeAsync(91_000);
    expect(listSessions).toHaveBeenCalledTimes(1);

    // The refocus listener goes with it, or a hidden tab's watcher would keep
    // reading for the life of the page.
    document.dispatchEvent(new Event("visibilitychange"));
    await vi.advanceTimersByTimeAsync(0);
    expect(listSessions).toHaveBeenCalledTimes(1);
  });

  // A revalidation that didn't land is not news. Blanking a streak over a
  // network blip would be strictly worse than showing a slightly stale one.
  it("leaves watchers holding what they had when the read fails", async () => {
    const watcher = vi.fn();
    watchHomeSessions(watcher);

    listSessions.mockResolvedValue(ok(undefined, 0));
    await vi.advanceTimersByTimeAsync(31_000);

    expect(listSessions).toHaveBeenCalledTimes(1);
    expect(watcher).not.toHaveBeenCalled();
  });
});
