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
 *
 * maxSets caps the ramp at the number of sets the lifter is actually there to
 * do — see `trimToCap`. Pass the work set count; the default is uncapped, for
 * callers asking what the full ramp would be.
 */
export function warmupSets(
  workLb: number,
  bar = DEFAULT_BAR_LB,
  plates: PlateInventory = DEFAULT_PLATES,
  maxSets = Infinity,
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
  return trimToCap(result, maxSets);
}

/**
 * Drop warm-ups from the light end until the ramp is at most `maxSets` sets.
 *
 * A warm-up longer than the workout is not a warm-up, it is the workout. The
 * low-volume programs are the ones that need this most: "StrongLifts 5x5 Lite"
 * squats twice, and the full five-set ramp in front of it buries the two sets
 * the lifter came in for.
 *
 * Lightest first, because the rungs nearest the work weight are the ones doing
 * the preparing. Given room for two sets before a 315 lb squat, 195 and 255 get
 * a lifter ready for it and two sets with the empty bar do not. The empty-bar
 * entry gives up its second set before it is dropped outright, so a ramp only
 * one over the cap keeps a light opener.
 *
 * Counts expanded sets, not entries — `sets: 2` on the bar is two sets in the
 * gym and two circles on the card, and the cap is a promise about those.
 */
function trimToCap(ramp: WarmupSet[], maxSets: number): WarmupSet[] {
  const out = ramp.map((w) => ({ ...w }));
  let total = out.reduce((n, w) => n + w.sets, 0);
  while (total > maxSets && out.length > 0) {
    if (out[0].sets > 1) out[0].sets--;
    else out.shift();
    total--;
  }
  return out;
}
