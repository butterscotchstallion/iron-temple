import { describe, it, expect } from "vitest";
import { warmupSets } from "./warmup";
import type { PlateInventory } from "./plates";

// The bar and the rack are arguments now, not module constants. Passed
// explicitly here for the same reason plates.test.ts does it: what a fresh
// account gets seeded is a separate decision from what this file asserts.
const BAR = 80;
const RACK: PlateInventory = [
  { plateLb: 45, pairs: 2 },
  { plateLb: 35, pairs: 2 },
  { plateLb: 25, pairs: 2 },
  { plateLb: 10, pairs: 2 },
  { plateLb: 5, pairs: 2 },
  { plateLb: 2.5, pairs: 2 },
];

describe("warmupSets", () => {
  it("returns no warm-ups at or below the bar", () => {
    expect(warmupSets(80, BAR, RACK)).toEqual([]);
    expect(warmupSets(50, BAR, RACK)).toEqual([]);
  });

  it("opens with two empty-bar sets", () => {
    expect(warmupSets(200, BAR, RACK)[0]).toEqual({
      weightLb: 80,
      reps: 5,
      sets: 2,
    });
  });

  it("ramps ~50/70/90% with descending reps", () => {
    expect(warmupSets(200, BAR, RACK)).toEqual([
      { weightLb: 80, reps: 5, sets: 2 },
      { weightLb: 100, reps: 5, sets: 1 },
      { weightLb: 140, reps: 3, sets: 1 },
      { weightLb: 180, reps: 2, sets: 1 },
    ]);
  });

  it("drops ramps below the bar for a light work weight", () => {
    // 50% and 70% of 100 fall below the 80 lb bar; only the 90% rung survives.
    expect(warmupSets(100, BAR, RACK)).toEqual([
      { weightLb: 80, reps: 5, sets: 2 },
      { weightLb: 90, reps: 2, sets: 1 },
    ]);
  });

  it("keeps every warm-up strictly below the work weight", () => {
    for (const w of warmupSets(120, BAR, RACK)) {
      expect(w.weightLb).toBeLessThan(120);
    }
  });

  it("starts the ramp at whatever the bar actually weighs", () => {
    // The whole point of taking the bar as an argument: a 45 lb bar warms up
    // from 45, and the rungs move with it.
    expect(warmupSets(200, 45, RACK)[0]).toEqual({
      weightLb: 45,
      reps: 5,
      sets: 2,
    });
  });

  it("never prescribes more warm-up sets than work sets", () => {
    // The full ramp for 200 is five sets. A program that squats twice gets the
    // two heaviest rungs — 140 and 180 — and not two sets with the empty bar.
    expect(warmupSets(200, BAR, RACK, 2)).toEqual([
      { weightLb: 140, reps: 3, sets: 1 },
      { weightLb: 180, reps: 2, sets: 1 },
    ]);
    expect(warmupSets(200, BAR, RACK, 5)).toHaveLength(4); // 5x5 keeps it all
  });

  it("sheds the second empty-bar set before dropping the bar entirely", () => {
    // One over the cap: the opener stays, it just happens once.
    expect(warmupSets(200, BAR, RACK, 4)).toEqual([
      { weightLb: 80, reps: 5, sets: 1 },
      { weightLb: 100, reps: 5, sets: 1 },
      { weightLb: 140, reps: 3, sets: 1 },
      { weightLb: 180, reps: 2, sets: 1 },
    ]);
  });

  it("returns no warm-ups when there are no work sets to warm up for", () => {
    expect(warmupSets(200, BAR, RACK, 0)).toEqual([]);
  });

  it("only proposes rungs the rack can build", () => {
    // 50/70/90% of 185 are 92.5, 129.5 and 166.5 — none of them loadable on a
    // rack of 45s alone. Every rung that survives must be a weight this gym can
    // actually put on the bar.
    const coarse: PlateInventory = [{ plateLb: 45, pairs: 2 }];
    for (const w of warmupSets(185, BAR, coarse)) {
      const perSide = (w.weightLb - BAR) / 2;
      expect(perSide % 45).toBe(0);
      expect(perSide / 45).toBeLessThanOrEqual(2);
    }
  });
});
