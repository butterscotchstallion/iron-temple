/**
 * Which movement pattern demonstrates a given exercise.
 *
 * Keyed on the WHOLE name, case-insensitively — never on a substring, unlike
 * the emoji table next door. An emoji makes no claim about technique, so
 * matching "curl" loosely costs nothing; a demonstration does make a claim, and
 * a Goblet Squat inheriting the barbell squat's figure would teach the wrong
 * thing about where the load sits.
 *
 * The names come from the seed migrations — src/api/db/migrations/0009 and
 * 0023. There is no link between those and this file, so a migration that adds
 * a movement leaves it without a figure until someone adds it here.
 * exerciseIcon.ts carries the same exposure. The failure is quiet and harmless:
 * an unknown name gets no demonstration block at all, which is also what every
 * lifter-authored movement gets, since the table can never cover an open set.
 */

import { ARCHETYPES, type ArchetypeName } from "./formArchetypes";
import type { SampledArchetype } from "./formKinematics";

const ARCHETYPE_BY_EXERCISE: Readonly<Record<string, ArchetypeName>> = {
  // legs
  squat: "squat",
  "pause squat": "squat",
  "front squat": "frontSquat",
  "goblet squat": "gobletSquat",
  "bulgarian split squat": "splitSquat",
  "walking lunge": "splitSquat",
  "standing calf raise": "calfRaise",
  "romanian deadlift": "romanianDeadlift",
  "barbell hip thrust": "hipThrust",
  "banded glute bridge": "gluteBridge",
  "banded kickback": "kickback",
  "banded lateral walk": "bandWalk",
  "banded hip abduction": "hipAbduction",
  "leg press": "legPress",
  "leg extension": "legExtension",
  "leg curl": "legCurl",

  // back
  deadlift: "deadlift",
  "pause deadlift": "deadlift",
  "back extension": "backExtension",
  "barbell row": "row",
  "dumbbell row": "row",
  "t-bar row": "row",
  "seated cable row": "seatedRow",
  "face pull": "facePull",
  "pull-up": "verticalPull",
  "chin-up": "verticalPull",
  "lat pulldown": "latPulldown",

  // chest
  "bench press": "benchPress",
  "pause bench press": "benchPress",
  "feet-up bench press": "benchPress",
  "dumbbell bench press": "benchPress",
  "incline bench press": "inclinePress",
  "dumbbell incline press": "inclinePress",
  "machine chest press": "machinePress",
  "push-up": "pushUp",
  dip: "dip",
  "cable fly": "cableFly",

  // shoulders
  "overhead press": "overheadPress",
  "dumbbell shoulder press": "overheadPress",
  "arnold press": "overheadPress",
  "lateral raise": "lateralRaise",
  "upright row": "uprightRow",

  // arms
  "barbell curl": "curl",
  "dumbbell curl": "curl",
  "hammer curl": "curl",
  "preacher curl": "preacherCurl",
  "triceps pushdown": "pushdown",
  "overhead triceps extension": "overheadExtension",
  "skull crusher": "skullCrusher",
  "close-grip bench press": "benchPress",

  // core
  plank: "plank",
  "hanging leg raise": "hangingLegRaise",
  "cable crunch": "cableCrunch",
  "ab wheel rollout": "abWheel",
};

/**
 * Movements deliberately left without a figure, so a later reader knows they
 * were considered rather than forgotten.
 *
 * The bar to clear: a lifter who did not already know the movement should be
 * able to tell its figure apart from its neighbours in the library. Something
 * that fails that is worse than nothing — an emoji promises nothing, but a
 * demonstration that could be three different exercises teaches confusion.
 */
const NO_HONEST_FIGURE: readonly string[] = [
  // A seated trunk rotation is invisible from the side and reads as "sitting
  // still" from the front. The movement is in the plane neither view shows.
  "russian twist",

  // Scapular elevation is a couple of inches on a body drawn at a hundred
  // units. Exaggerating it enough to see would teach a shrug that does not
  // exist, and drawn honestly it is a figure standing perfectly still.
  "barbell shrug",

  // Both are transverse: the arms travel toward and away from the viewer. From
  // the side a lying fly is a bench press, and from the front a bent-over rear
  // delt fly is a lateral raise. Cable Fly keeps a figure because it is done
  // standing, where the sweep across the chest genuinely reads from the front.
  "dumbbell fly",
  "rear delt fly",
];

/** The demonstration for an exercise, or undefined where there is none. */
export function exerciseDemo(name: string): SampledArchetype | undefined {
  const key = ARCHETYPE_BY_EXERCISE[name.trim().toLowerCase()];
  return key ? ARCHETYPES[key] : undefined;
}

/** Whether a movement was deliberately left undrawn. Used by the coverage test. */
export function isDeliberatelyUndrawn(name: string): boolean {
  return NO_HONEST_FIGURE.includes(name.trim().toLowerCase());
}
