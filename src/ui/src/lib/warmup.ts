import { BAR_LB, PLATES_LB } from "./plates";

/** One warm-up entry: a weight for `reps` reps, performed `sets` times. */
export type WarmupSet = {
  weightLb: number;
  reps: number;
  sets: number;
};

/**
 * StrongLifts-style warm-up ramp for a work weight: two sets with the empty
 * bar, then ascending sets at ~50/70/90% of the work weight with descending
 * reps. Weights round to the smallest loadable step (2 × the lightest plate);
 * ramps at/below the previous rung, or at/above the work weight, are dropped —
 * so a light work weight yields fewer warm-ups. Returns [] when the work weight
 * is at or below the bar.
 */
export function warmupSets(
  workLb: number,
  bar = BAR_LB,
  plates = PLATES_LB,
): WarmupSet[] {
  if (workLb <= bar) return [];

  const step = 2 * Math.min(...plates);
  const round = (w: number) => Math.round(w / step) * step;

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
