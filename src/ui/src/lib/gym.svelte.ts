import { auth } from "./auth.svelte";
import { DEFAULT_BAR_LB, barStepLb, type PlateInventory } from "./plates";
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
 * honoured — every prescription then reads bar-only, which is the truth.
 *
 * No profile means no plates, rather than the standard rack this used to
 * substitute. That fallback was a rack nobody had agreed to: it named 35s and
 * 25s, and a caller reading it got a confident answer assembled out of nothing.
 * It could not actually reach a screen — App.svelte renders nothing until
 * `auth.loaded` and mounts no route without an `auth.me` — so this removes a
 * claim rather than a flash. But the claim was the same one that put phantom
 * plates in a lifter's profile in the first place, and an accessor that invents
 * equipment is one new caller away from drawing them.
 *
 * The bar above keeps its fallback: a bar is one number with a near-universal
 * value, and something has to be divided by two. An inventory is a list of
 * specific objects, and the honest empty answer is the empty list.
 */
export function plateInventory(): PlateInventory {
  return auth.me?.plates ?? [];
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
 * The bar's and the pair's are derived here rather than sent as two more
 * profile fields, so there is one place on the client that knows a pair is two
 * bells and a plate change is two plates. The API derives the same two numbers
 * the same way in GetGymSteps; these have to agree, because one draws the
 * button and the other moves the weight.
 *
 * The three stacks are NOT derived — there is nothing to derive them from. The
 * app records no inventory of pins or of graded bands, only what the lifter
 * said the gaps are, so they come off the profile as stored and are not
 * doubled: one pin, one band. See 0028.
 */
export function gymSteps(): GymSteps {
  return {
    barLb: barStepLb(plateInventory()),
    dumbbellLb: dumbbellStepLb() * 2,
    machineLb: auth.me?.machineStepLb,
    cableLb: auth.me?.cableStepLb,
    bandLb: auth.me?.bandStepLb,
  };
}

/**
 * Whether this lifter has ever confirmed the gym on file, rather than leaving
 * the one the app seeded for them.
 *
 * False while no profile is loaded, because "no" is the only honest answer to
 * "has this person confirmed?" when there is no person yet. Note that this is
 * NOT the same as "show the nudge": a caller that treats false as a cue to
 * render would put a claim about somebody's gym on screen before the gym had
 * been fetched. Callers check for a profile as well — see
 * ConfirmEquipmentCard.svelte, where that is spelled out.
 *
 * Deliberately not consulted by anything that computes a weight. An unconfirmed
 * rack is still the best information available, so prescribing differently
 * until somebody clicks a button would give two lifters with identical
 * equipment different numbers. Copy only.
 */
export function equipmentConfirmed(): boolean {
  return auth.me?.equipmentConfirmedAt != null;
}
