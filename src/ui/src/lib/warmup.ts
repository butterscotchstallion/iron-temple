import {
  DEFAULT_BAR_LB,
  DEFAULT_PLATES,
  loadBar,
  type PlateInventory,
} from "./plates";
import { equipmentStepLb, type GymSteps } from "./library";

/** One warm-up entry: a weight for `reps` reps, performed `sets` times. */
export type WarmupSet = {
  weightLb: number;
  reps: number;
  sets: number;
};

/**
 * What a ramp is built against. An object rather than more positional
 * arguments, because these come from three different places and a caller
 * passing the fourth one by counting commas is a caller that will eventually
 * miscount: the bar and the rack are the lifter's gym, maxSets is today's
 * prescription, and equipment belongs to the movement.
 */
export type WarmupGym = {
  /**
   * The movement's equipment. "barbell" is the default because it is what the
   * programs prescribe, and because a caller that does not know is better off
   * with the ramp this app has always produced.
   */
  equipment?: string;
  bar?: number;
  plates?: PlateInventory;
  /**
   * The jumps this lifter's equipment admits, for the rungs that are not loaded
   * on a bar. A barbell ramp rounds against `plates` and never consults this;
   * a pair of dumbbells has no plates to add up, so it rounds to the rack's own
   * step — which is a fact about their rack and not a constant.
   */
  steps?: GymSteps;
  /**
   * Whether this is assistance work rather than a prescribed lift.
   *
   * An accessory gets one light feeler set instead of a ramp. The ramp below is
   * a StrongLifts ramp — it exists to walk a lifter up to a heavy barbell single
   * digit, and applied to a curl it is absurd: three loaded rungs in front of
   * three work sets is a warm-up as long as the lift, and nobody in a gym ramps
   * a lateral raise in three steps. The accessory is the warm-up, past the first
   * set.
   *
   * One set rather than none because an accessory is not always small — a heavy
   * dumbbell row or an RDL earns a feeler — and because a rung that rounds away
   * to nothing is dropped anyway, so light work still ends up with no ramp at all
   * without this having to decide where "light" starts.
   */
  accessory?: boolean;
  /** See `trimToCap`. Uncapped by default, for callers asking what the full ramp would be. */
  maxSets?: number;
};

/**
 * StrongLifts-style warm-up ramp for a work weight: ascending sets at ~50/70/90%
 * of the work weight with descending reps, opening on a barbell with two sets of
 * the empty bar. Ramps at or below the previous rung, or at or above the work
 * weight, are dropped — so a light work weight yields fewer warm-ups.
 *
 * `accessory` collapses all of that to a single ~50% feeler set, because the
 * ramp above is for the lift a session is built around and an accessory is not
 * one. See the field on `WarmupGym`.
 *
 * Equipment decides two things and nothing else.
 *
 * The floor. A barbell ramp starts at the bar, because that is the lightest
 * thing the lift can be done with and starting there is the point of the empty-
 * bar sets. Nothing else has a floor: there is no empty dumbbell, so a pair of
 * bells ramps from nothing and opens on the first percentage that rounds to a
 * weight the rack holds. Returns [] at or below the floor, which is also what
 * makes a bodyweight lift — logged at 0 — produce no warm-ups at all.
 *
 * The rounding. Each rung is rounded by loading it on the lifter's own rack
 * rather than by snapping to a nominal step. That is the difference between a
 * warm-up they can put on the bar and one they have to improvise: 70% of 185 is
 * 129.5, and what belongs on the screen is the nearest weight the plates in the
 * room add up to. Off the barbell there are no plates to add up, so a rung
 * rounds down to the smallest jump its equipment admits — `equipmentStepLb`,
 * which is the same answer the progression engine gives when it moves the weight.
 * Down rather than nearest, for the reason `closestLoad` rounds down: a rung
 * above the one that was asked for is the only direction that can hurt.
 */
export function warmupSets(workLb: number, gym: WarmupGym = {}): WarmupSet[] {
  const {
    equipment = "barbell",
    bar = DEFAULT_BAR_LB,
    plates = DEFAULT_PLATES,
    steps = {},
    accessory = false,
    maxSets = Infinity,
  } = gym;

  const barbell = equipment === "barbell";
  const floor = barbell ? bar : 0;
  if (workLb <= floor) return [];

  const step = equipmentStepLb(equipment, steps);
  const round = barbell
    ? (w: number) => loadBar(w, bar, plates).weightLb
    : (w: number) => Math.floor(w / step) * step;

  // The empty-bar opener is a prescribed-lift thing: two sets of an empty bar
  // before a barbell curl is a warm-up for the warm-up. The floor stays at the
  // bar either way — nothing loads below it, so a light barbell accessory still
  // yields no rungs rather than impossible ones.
  const result: WarmupSet[] = barbell && !accessory ? [{ weightLb: bar, reps: 5, sets: 2 }] : [];
  const ramps = accessory
    ? // The lightest rung, and only it. Note this is NOT `maxSets: 1` on the full
      // ramp: trimToCap sheds from the light end, so a cap of one would keep the
      // 90% double — a near-max single, which is the opposite of a feeler set.
      [{ pct: 0.5, reps: 5 }]
    : [
        { pct: 0.5, reps: 5 },
        { pct: 0.7, reps: 3 },
        { pct: 0.9, reps: 2 },
      ];

  let prev = floor;
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
 * the preparing. Given room for two sets before a 315 lb squat, 220 and 280 get
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
