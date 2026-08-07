import { describe, it, expect } from "vitest";
import { warmupSets } from "./warmup";

describe("warmupSets", () => {
  it("returns no warm-ups at or below the bar", () => {
    expect(warmupSets(80)).toEqual([]);
    expect(warmupSets(50)).toEqual([]);
  });

  it("opens with two empty-bar sets", () => {
    expect(warmupSets(200)[0]).toEqual({ weightLb: 80, reps: 5, sets: 2 });
  });

  it("ramps ~50/70/90% with descending reps, rounded to 5 lb", () => {
    expect(warmupSets(200)).toEqual([
      { weightLb: 80, reps: 5, sets: 2 },
      { weightLb: 100, reps: 5, sets: 1 },
      { weightLb: 140, reps: 3, sets: 1 },
      { weightLb: 180, reps: 2, sets: 1 },
    ]);
  });

  it("drops ramps below the bar for a light work weight", () => {
    // 50% and 70% of 100 fall below the 80 lb bar; only the 90% rung survives.
    expect(warmupSets(100)).toEqual([
      { weightLb: 80, reps: 5, sets: 2 },
      { weightLb: 90, reps: 2, sets: 1 },
    ]);
  });

  it("keeps every warm-up strictly below the work weight", () => {
    for (const w of warmupSets(120)) {
      expect(w.weightLb).toBeLessThan(120);
    }
  });
});
