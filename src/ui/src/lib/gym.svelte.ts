import { auth } from "./auth.svelte";
import {
  DEFAULT_BAR_LB,
  DEFAULT_PLATES,
  barStepLb,
  type PlateInventory,
} from "./plates";
import { DEFAULT_DUMBBELL_STEP_LB, type GymSteps } from "./library";

/**
 * The lifter's gym, read off the profile.
 *
 * Deliberately not a store of its own. The bar and the rack arrive on `User`
 * with everything else the profile carries, so `auth.me` is already the single
 * source — a second copy here would only be a second thing to keep in sync, and
 * the failure mode is the plate bar disagreeing with the warm-up ramp about
 * what the bar weighs.
 *
 * Both accessors fall back rather than returning null. Every caller is drawing a
 * weight and has to draw something; the defaults are the same ones the server
 * uses when nothing has been configured, so the fallback renders a plausible bar
 * rather than an empty one during the moment before /me settles.
 */

/** What the bar weighs, in pounds. */
export function barWeightLb(): number {
  return auth.me?.barWeightLb || DEFAULT_BAR_LB;
}

/**
 * The plates the lifter owns.
 *
 * An account that genuinely owns no plates has an empty array, and that is
 * honoured — every prescription then reads bar-only, which is the truth. Only a
 * missing profile falls back to the standard rack, which is why this tests for
 * the array rather than for its length.
 */
export function plateInventory(): PlateInventory {
  return auth.me?.plates ?? DEFAULT_PLATES;
}

/**
 * What the lifter's dumbbell rack steps by, PER BELL.
 *
 * Per bell because that is how a rack is labelled and how the lifter reads it;
 * every weight the app *shows* is the whole load, so the pair steps twice this.
 * The doubling happens in `gymSteps` and nowhere else.
 */
export function dumbbellStepLb(): number {
  return auth.me?.dumbbellStepLb || DEFAULT_DUMBBELL_STEP_LB / 2;
}

/**
 * The smallest change each kind of equipment admits in this lifter's gym, as
 * whole load — what `equipmentStepLb` needs and what the API's progression
 * engine computed the prescription with.
 *
 * Derived here rather than sent as two more profile fields, so there is one
 * place on the client that knows a pair is two bells and a plate change is two
 * plates. The API derives the same pair of numbers the same way in
 * GetGymSteps; these have to agree, because one draws the button and the other
 * moves the weight.
 */
export function gymSteps(): GymSteps {
  return {
    barLb: barStepLb(plateInventory()),
    dumbbellLb: dumbbellStepLb() * 2,
  };
}
