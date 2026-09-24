import { describe, it, expect, afterEach, beforeEach, vi } from "vitest";
import {
  achievementLabel,
  achievements,
  crownsFor,
  loadAchievements,
  resetAchievements,
} from "./achievements.svelte";
import { testAchievement, testAchievementHolders, testLifter } from "./testFixtures";

const getAchievements = vi.hoisted(() => vi.fn());

// The barrel, never the generated files — mocking those would pin the tests to
// orval's output shape rather than to the contract.
vi.mock("./api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./api")>()),
  getAchievements,
}));

/** A 200 carrying these entries. */
function served(items: ReturnType<typeof testAchievementHolders>[]) {
  getAchievements.mockResolvedValue({ status: 200, data: { items } });
}

beforeEach(() => {
  getAchievements.mockReset();
  resetAchievements();
});
afterEach(() => resetAchievements());

describe("loading", () => {
  it("keeps what the server said", async () => {
    const entry = testAchievementHolders({ holders: [testLifter({ id: 7 })] });
    served([entry]);

    await loadAchievements();

    expect(achievements.loaded).toBe(true);
    expect(achievements.items).toEqual([entry]);
  });

  // A crown is an ornament. A lifter whose fetch failed sees names without
  // crowns, which is what they saw before the feature existed — so a failure
  // must not clear what is already on screen, and must not claim to have loaded.
  it("leaves the last good standings in place on a failure", async () => {
    served([testAchievementHolders({ holders: [testLifter({ id: 7 })] })]);
    await loadAchievements();

    getAchievements.mockResolvedValue({ status: 500, data: {} });
    await loadAchievements();

    expect(achievements.items).toHaveLength(1);
    expect(crownsFor(7)).toHaveLength(1);
  });

  it("does not report loaded when the first attempt failed", async () => {
    getAchievements.mockResolvedValue({ status: 500, data: {} });
    await loadAchievements();
    expect(achievements.loaded).toBe(false);
  });
});

describe("crownsFor", () => {
  it("returns nothing for a lifter holding nothing", async () => {
    served([testAchievementHolders({ holders: [testLifter({ id: 7 })] })]);
    await loadAchievements();
    expect(crownsFor(1)).toEqual([]);
  });

  // Callers pass `auth.me?.id`, so undefined arrives here routinely and is not
  // an error — a signed-out reader wears no crown.
  it("treats an unknown lifter as holding nothing", () => {
    expect(crownsFor(undefined)).toEqual([]);
  });

  it("gathers every board one lifter leads", async () => {
    served([
      testAchievementHolders({ holders: [testLifter({ id: 7 })] }),
      testAchievementHolders({
        achievement: testAchievement({ slug: "crown-volume", label: "Top of Volume" }),
        holders: [testLifter({ id: 7 })],
      }),
    ]);
    await loadAchievements();

    expect(crownsFor(7).map((a) => a.slug)).toEqual(["crown-streak", "crown-volume"]);
  });

  // Tied figures share rank 1, so one board can have several holders and each of
  // them holds it — not just the first.
  it("gives a tied board's crown to all of its holders", async () => {
    served([
      testAchievementHolders({
        holders: [testLifter({ id: 7 }), testLifter({ id: 8 })],
      }),
    ]);
    await loadAchievements();

    expect(crownsFor(7)).toHaveLength(1);
    expect(crownsFor(8)).toHaveLength(1);
  });

  // The map is rebuilt per load, so a crown that changed hands must not leave the
  // previous holder still wearing it.
  it("drops a crown from the lifter who lost it", async () => {
    served([testAchievementHolders({ holders: [testLifter({ id: 7 })] })]);
    await loadAchievements();
    expect(crownsFor(7)).toHaveLength(1);

    served([testAchievementHolders({ holders: [testLifter({ id: 8 })] })]);
    await loadAchievements();

    expect(crownsFor(7)).toEqual([]);
    expect(crownsFor(8)).toHaveLength(1);
  });
});

describe("achievementLabel", () => {
  it("names a slug it knows", async () => {
    served([testAchievementHolders()]);
    await loadAchievements();
    expect(achievementLabel("crown-streak")).toBe("Top of Week streak");
  });

  // Both fall back to null so the notification panel says the unnamed thing
  // rather than rendering a blank where a board name should be.
  it("returns null for a slug it does not know", async () => {
    served([testAchievementHolders()]);
    await loadAchievements();
    expect(achievementLabel("crown-nonsense")).toBeNull();
  });

  it("returns null before the catalogue has loaded", () => {
    expect(achievementLabel("crown-streak")).toBeNull();
  });

  it("returns null for an absent slug", () => {
    expect(achievementLabel(undefined)).toBeNull();
  });
});

// Sign-out runs this, and it holds OTHER lifters' names and avatars — the next
// person to use the browser must not be shown a roster from an install they may
// not be on.
describe("resetAchievements", () => {
  it("forgets everything", async () => {
    served([testAchievementHolders({ holders: [testLifter({ id: 7 })] })]);
    await loadAchievements();

    resetAchievements();

    expect(achievements.items).toEqual([]);
    expect(achievements.loaded).toBe(false);
    expect(crownsFor(7)).toEqual([]);
  });
});
