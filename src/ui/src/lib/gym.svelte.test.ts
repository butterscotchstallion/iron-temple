import { describe, it, expect, afterEach } from "vitest";
import { barWeightLb, plateInventory } from "./gym.svelte";
import { auth } from "./auth.svelte";
import { DEFAULT_BAR_LB, DEFAULT_PLATES } from "./plates";
import type { User } from "./api";

function signIn(gym: Partial<User>) {
  auth.me = {
    id: 1,
    username: "ada",
    displayName: "Ada",
    avatarColor: "",
    isAdmin: true,
    hasAvatar: false,
    barWeightLb: 45,
    plates: [],
    ...gym,
  } as User;
  auth.loaded = true;
}

afterEach(() => {
  auth.me = null;
  auth.loaded = false;
});

describe("barWeightLb", () => {
  it("reads the signed-in lifter's bar", () => {
    signIn({ barWeightLb: 80 });
    expect(barWeightLb()).toBe(80);
  });

  it("falls back while the profile is still loading", () => {
    auth.me = null;
    expect(barWeightLb()).toBe(DEFAULT_BAR_LB);
  });
});

describe("plateInventory", () => {
  it("reads the signed-in lifter's rack", () => {
    signIn({ plates: [{ plateLb: 45, pairs: 1 }] });
    expect(plateInventory()).toEqual([{ plateLb: 45, pairs: 1 }]);
  });

  it("honours a lifter who owns no plates", () => {
    // An empty rack is a fact about the gym, not a sign that nothing loaded —
    // the bar is all there is, and the prescription should say so rather than
    // quietly substituting a rack they do not have.
    signIn({ plates: [] });
    expect(plateInventory()).toEqual([]);
  });

  it("falls back only when there is no profile at all", () => {
    auth.me = null;
    expect(plateInventory()).toBe(DEFAULT_PLATES);
  });
});
