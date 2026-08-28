import {
  DEFAULT_BAR_LB,
  DEFAULT_PLATES,
  loadBar,
  type PlateInventory,
} from "./plates";

/** One warm-up entry: a weight for `reps` reps, performed `sets` times. */
export type WarmupSet = {
  weightLb: number;
  reps: number;
  sets: number;
};

/**
 * StrongLifts-style warm-up ramp for a work weight: two sets with the empty
 * bar, then ascending sets at ~50/70/90% of the work weight with descending
 * reps. Ramps at or below the previous rung, or at or above the work weight,
 * are dropped — so a light work weight yields fewer warm-ups. Returns [] when
 * the work weight is at or below the bar.
 *
 * Each rung is rounded by loading it on the lifter's own rack rather than by
 * snapping to a nominal step. That is the difference between a warm-up they can
 * put on the bar and one they have to improvise: 70% of 185 is 129.5, and what
 * belongs on the screen is the nearest weight the plates in the room add up to.
 */
export function warmupSets(
  workLb: number,
  bar = DEFAULT_BAR_LB,
  plates: PlateInventory = DEFAULT_PLATES,
): WarmupSet[] {
  if (workLb <= bar) return [];

  const round = (w: number) => loadBar(w, bar, plates).weightLb;

  const result: WarmupSet[] = [{ weightLb: bar, reps: 5, sets: 2 }];
  const ramps = [
    { pct: 0.5, reps: 5 },
    { pct: 0.7, reps: 3 },
    { pct: 0.9, reps: 2 },
  ];

  let prev = bar;
  for (const { pct, reps } of ramps) {
    const weightLb = round(workLb * pct);
    if (weightLb > prev && weightLb < workLb) {
      result.push({ weightLb, reps, sets: 1 });
      prev = weightLb;
    }
  }
  return result;
}
