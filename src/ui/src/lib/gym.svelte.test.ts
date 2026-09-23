import { describe, it, expect, afterEach } from "vitest";
import {
  barWeightLb,
  equipmentConfirmed,
  gymSteps,
  plateInventory,
} from "./gym.svelte";
import { auth } from "./auth.svelte";
import { DEFAULT_BAR_LB } from "./plates";
import type { User } from "./api";
import { testUser } from "./testFixtures";

function signIn(gym: Partial<User>) {
  auth.me = testUser({ barWeightLb: 45, plates: [], ...gym });
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

  it("invents no rack when there is no profile", () => {
    // This used to answer with the standard rack — 45s, 35s, 25s — for a
    // caller that had not loaded a profile yet. Nothing owned those plates.
    // A lifter reported the plate calculator offering them a 35 they do not
    // have; that one came from a seeded row rather than from here, but this
    // was the same guess waiting for its turn, so it is gone.
    auth.me = null;
    expect(plateInventory()).toEqual([]);
  });
});

describe("gymSteps", () => {
  // The bar's and the pair's are derived, so this is the one place that knows a
  // pair is two bells and a plate change is two plates.
  it("doubles the rack's per-bell step and derives the bar's", () => {
    signIn({
      dumbbellStepLb: 2.5,
      plates: [{ plateLb: 1.25, pairs: 2 }],
    });
    const steps = gymSteps();
    expect(steps.dumbbellLb).toBe(5);
    expect(steps.barLb).toBe(2.5);
  });

  // The stacks are NOT doubled and NOT derived: a lifter holds two bells and
  // moves one pin, and the app records no inventory to derive a stack from.
  it("carries the stacks through as stored", () => {
    signIn({ machineStepLb: 15, cableStepLb: 10, bandStepLb: 20 });
    const steps = gymSteps();
    expect(steps.machineLb).toBe(15);
    expect(steps.cableLb).toBe(10);
    expect(steps.bandLb).toBe(20);
  });
});

describe("equipmentConfirmed", () => {
  it("is false for a gym the app assembled", () => {
    signIn({ equipmentConfirmedAt: null });
    expect(equipmentConfirmed()).toBe(false);
  });

  it("is true once a lifter has said it is right", () => {
    signIn({ equipmentConfirmedAt: "2026-09-23T00:00:00Z" });
    expect(equipmentConfirmed()).toBe(true);
  });

  // No profile reads as unconfirmed, which is the same answer an unconfirmed
  // gym gives. The copy this drives is a prompt to go and check, and there is
  // nothing to prompt on a screen with no lifter on it yet.
  it("is false while no profile is loaded", () => {
    auth.me = null;
    expect(equipmentConfirmed()).toBe(false);
  });
});
