import { describe, it, expect, afterEach, beforeEach, vi } from "vitest";
import {
  houseById,
  houseFor,
  houses,
  loadHouses,
  resetHouses,
  sigilFor,
} from "./houses.svelte";
import { testHouse, testHouseMembership } from "./testFixtures";

const listHouses = vi.hoisted(() => vi.fn());

// The barrel, never the generated files — mocking those would pin the tests to
// orval's output shape rather than to the contract.
vi.mock("./api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./api")>()),
  listHouses,
}));

// Which House everybody is in.
//
// What is worth most here is the lookup rather than the fetch: every name on the
// screen consults it, and the two answers a caller must be able to tell apart are
// "in no House" and "we have not been told yet".

const ME = 1;
const THEM = 2;

/** A 200 carrying these Houses and memberships. */
function served(
  items: ReturnType<typeof testHouse>[],
  memberships: ReturnType<typeof testHouseMembership>[],
) {
  listHouses.mockResolvedValue({ status: 200, data: { items, memberships } });
}

beforeEach(() => {
  listHouses.mockReset();
  resetHouses();
});

afterEach(() => resetHouses());

describe("sigilFor", () => {
  it("is the sigil of the House the lifter is in", async () => {
    served([testHouse({ id: 7, sigil: "IRON" })], [testHouseMembership({ userId: ME, houseId: 7 })]);
    await loadHouses();

    expect(sigilFor(ME)).toBe("IRON");
  });

  it("is null for a lifter in no House", async () => {
    served([testHouse({ id: 7 })], [testHouseMembership({ userId: ME, houseId: 7 })]);
    await loadHouses();

    expect(sigilFor(THEM)).toBeNull();
  });

  it("is null for an undefined lifter, rather than throwing", async () => {
    // Callers pass `auth.me?.id`, so this is the signed-out case and it is an
    // answer rather than an error for them to handle.
    expect(sigilFor(undefined)).toBeNull();
  });

  it("is null before anything has loaded", () => {
    expect(sigilFor(ME)).toBeNull();
    expect(houses.loaded).toBe(false);
  });
});

describe("houseFor", () => {
  it("is the whole House, so a hover card needs no second lookup", async () => {
    served(
      [testHouse({ id: 7, name: "House Iron", tagline: "We lift at dawn" })],
      [testHouseMembership({ userId: ME, houseId: 7 })],
    );
    await loadHouses();

    expect(houseFor(ME)?.name).toBe("House Iron");
    expect(houseFor(ME)?.tagline).toBe("We lift at dawn");
  });

  it("ignores a membership naming a House that is not in the list", async () => {
    // Not a case the API produces — the two halves come from one request — but a
    // stale membership must render nothing rather than a House with no name.
    served([testHouse({ id: 7 })], [testHouseMembership({ userId: ME, houseId: 999 })]);
    await loadHouses();

    expect(houseFor(ME)).toBeNull();
  });
});

describe("houseById", () => {
  it("names a House for a notification carrying only its id", async () => {
    served([testHouse({ id: 7, name: "House Iron" })], []);
    await loadHouses();

    expect(houseById(7)?.name).toBe("House Iron");
  });

  it("is null for a House that has since been deleted", async () => {
    // Its last member left. The notification outlives it only until the cascade,
    // and the panel says the unnamed thing in the meantime.
    served([], []);
    await loadHouses();

    expect(houseById(7)).toBeNull();
  });
});

describe("loadHouses", () => {
  it("leaves the last good list in place when a load fails", async () => {
    served([testHouse({ id: 7, sigil: "IRON" })], [testHouseMembership({ userId: ME, houseId: 7 })]);
    await loadHouses();

    listHouses.mockResolvedValue({ status: 500, data: undefined });
    await loadHouses();

    // A sigil is an ornament: a failed poll shows the previous answer rather than
    // stripping every tag off every name on the screen.
    expect(sigilFor(ME)).toBe("IRON");
    expect(houses.loaded).toBe(true);
  });

  it("does not become loaded on a first load that fails", async () => {
    listHouses.mockResolvedValue({ status: 500, data: undefined });
    await loadHouses();

    // Which is what lets the Houses page tell "couldn't load" from "none yet".
    expect(houses.loaded).toBe(false);
  });
});

describe("resetHouses", () => {
  it("drops everything, so the next account sees no frame of this one", async () => {
    served([testHouse({ id: 7 })], [testHouseMembership({ userId: ME, houseId: 7 })]);
    await loadHouses();

    resetHouses();

    expect(houses.items).toEqual([]);
    expect(houses.memberships).toEqual([]);
    expect(houses.loaded).toBe(false);
    expect(sigilFor(ME)).toBeNull();
  });
});
