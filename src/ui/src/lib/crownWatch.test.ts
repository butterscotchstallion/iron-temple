import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { noteCrowns, resetCrownWatch } from "./crownWatch";
import { testAchievement, testAchievementHolders, testLifter } from "./testFixtures";

// The memory behind the celebration.
//
// Everything worth testing here is a case where it must stay QUIET. Announcing a
// crown twice, or announcing every crown a lifter already had the first time they
// open the app on a phone, turns the feature from a moment into noise — and the
// one that would be hardest to notice in review is the shared-browser case, where
// the second account inherits the first's set.

const ME = 1;
const THEM = 2;

/** The catalogue, with `holders` naming who wears each crown. */
function standings(entries: { slug: string; holders: number[] }[]) {
  return entries.map((e) =>
    testAchievementHolders({
      achievement: testAchievement({ slug: e.slug, label: `Top of ${e.slug}` }),
      holders: e.holders.map((id) => testLifter({ id })),
    }),
  );
}

beforeEach(() => {
  localStorage.clear();
  resetCrownWatch();
});
afterEach(() => {
  localStorage.clear();
  resetCrownWatch();
});

describe("the first observation", () => {
  // The whole reason this module exists rather than a plain comparison. Without
  // it, installing the app on a new phone congratulates you for everything you
  // already had.
  it("is silent, however much the lifter already holds", () => {
    const fresh = noteCrowns(ME, standings([
      { slug: "a", holders: [ME] },
      { slug: "b", holders: [ME] },
    ]));
    expect(fresh).toEqual([]);
  });

  it("still records what was held, so the next one can compare", () => {
    noteCrowns(ME, standings([{ slug: "a", holders: [ME] }]));
    const fresh = noteCrowns(ME, standings([
      { slug: "a", holders: [ME] },
      { slug: "b", holders: [ME] },
    ]));
    expect(fresh.map((a) => a.slug)).toEqual(["b"]);
  });
});

describe("what counts as news", () => {
  it("reports a crown that is newly theirs", () => {
    noteCrowns(ME, standings([{ slug: "a", holders: [THEM] }]));
    const fresh = noteCrowns(ME, standings([{ slug: "a", holders: [ME] }]));
    expect(fresh.map((a) => a.slug)).toEqual(["a"]);
  });

  it("says nothing about a crown they already held", () => {
    noteCrowns(ME, standings([{ slug: "a", holders: [ME] }]));
    expect(noteCrowns(ME, standings([{ slug: "a", holders: [ME] }]))).toEqual([]);
  });

  it("says nothing about somebody else's crown", () => {
    noteCrowns(ME, standings([]));
    expect(noteCrowns(ME, standings([{ slug: "a", holders: [THEM] }]))).toEqual([]);
  });

  // A crown describes a standing, so taking one back is genuinely news the second
  // time — this is not the repeat case the first-observation rule guards against.
  it("reports a crown lost and then retaken", () => {
    noteCrowns(ME, standings([{ slug: "a", holders: [ME] }]));
    expect(noteCrowns(ME, standings([{ slug: "a", holders: [THEM] }]))).toEqual([]);

    const again = noteCrowns(ME, standings([{ slug: "a", holders: [ME] }]));
    expect(again.map((a) => a.slug)).toEqual(["a"]);
  });

  // Ties share rank 1, so a crown can be shared — and holding half of one is still
  // holding it.
  it("reports a crown they share with somebody", () => {
    noteCrowns(ME, standings([{ slug: "a", holders: [THEM] }]));
    const fresh = noteCrowns(ME, standings([{ slug: "a", holders: [THEM, ME] }]));
    expect(fresh.map((a) => a.slug)).toEqual(["a"]);
  });

  it("carries the whole catalogue entry, so a caller can name what was won", () => {
    noteCrowns(ME, standings([]));
    const fresh = noteCrowns(ME, standings([{ slug: "a", holders: [ME] }]));
    expect(fresh[0].label).toBe("Top of a");
  });

  // Callers pass `auth.me?.id`. A signed-out reader holds nothing and is nobody to
  // congratulate.
  it("says nothing when there is no viewer", () => {
    expect(noteCrowns(undefined, standings([{ slug: "a", holders: [ME] }]))).toEqual([]);
  });
});

describe("across accounts and reloads", () => {
  // Survives a reload — that is the whole reason this is localStorage and not
  // memory. Simulated by dropping the in-memory mirror while leaving storage.
  it("does not re-announce after a reload", () => {
    noteCrowns(ME, standings([{ slug: "a", holders: [THEM] }]));
    const fresh = noteCrowns(ME, standings([{ slug: "a", holders: [ME] }]));
    expect(fresh).toHaveLength(1);

    // A reload: module state gone, storage intact. resetCrownWatch would clear
    // storage too, so the mirror is dropped by re-importing the behaviour instead —
    // noteCrowns reads storage whenever the viewer differs, so a different id and
    // back again is the same effect.
    noteCrowns(THEM, standings([]));

    expect(noteCrowns(ME, standings([{ slug: "a", holders: [ME] }]))).toEqual([]);
  });

  // TWO ACCOUNTS, ONE BROWSER. The second lifter must not be compared against the
  // first's crowns — that would either congratulate them for somebody else's or
  // hide one they genuinely just took.
  it("treats a different account as a first observation", () => {
    noteCrowns(ME, standings([{ slug: "a", holders: [ME] }]));
    const theirs = noteCrowns(THEM, standings([{ slug: "a", holders: [THEM] }]));
    expect(theirs).toEqual([]);
  });

  it("forgets everything on reset", () => {
    noteCrowns(ME, standings([{ slug: "a", holders: [ME] }]));
    expect(localStorage.getItem("iron-temple:crowns:v1")).not.toBeNull();

    resetCrownWatch();
    // Checked before observing again, because observing writes a new set — the
    // storage is only empty in the window between the two.
    expect(localStorage.getItem("iron-temple:crowns:v1")).toBeNull();

    // And back to a first observation, so silent even though the crown is held.
    expect(noteCrowns(ME, standings([{ slug: "a", holders: [ME] }]))).toEqual([]);
  });

  it("discards a stored set it cannot read", () => {
    localStorage.setItem("iron-temple:crowns:v1", "{not json");
    // A first observation rather than a throw.
    expect(noteCrowns(ME, standings([{ slug: "a", holders: [ME] }]))).toEqual([]);
  });

  it("discards a stored set from an older shape", () => {
    localStorage.setItem(
      "iron-temple:crowns:v1",
      JSON.stringify({ version: 0, viewerId: ME, slugs: ["a"] }),
    );
    expect(noteCrowns(ME, standings([{ slug: "a", holders: [ME] }]))).toEqual([]);
  });
});

// Safari private mode has historically thrown on access and an embedded webview
// can deny storage outright. Celebrating correctly for the life of the page is
// most of what this is for.
describe("without storage", () => {
  it("still works in memory", () => {
    const denied = vi.spyOn(window, "localStorage", "get").mockImplementation(() => {
      throw new Error("denied");
    });
    try {
      expect(noteCrowns(ME, standings([{ slug: "a", holders: [THEM] }]))).toEqual([]);
      const fresh = noteCrowns(ME, standings([{ slug: "a", holders: [ME] }]));
      expect(fresh.map((a) => a.slug)).toEqual(["a"]);
    } finally {
      denied.mockRestore();
    }
  });
});
