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
// The gym, spread into each call so a test names only what it is about.
const GYM = { bar: BAR, plates: RACK };

describe("warmupSets on a barbell", () => {
  it("returns no warm-ups at or below the bar", () => {
    expect(warmupSets(80, GYM)).toEqual([]);
    expect(warmupSets(50, GYM)).toEqual([]);
  });

  it("opens with two empty-bar sets", () => {
    expect(warmupSets(200, GYM)[0]).toEqual({
      weightLb: 80,
      reps: 5,
      sets: 2,
    });
  });

  it("ramps ~50/70/90% with descending reps", () => {
    expect(warmupSets(200, GYM)).toEqual([
      { weightLb: 80, reps: 5, sets: 2 },
      { weightLb: 100, reps: 5, sets: 1 },
      { weightLb: 140, reps: 3, sets: 1 },
      { weightLb: 180, reps: 2, sets: 1 },
    ]);
  });

  it("drops ramps below the bar for a light work weight", () => {
    // 50% and 70% of 100 fall below the 80 lb bar; only the 90% rung survives.
    expect(warmupSets(100, GYM)).toEqual([
      { weightLb: 80, reps: 5, sets: 2 },
      { weightLb: 90, reps: 2, sets: 1 },
    ]);
  });

  it("keeps every warm-up strictly below the work weight", () => {
    for (const w of warmupSets(120, GYM)) {
      expect(w.weightLb).toBeLessThan(120);
    }
  });

  it("starts the ramp at whatever the bar actually weighs", () => {
    // The whole point of taking the bar as an argument: a 45 lb bar warms up
    // from 45, and the rungs move with it.
    expect(warmupSets(200, { ...GYM, bar: 45 })[0]).toEqual({
      weightLb: 45,
      reps: 5,
      sets: 2,
    });
  });

  it("never prescribes more warm-up sets than work sets", () => {
    // The full ramp for 200 is five sets. A program that squats twice gets the
    // two heaviest rungs — 140 and 180 — and not two sets with the empty bar.
    expect(warmupSets(200, { ...GYM, maxSets: 2 })).toEqual([
      { weightLb: 140, reps: 3, sets: 1 },
      { weightLb: 180, reps: 2, sets: 1 },
    ]);
    expect(warmupSets(200, { ...GYM, maxSets: 5 })).toHaveLength(4); // 5x5 keeps it all
  });

  it("sheds the second empty-bar set before dropping the bar entirely", () => {
    // One over the cap: the opener stays, it just happens once.
    expect(warmupSets(200, { ...GYM, maxSets: 4 })).toEqual([
      { weightLb: 80, reps: 5, sets: 1 },
      { weightLb: 100, reps: 5, sets: 1 },
      { weightLb: 140, reps: 3, sets: 1 },
      { weightLb: 180, reps: 2, sets: 1 },
    ]);
  });

  it("returns no warm-ups when there are no work sets to warm up for", () => {
    expect(warmupSets(200, { ...GYM, maxSets: 0 })).toEqual([]);
  });

  it("puts whole plate weights on the rungs, not float arithmetic", () => {
    // A 165 lb bench: 50/70/90% are 82.5, 115.49999999999999 and 148.5. Every
    // rung is a number a lifter reads off a card and loads, so none of them may
    // carry the dust from the multiplication that produced it.
    for (const w of warmupSets(165, GYM)) {
      expect(w.weightLb).toBe(Math.round(w.weightLb * 2) / 2);
    }
    expect(warmupSets(165, GYM)).toEqual([
      { weightLb: 80, reps: 5, sets: 2 },
      { weightLb: 115, reps: 3, sets: 1 },
      { weightLb: 145, reps: 2, sets: 1 },
    ]);
  });

  it("only proposes rungs the rack can build", () => {
    // 50/70/90% of 185 are 92.5, 129.5 and 166.5 — none of them loadable on a
    // rack of 45s alone. Every rung that survives must be a weight this gym can
    // actually put on the bar.
    const coarse: PlateInventory = [{ plateLb: 45, pairs: 2 }];
    for (const w of warmupSets(185, { bar: BAR, plates: coarse })) {
      const perSide = (w.weightLb - BAR) / 2;
      expect(perSide % 45).toBe(0);
      expect(perSide / 45).toBeLessThanOrEqual(2);
    }
  });

  it("warms up on a bar when nobody says what the equipment is", () => {
    // The default, and the reason it is the default: a caller that does not know
    // gets the ramp this app has always produced rather than a barless one.
    expect(warmupSets(200, GYM)).toEqual(
      warmupSets(200, { ...GYM, equipment: "barbell" }),
    );
  });
});

// The bug these cover: a lifter curling a 30 lb pair of dumbbells was told to
// warm up with two sets of an 80 lb bar, which is not a thing that exists for
// this movement, and every rung was rounded onto plates nobody was loading.
describe("warmupSets off the barbell", () => {
  const dumbbell = { ...GYM, equipment: "dumbbell" };

  it("has no empty-bar opener, because there is no bar", () => {
    // The opener is the entry that carries sets: 2 and sits at exactly the bar's
    // weight. Asserted on curls rather than a press, because 50% of a 160 lb
    // press is 80 — the bar's weight by coincidence, which would make "no rung
    // equals the bar" pass for the wrong reason.
    const ramp = warmupSets(30, dumbbell);
    expect(ramp.every((w) => w.sets === 1)).toBe(true);
    expect(ramp.some((w) => w.weightLb === BAR)).toBe(false);
  });

  it("ramps a heavy dumbbell press in jumps the rack can make", () => {
    // 50/70/90% of 160 are 80, 112 and 144, rounded down to the 10 lb a pair of
    // bells steps by.
    expect(warmupSets(160, dumbbell)).toEqual([
      { weightLb: 80, reps: 5, sets: 1 },
      { weightLb: 110, reps: 3, sets: 1 },
      { weightLb: 140, reps: 2, sets: 1 },
    ]);
  });

  it("gives light curls a short ramp rather than a nonsense one", () => {
    // 50/70/90% of 30 are 15, 21 and 27 → 10, 20, 20. The last duplicates the
    // rung below it and is dropped, the way a barbell ramp drops a repeat.
    expect(warmupSets(30, dumbbell)).toEqual([
      { weightLb: 10, reps: 5, sets: 1 },
      { weightLb: 20, reps: 3, sets: 1 },
    ]);
  });

  it("ignores the bar and the rack entirely", () => {
    // Nothing about the lifter's gym reaches a lift their gym does not load.
    expect(warmupSets(160, { equipment: "dumbbell" })).toEqual(
      warmupSets(160, { equipment: "dumbbell", bar: 45, plates: [] }),
    );
  });

  it("steps a machine by 5, the way everything that is not a dumbbell does", () => {
    // 50/70/90% of 90 are 45, 63 and 81 → 45, 60, 80.
    expect(warmupSets(90, { equipment: "machine" })).toEqual([
      { weightLb: 45, reps: 5, sets: 1 },
      { weightLb: 60, reps: 3, sets: 1 },
      { weightLb: 80, reps: 2, sets: 1 },
    ]);
  });

  it("gives a bodyweight lift no warm-ups at all", () => {
    // Logged at 0, so there is no percentage of it worth doing. The barbell path
    // gets this from the bar; here it falls out of the floor being 0.
    expect(warmupSets(0, { equipment: "bodyweight" })).toEqual([]);
  });

  it("drops rungs that round away to nothing", () => {
    // A 15 lb pair: 50% and 70% are 7.5 and 10.5, and only the second reaches a
    // 10 lb jump. A rung of 0 is not a warm-up.
    expect(warmupSets(15, dumbbell)).toEqual([{ weightLb: 10, reps: 3, sets: 1 }]);
  });

  it("still caps a barless ramp at the work sets", () => {
    // 3x10 dumbbell press: the ramp is already three, and a 2x10 would trim it.
    expect(warmupSets(160, { ...dumbbell, maxSets: 2 })).toEqual([
      { weightLb: 110, reps: 3, sets: 1 },
      { weightLb: 140, reps: 2, sets: 1 },
    ]);
  });
});

// A ramp is for the lift a session is built around. An accessory gets a feeler
// set: three loaded rungs in front of three sets of curls is a warm-up as long
// as the lift, and nobody in a gym ramps a lateral raise in three steps.
describe("warmupSets on assistance work", () => {
  it("gives one light set instead of a ramp", () => {
    // The prescribed version of this is bar×2 then 100/140/180.
    expect(warmupSets(200, { accessory: true })).toEqual([
      { weightLb: 100, reps: 5, sets: 1 },
    ]);
  });

  it("is the LIGHT rung, not the heavy one a cap would have left", () => {
    // trimToCap sheds from the light end, so `maxSets: 1` keeps the 90% double
    // — a near-max single, which is the opposite of a feeler set. This is why
    // the accessory path picks its rung rather than reusing the cap.
    expect(warmupSets(200, { maxSets: 1 })).toEqual([
      { weightLb: 180, reps: 2, sets: 1 },
    ]);
    expect(warmupSets(200, { accessory: true })[0].weightLb).toBe(100);
  });

  it("drops the empty-bar opener a barbell accessory does not need", () => {
    // Two sets of an empty bar before a barbell curl is a warm-up for the
    // warm-up. The floor stays at the bar, so nothing below it is offered.
    const ramp = warmupSets(200, { accessory: true, bar: 45 });
    expect(ramp).toHaveLength(1);
    expect(ramp[0].weightLb).toBe(100);
  });

  it("gives a light barbell accessory nothing at all", () => {
    // 50% of 100 is 50, above a 45 lb bar but only just; at 80 it is under the
    // bar and there is no honest rung to offer.
    expect(warmupSets(100, { accessory: true, bar: 80 })).toEqual([]);
  });

  it("rounds the rung onto the rack, the same as a full ramp", () => {
    // 50% of a 90 lb pair is 45, and a rack of 5 lb bells makes 40.
    expect(warmupSets(90, { equipment: "dumbbell", accessory: true })).toEqual([
      { weightLb: 40, reps: 5, sets: 1 },
    ]);
  });

  it("gives bodyweight assistance no warm-up", () => {
    // Logged at 0, so there is no percentage of it worth doing — and this is
    // most accessories on the day they are added.
    expect(warmupSets(0, { equipment: "bodyweight", accessory: true })).toEqual([]);
  });
});
