import { auth } from "./auth.svelte";
import { DEFAULT_BAR_LB, DEFAULT_PLATES, type PlateInventory } from "./plates";

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
