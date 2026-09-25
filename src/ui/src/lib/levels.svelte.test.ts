import { describe, it, expect, afterEach, beforeEach, vi } from "vitest";
import { levelFor, levels, loadLevels, resetLevels } from "./levels.svelte";
import { testLifterLevel } from "./testFixtures";

const getLevels = vi.hoisted(() => vi.fn());

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
  resetLevels();
});

afterEach(() => resetLevels());

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
