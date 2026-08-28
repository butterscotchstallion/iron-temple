import { describe, it, expect } from "vitest";
import {
  platesPerSide,
  plateLabel,
  loadBar,
  type PlateInventory,
} from "./plates";

// The bar and the rack are arguments now, not module constants — they come off
// the profile. These pass them explicitly rather than leaning on the defaults,
// so a change to what a fresh account gets seeded cannot quietly rewrite what
// this file claims.
const BAR = 80;
const RACK: PlateInventory = [
  { plateLb: 45, pairs: 2 },
  { plateLb: 35, pairs: 2 },
  { plateLb: 25, pairs: 2 },
  { plateLb: 10, pairs: 2 },
  { plateLb: 5, pairs: 2 },
  { plateLb: 2.5, pairs: 2 },
];

describe("platesPerSide", () => {
  it("is empty for the bar alone or lighter", () => {
    expect(platesPerSide(80, BAR, RACK)).toEqual([]);
    expect(platesPerSide(60, BAR, RACK)).toEqual([]);
  });

  it("loads a single plate per side", () => {
    expect(platesPerSide(90, BAR, RACK)).toEqual([5]); // (90-80)/2 = 5
    expect(platesPerSide(130, BAR, RACK)).toEqual([25]); // (130-80)/2 = 25
    expect(platesPerSide(85, BAR, RACK)).toEqual([2.5]); // (85-80)/2 = 2.5
  });

  it("stacks plates largest-first", () => {
    expect(platesPerSide(220, BAR, RACK)).toEqual([45, 25]); // 70 = 45 + 25
    expect(platesPerSide(135, BAR, RACK)).toEqual([25, 2.5]); // 27.5 = 25 + 2.5
  });

  it("respects a custom bar weight", () => {
    expect(platesPerSide(95, 45, RACK)).toEqual([25]); // (95-45)/2 = 25
  });
});

describe("loadBar with a finite rack", () => {
  it("never asks for more pairs than are owned", () => {
    const oneFortyFive: PlateInventory = [{ plateLb: 45, pairs: 1 }];
    // 3 pairs of 45 a side would make 350; only one pair exists.
    const loaded = loadBar(350, BAR, oneFortyFive);
    expect(loaded.plates).toEqual([45]);
    expect(loaded.weightLb).toBe(170); // 80 + 45*2
    expect(loaded.rounded).toBe(true);
  });

  it("beats greedy when the heaviest plate is a dead end", () => {
    // A 50 lb side: greedy takes the single 45 and then cannot make the last 5.
    // Two 25s are exact, and that is what should come back.
    const awkward: PlateInventory = [
      { plateLb: 45, pairs: 1 },
      { plateLb: 25, pairs: 2 },
    ];
    const loaded = loadBar(180, BAR, awkward); // (180-80)/2 = 50
    expect(loaded.plates).toEqual([25, 25]);
    expect(loaded.weightLb).toBe(180);
    expect(loaded.rounded).toBe(false);
  });

  it("rounds down rather than up when nothing lands exactly", () => {
    // Rounding a working set UP is a heavier set than the program called for,
    // which is the one direction that can hurt.
    const coarse: PlateInventory = [{ plateLb: 45, pairs: 2 }];
    const loaded = loadBar(200, BAR, coarse); // wants 60 a side
    expect(loaded.weightLb).toBe(170); // 45 a side, not 90
    expect(loaded.weightLb).toBeLessThan(200);
    expect(loaded.rounded).toBe(true);
  });

  it("treats an empty rack as a bar and nothing else", () => {
    const loaded = loadBar(225, BAR, []);
    expect(loaded.plates).toEqual([]);
    expect(loaded.weightLb).toBe(BAR);
    expect(loaded.rounded).toBe(true);
  });

  it("reports the bar itself as exact, not as a rounding", () => {
    const loaded = loadBar(BAR, BAR, RACK);
    expect(loaded.rounded).toBe(false);
    expect(loaded.weightLb).toBe(BAR);
  });

  it("keeps the pair count honest across denominations", () => {
    // 1x10 + 2x5 = 20 a side. Nothing may use a third 5.
    const small: PlateInventory = [
      { plateLb: 10, pairs: 1 },
      { plateLb: 5, pairs: 2 },
    ];
    const loaded = loadBar(120, BAR, small); // (120-80)/2 = 20
    expect(loaded.weightLb).toBe(120);
    expect(loaded.plates).toEqual([10, 5, 5]);
    const fives = loaded.plates.filter((p) => p === 5).length;
    expect(fives).toBeLessThanOrEqual(2);
  });
});

describe("plateLabel", () => {
  it("says 'bar only' at or below the bar", () => {
    expect(plateLabel(80, BAR, RACK)).toBe("bar only");
    expect(plateLabel(60, BAR, RACK)).toBe("bar only");
  });

  it("joins the per-side plates largest-first", () => {
    expect(plateLabel(90, BAR, RACK)).toBe("5 / side");
    expect(plateLabel(140, BAR, RACK)).toBe("25 + 5 / side"); // (140-80)/2 = 30
    expect(plateLabel(180, BAR, RACK)).toBe("45 + 5 / side"); // (180-80)/2 = 50
  });

  it("names the weight it actually reached when it had to round", () => {
    // Silently loading something other than the number on the screen is how a
    // lifter ends up doing the wrong set.
    expect(plateLabel(200, BAR, [{ plateLb: 45, pairs: 2 }])).toBe(
      "45 / side · 170 lb",
    );
  });
});
