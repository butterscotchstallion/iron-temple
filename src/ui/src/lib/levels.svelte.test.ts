import { describe, it, expect, afterEach, beforeEach, vi } from "vitest";
import { levelFor, levels, loadLevels, resetLevels } from "./levels.svelte";
import { resetLevelWatch } from "./levelWatch";
import { testLifterLevel } from "./testFixtures";

const getLevels = vi.hoisted(() => vi.fn());

const celebrate = vi.hoisted(() => vi.fn());
vi.mock("./celebrate", () => ({ celebrate }));

const pushToast = vi.hoisted(() => vi.fn());
vi.mock("./toast.svelte", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./toast.svelte")>()),
  pushToast,
}));

// The barrel, never the generated files — mocking those would pin the tests to
// orval's output shape rather than to the contract.
vi.mock("./api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./api")>()),
  getLevels,
}));

// How much everybody has trained.
//
// What is worth most here is the lookup rather than the fetch: every name on the
// screen consults it, and the answer that has to be right is what an id the list
// does not carry gets — because that is the state of every name on a cold load.

const ME = 1;
const THEM = 2;

/** A 200 carrying these entries. */
function served(items: ReturnType<typeof testLifterLevel>[]) {
  getLevels.mockResolvedValue({ status: 200, data: { items } });
}

beforeEach(() => {
  getLevels.mockReset();
  celebrate.mockReset();
  pushToast.mockReset();
  localStorage.clear();
  resetLevels();
});

afterEach(() => {
  resetLevels();
  resetLevelWatch();
  localStorage.clear();
});

describe("levelFor", () => {
  it("gives a lifter the entry they were sent", async () => {
    served([testLifterLevel({ lifterId: THEM, level: 12 })]);
    await loadLevels();

    expect(levelFor(THEM)?.level).toBe(12);
  });

  // The cold-load case, and the reason there is no Level 1 default in this
  // module: one would put a "1" beside every name on the feed for the first beat
  // of a load and then jump to the real number.
  it("gives nothing before the first load lands", () => {
    expect(levelFor(THEM)).toBeNull();
  });

  // An untrained lifter is NOT the null case — the server lists them at level 1,
  // so the client never has to know what an untrained lifter looks like.
  it("gives level 1 to a lifter the server listed as untrained", async () => {
    served([testLifterLevel({ lifterId: THEM, level: 1, xp: 0, xpIntoLevel: 0 })]);
    await loadLevels();

    expect(levelFor(THEM)?.level).toBe(1);
  });

  // After a load, null means an id this install does not carry: an account
  // deleted but still named by an old notification row, or somebody who
  // registered since the last poll.
  it("gives nothing for a lifter the list does not carry", async () => {
    served([testLifterLevel({ lifterId: THEM })]);
    await loadLevels();

    expect(levelFor(ME)).toBeNull();
  });

  // Callers pass `auth.me?.id`, and a reader with no account is not a case each
  // of them should have to handle.
  it("answers undefined like a lifter it has never heard of", () => {
    expect(levelFor(undefined)).toBeNull();
  });
});

describe("loadLevels", () => {
  it("marks itself loaded once an answer lands", async () => {
    served([]);
    await loadLevels();

    expect(levels.loaded).toBe(true);
  });

  // A level is an ornament: failing to fetch one shows names without badges,
  // which is what they looked like before the feature existed.
  it("keeps the last good list when a request fails", async () => {
    served([testLifterLevel({ lifterId: THEM, level: 12 })]);
    await loadLevels();

    getLevels.mockResolvedValue({ status: 500, data: undefined });
    await loadLevels();

    expect(levelFor(THEM)?.level).toBe(12);
  });

  // `loaded` is what lets a surface tell "nobody has trained" from "we have not
  // been told yet", so a failure must not claim to have been told.
  it("does not claim to have loaded after a failure", async () => {
    getLevels.mockResolvedValue({ status: 500, data: undefined });
    await loadLevels();

    expect(levels.loaded).toBe(false);
  });
});

// Sign-out. This holds how much every lifter on the previous account's install
// has trained, and the next person to use the browser must not see a frame of it.
describe("resetLevels", () => {
  it("drops everything", async () => {
    served([testLifterLevel({ lifterId: THEM })]);
    await loadLevels();

    resetLevels();

    expect(levels.items).toEqual([]);
    expect(levels.loaded).toBe(false);
    expect(levelFor(THEM)).toBeNull();
  });
});

// The earner's half of the news, and the only half there is: nothing on the server
// knows a level changed, because a level is derived from a count of sessions rather
// than written. The difference exists only between two readings, and this is what
// holds both.
describe("celebrating a level the caller reached", () => {
  /** Get past the silent first observation, sitting at level 11. */
  async function settled() {
    served([testLifterLevel({ lifterId: ME, level: 11 })]);
    await loadLevels(ME);
    celebrate.mockReset();
    pushToast.mockReset();
  }

  it("says nothing on the first load, however high the level", async () => {
    served([testLifterLevel({ lifterId: ME, level: 27 })]);
    await loadLevels(ME);

    expect(pushToast).not.toHaveBeenCalled();
    expect(celebrate).not.toHaveBeenCalled();
  });

  it("toasts and fires confetti on a level-up", async () => {
    await settled();

    served([testLifterLevel({ lifterId: ME, level: 12 })]);
    await loadLevels(ME);

    expect(pushToast).toHaveBeenCalledTimes(1);
    expect(pushToast.mock.calls[0][0]).toMatchObject({
      title: "You reached Level 12",
      tone: "success",
    });
    expect(celebrate).toHaveBeenCalledTimes(1);
  });

  // ONE TOAST AND ONE BURST however many levels landed between two readings — an
  // offline queue draining, or a poll that missed an afternoon. Where they got to
  // is the whole of the news; three bursts would be a stutter.
  it("says it once when several levels land at once", async () => {
    await settled();

    served([testLifterLevel({ lifterId: ME, level: 14 })]);
    await loadLevels(ME);

    expect(pushToast).toHaveBeenCalledTimes(1);
    expect(pushToast.mock.calls[0][0]).toMatchObject({
      title: "You reached Level 14",
    });
    expect(celebrate).toHaveBeenCalledTimes(1);
  });

  it("says nothing about somebody else levelling up", async () => {
    await settled();

    served([
      testLifterLevel({ lifterId: ME, level: 11 }),
      testLifterLevel({ lifterId: THEM, level: 30 }),
    ]);
    await loadLevels(ME);

    expect(pushToast).not.toHaveBeenCalled();
    expect(celebrate).not.toHaveBeenCalled();
  });

  it("says nothing on an unchanged poll", async () => {
    await settled();

    served([testLifterLevel({ lifterId: ME, level: 11 })]);
    await loadLevels(ME);

    expect(pushToast).not.toHaveBeenCalled();
  });

  // Deleting a session lowers experience, which lowers the level. Not an occasion.
  it("says nothing when the level fell", async () => {
    await settled();

    served([testLifterLevel({ lifterId: ME, level: 10 })]);
    await loadLevels(ME);

    expect(pushToast).not.toHaveBeenCalled();
    expect(celebrate).not.toHaveBeenCalled();
  });

  // Every caller that just wants fresh badges passes no id — the live frame's
  // refetch among them. Those must not celebrate, and must not record a baseline
  // either, or they would eat the announcement the next celebrating read owes.
  it("says nothing when no viewer is named, and eats nothing", async () => {
    await settled();

    served([testLifterLevel({ lifterId: ME, level: 12 })]);
    await loadLevels();
    expect(pushToast).not.toHaveBeenCalled();

    await loadLevels(ME);
    expect(pushToast).toHaveBeenCalledTimes(1);
  });

  // A FAILED POLL MUST NOT READ AS A COLLAPSE. loadLevels leaves the last good
  // list in place on a non-200, so there is nothing to compare against nothing —
  // and nothing to announce when it comes back.
  it("says nothing when the request failed", async () => {
    await settled();

    getLevels.mockResolvedValue({ status: 500, data: undefined });
    await loadLevels(ME);

    expect(pushToast).not.toHaveBeenCalled();
    expect(celebrate).not.toHaveBeenCalled();
  });

  // Sign-out clears the watch as well as the list, so the next account is a first
  // observation rather than a comparison against somebody else's training.
  it("forgets the level on sign-out", async () => {
    await settled();
    resetLevels();

    served([testLifterLevel({ lifterId: ME, level: 12 })]);
    await loadLevels(ME);

    expect(pushToast).not.toHaveBeenCalled();
  });
});
