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

// The two the celebration is made of. Mocked rather than observed, because what is
// under test is WHETHER they fire and how often — confetti in jsdom is a canvas
// nobody can assert on, and a real toast would leave state across tests.
const celebrate = vi.hoisted(() => vi.fn());
vi.mock("./celebrate", () => ({ celebrate }));

const pushToast = vi.hoisted(() => vi.fn());
vi.mock("./toast.svelte", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./toast.svelte")>()),
  pushToast,
}));

const ME = 1;
const THEM = 2;

/** A 200 carrying these entries. */
function served(items: ReturnType<typeof testAchievementHolders>[]) {
  getAchievements.mockResolvedValue({ status: 200, data: { items } });
}

beforeEach(() => {
  getAchievements.mockReset();
  celebrate.mockReset();
  pushToast.mockReset();
  localStorage.clear();
  resetAchievements();
});
afterEach(() => {
  localStorage.clear();
  resetAchievements();
});

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

// The earner's half of the news. The server tells everybody else and deliberately
// not them, so this is the only thing that marks the moment for the person who
// actually did it.
describe("celebrating a crown the caller took", () => {
  /** Get past the silent first observation, holding nothing. */
  async function settled() {
    served([testAchievementHolders({ holders: [testLifter({ id: THEM })] })]);
    await loadAchievements(ME);
    celebrate.mockReset();
    pushToast.mockReset();
  }

  it("says nothing on the first load, however much is already held", async () => {
    served([testAchievementHolders({ holders: [testLifter({ id: ME })] })]);
    await loadAchievements(ME);

    expect(pushToast).not.toHaveBeenCalled();
    expect(celebrate).not.toHaveBeenCalled();
  });

  it("toasts and fires confetti when a crown becomes theirs", async () => {
    await settled();

    served([testAchievementHolders({ holders: [testLifter({ id: ME })] })]);
    await loadAchievements(ME);

    expect(pushToast).toHaveBeenCalledTimes(1);
    expect(pushToast.mock.calls[0][0]).toMatchObject({
      body: "Top of Week streak",
      tone: "success",
    });
    expect(celebrate).toHaveBeenCalledTimes(1);
  });

  // ONE BURST, MANY TOASTS. The reconciler runs a whole pass at a time, so taking
  // three boards at once is ordinary — three overlapping confetti bursts is a
  // stutter, not three times the celebration.
  it("fires confetti once however many crowns arrived together", async () => {
    await settled();

    served([
      testAchievementHolders({ holders: [testLifter({ id: ME })] }),
      testAchievementHolders({
        achievement: testAchievement({ slug: "crown-volume", label: "Top of Volume" }),
        holders: [testLifter({ id: ME })],
      }),
    ]);
    await loadAchievements(ME);

    expect(pushToast).toHaveBeenCalledTimes(2);
    expect(celebrate).toHaveBeenCalledTimes(1);
  });

  it("says nothing about somebody else taking one", async () => {
    await settled();

    served([
      testAchievementHolders({
        achievement: testAchievement({ slug: "crown-volume" }),
        holders: [testLifter({ id: THEM })],
      }),
      testAchievementHolders({ holders: [testLifter({ id: THEM })] }),
    ]);
    await loadAchievements(ME);

    expect(pushToast).not.toHaveBeenCalled();
    expect(celebrate).not.toHaveBeenCalled();
  });

  it("says nothing on a poll that found no change", async () => {
    await settled();

    served([testAchievementHolders({ holders: [testLifter({ id: ME })] })]);
    await loadAchievements(ME);
    expect(pushToast).toHaveBeenCalledTimes(1);

    pushToast.mockReset();
    celebrate.mockReset();
    await loadAchievements(ME);

    expect(pushToast).not.toHaveBeenCalled();
    expect(celebrate).not.toHaveBeenCalled();
  });

  // Without an id there is nobody to congratulate, and the poller is the only
  // caller that has one — every other call site loads the standings to draw them.
  it("says nothing when no viewer was named", async () => {
    served([testAchievementHolders({ holders: [testLifter({ id: ME })] })]);
    await loadAchievements();
    served([testAchievementHolders({ holders: [testLifter({ id: ME })] })]);
    await loadAchievements();

    expect(pushToast).not.toHaveBeenCalled();
  });

  // A failed poll must not be read as "you lost every crown", which would then
  // announce them all again on the next success.
  it("says nothing when the request failed", async () => {
    await settled();

    getAchievements.mockResolvedValue({ status: 500, data: {} });
    await loadAchievements(ME);

    expect(pushToast).not.toHaveBeenCalled();
    expect(celebrate).not.toHaveBeenCalled();
  });
});
