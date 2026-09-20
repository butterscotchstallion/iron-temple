import { describe, it, expect, vi, afterEach } from "vitest";
import { PUN_NAMES, randomPunName } from "./punNames";

afterEach(() => {
  vi.restoreAllMocks();
});

describe("PUN_NAMES", () => {
  // The point of the list is that a suggestion is always something the form can
  // submit. A name the API answers 400 to would put the blame on the admin for
  // a value they never typed.
  it("holds only names the API accepts as usernames", () => {
    for (const name of PUN_NAMES) {
      expect(name).toMatch(/^[A-Za-z0-9._-]+$/);
      expect(name.length).toBeGreaterThanOrEqual(3);
      expect(name.length).toBeLessThanOrEqual(32);
    }
  });

  // A duplicate would be a name twice as likely as the rest, and — worse — one
  // that randomPunName can offer again right after excluding it.
  it("has no duplicates", () => {
    expect(new Set(PUN_NAMES).size).toBe(PUN_NAMES.length);
  });
});

describe("randomPunName", () => {
  it("returns a name from the list", () => {
    expect(PUN_NAMES).toContain(randomPunName());
  });

  it("never returns an excluded name", () => {
    const exclude = PUN_NAMES.slice(0, PUN_NAMES.length - 1);
    // One name is left, so the pick is forced and the assertion is exact rather
    // than probabilistic.
    expect(randomPunName(exclude)).toBe(PUN_NAMES[PUN_NAMES.length - 1]);
  });

  // An install whose roster has grown past the list still has to suggest
  // something: an empty field is worse than one that might collide.
  it("falls back to the full list when everything is excluded", () => {
    expect(PUN_NAMES).toContain(randomPunName(PUN_NAMES));
  });

  it("spreads across the list rather than favouring one end", () => {
    // Math.random is [0, 1), so these are the first and last entries.
    vi.spyOn(Math, "random").mockReturnValue(0);
    expect(randomPunName()).toBe(PUN_NAMES[0]);
    vi.spyOn(Math, "random").mockReturnValue(0.999999);
    expect(randomPunName()).toBe(PUN_NAMES[PUN_NAMES.length - 1]);
  });
});
