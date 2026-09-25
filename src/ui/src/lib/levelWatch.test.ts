import { describe, it, expect, afterEach, beforeEach, vi } from "vitest";
import { noteLevel, resetLevelWatch } from "./levelWatch";

// Everything worth testing here is a case where it must stay QUIET.
//
// crownWatch.test.ts's charter, and for the same reason: the failure mode of a
// watch like this is not missing a celebration, it is celebrating something the
// lifter has had for a year — on every reload, on a shared browser, or after a
// failed request read as a collapse.

const ME = 1;
const THEM = 2;

beforeEach(() => {
  localStorage.clear();
  resetLevelWatch();
});

afterEach(() => {
  localStorage.clear();
  resetLevelWatch();
});

describe("the first observation", () => {
  it("says nothing, however high the level", () => {
    expect(noteLevel(ME, 27)).toBeNull();
  });

  // Silent, but not forgetful: the baseline has to be written or the NEXT
  // reading is a first observation too and nothing is ever announced.
  it("still records a baseline to diff against", () => {
    noteLevel(ME, 11);
    expect(noteLevel(ME, 12)).toBe(12);
  });
});

describe("what counts as news", () => {
  it("reports a level the lifter just reached", () => {
    noteLevel(ME, 11);
    expect(noteLevel(ME, 12)).toBe(12);
  });

  it("says nothing when the level has not moved", () => {
    noteLevel(ME, 12);
    expect(noteLevel(ME, 12)).toBeNull();
  });

  // Several levels can land between two readings — an offline queue draining, or
  // a poll that missed an afternoon — and where they got to is the whole of the
  // news. One report, naming the level reached rather than each one passed.
  it("reports only where they got to when several levels land at once", () => {
    noteLevel(ME, 11);
    expect(noteLevel(ME, 14)).toBe(14);
  });

  // A REAL case, not a defensive one: experience is derived from sessions, so
  // deleting one lowers it. Losing a level is not an occasion.
  it("says nothing when the level fell", () => {
    noteLevel(ME, 12);
    expect(noteLevel(ME, 11)).toBeNull();
  });

  // And the baseline follows it down, or the genuine re-crossing on the way back
  // up would be swallowed by a high-water mark nobody can see.
  it("moves the baseline down so the level can be reached again", () => {
    noteLevel(ME, 12);
    noteLevel(ME, 11);
    expect(noteLevel(ME, 12)).toBe(12);
  });

  it("says nothing when no viewer is named", () => {
    expect(noteLevel(undefined, 12)).toBeNull();
  });

  // The load-bearing half of that: a refresh with no viewer must not record a
  // baseline either, or a non-celebrating caller would silently eat the next
  // real level-up. loadLevels is called without an id from several places.
  it("records nothing when no viewer is named", () => {
    noteLevel(ME, 11);
    noteLevel(undefined, 12);
    expect(noteLevel(ME, 12)).toBe(12);
  });

  // A cold list is not a fall to level 1. `levelFor` gives null before the first
  // load and for a lifter the install does not carry, and neither is a reading.
  it("treats an unknown level as no reading at all", () => {
    noteLevel(ME, 12);
    expect(noteLevel(ME, null)).toBeNull();
    expect(noteLevel(ME, 12)).toBeNull();
  });
});

describe("across accounts and reloads", () => {
  // The reload case. Dropping the in-memory mirror while leaving storage alone is
  // what a fresh page load looks like — simulated by noting a different viewer,
  // which is what clears it.
  it("does not congratulate again after a reload", () => {
    noteLevel(ME, 11);
    expect(noteLevel(ME, 12)).toBe(12);

    noteLevel(THEM, 3);
    expect(noteLevel(ME, 12)).toBeNull();
  });

  // Two lifters sharing a browser. The second must not inherit the first's
  // level, in either direction: no congratulation for a level they already had,
  // and no silence about one they genuinely just reached.
  it("treats a different account as a first observation", () => {
    noteLevel(ME, 20);
    expect(noteLevel(THEM, 4)).toBeNull();
    expect(noteLevel(THEM, 5)).toBe(5);
  });

  it("returns to silence after a reset", () => {
    noteLevel(ME, 11);
    resetLevelWatch();

    expect(localStorage.getItem("iron-temple:level:v1")).toBeNull();
    expect(noteLevel(ME, 12)).toBeNull();
  });

  it("starts clean rather than throwing on unreadable storage", () => {
    localStorage.setItem("iron-temple:level:v1", "{not json");
    expect(noteLevel(ME, 12)).toBeNull();
    expect(noteLevel(ME, 13)).toBe(13);
  });

  it("discards a stored shape it does not recognise", () => {
    localStorage.setItem(
      "iron-temple:level:v1",
      JSON.stringify({ version: 0, viewerId: ME, level: 11 }),
    );
    expect(noteLevel(ME, 12)).toBeNull();
  });

  it("discards a stored level that is not a number", () => {
    localStorage.setItem(
      "iron-temple:level:v1",
      JSON.stringify({ version: 1, viewerId: ME, level: "eleven" }),
    );
    expect(noteLevel(ME, 12)).toBeNull();
  });
});

// Safari private mode has thrown on access, and an embedded webview can deny
// storage outright. Celebrating correctly for the life of the page is most of
// what this is for, so it still has to work — it just forgets across a reload.
describe("without storage", () => {
  it("still diffs in memory", () => {
    vi.spyOn(window, "localStorage", "get").mockImplementation(() => {
      throw new Error("denied");
    });

    noteLevel(ME, 11);
    expect(noteLevel(ME, 12)).toBe(12);
    expect(noteLevel(ME, 12)).toBeNull();

    vi.restoreAllMocks();
  });
});
