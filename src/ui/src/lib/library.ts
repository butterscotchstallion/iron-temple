import type { Exercise, MuscleGroup, Equipment } from "./api";
import { DEFAULT_BAR_STEP_LB } from "./plates";

/**
 * Pure helpers behind the exercise library: how the catalogue is labelled,
 * searched and grouped. Kept out of the component so the rules can be tested
 * without rendering anything, the way warmup.ts and streak.ts are.
 */

/**
 * Muscle groups in the order the library lists them — the conventional
 * push/pull/legs reading order rather than alphabetical, so a lifter scanning
 * for "where do I find dips" looks in the place they'd expect. "other" is last
 * because it is the fallback bucket, and custom exercises land in it most.
 */
export const MUSCLE_GROUPS: readonly MuscleGroup[] = [
  "chest",
  "back",
  "legs",
  "shoulders",
  "arms",
  "core",
  "other",
];

export const EQUIPMENT: readonly Equipment[] = [
  "barbell",
  "dumbbell",
  "machine",
  "cable",
  "bodyweight",
  "band",
  "other",
];

const MUSCLE_GROUP_LABELS: Record<MuscleGroup, string> = {
  chest: "Chest",
  back: "Back",
  legs: "Legs",
  shoulders: "Shoulders",
  arms: "Arms",
  core: "Core",
  other: "Other",
};

const EQUIPMENT_LABELS: Record<Equipment, string> = {
  barbell: "Barbell",
  dumbbell: "Dumbbell",
  machine: "Machine",
  cable: "Cable",
  bodyweight: "Bodyweight",
  band: "Band",
  other: "Other",
};

/** Display name for a muscle group; unknown values pass through unchanged. */
export function muscleGroupLabel(group: string): string {
  return MUSCLE_GROUP_LABELS[group as MuscleGroup] ?? group;
}

/**
 * The smallest weight change each kind of equipment admits in ONE lifter's gym,
 * as WHOLE load — the pair, not the bell; the bar and both sides of it, not one
 * plate.
 *
 * Mirrors `progression.GymSteps` on the API side, field for field, including
 * the rule that a missing or zero value means "not configured" and falls back
 * to the constant rather than to nothing.
 */
export type GymSteps = {
  /** Twice the lightest plate owned. See `barStepLb`. */
  barLb?: number;
  /** Twice the rack's per-bell step. */
  dumbbellLb?: number;
  /**
   * The gap between two holes on a stack. Already the whole load — a lifter
   * holds two dumbbells and moves one pin, so unlike `dumbbellLb` this is not
   * doubled on its way here.
   */
  machineLb?: number;
  /** The next plate up on a cable stack. Whole load. */
  cableLb?: number;
  /** The jump between two bands in a graded set. Whole load. */
  bandLb?: number;
};

/** The smallest change a pair of dumbbells admits when the rack is unknown. */
export const DEFAULT_DUMBBELL_STEP_LB = 10;

/**
 * What a stack steps by when the lifter has not said.
 *
 * One constant for three kinds, mirroring `defaultEquipmentStepLb` on the API
 * side and the three column defaults in 0028 — the fallback is a single fact,
 * "we were not told", where the stored values are three separate ones.
 */
export const DEFAULT_EQUIPMENT_STEP_LB = 5;

/**
 * The smallest weight change a kind of equipment admits, in lb.
 *
 * Both numbers used to be constants here — five for a barbell, ten for a pair
 * of dumbbells — and neither is one. Five is twice a 2.5 lb plate, which is
 * only the lightest plate if that is what you happen to own; ten is twice a
 * 5 lb bell, which is only the rack's step if that is the rack you have. A
 * lifter with finer equipment was being promised, and prescribed, jumps coarser
 * than their gym actually makes. Both now come off the profile — see
 * `gymSteps` — and the constants are the fallback for a client that has not
 * loaded one yet.
 *
 * Machines, cables and bands have their own grids too, since 0028. They used to
 * fall through to the bar's, which said that buying a pair of 1.25 lb plates
 * made the leg press finer — it does not, because a stack is a stack and the
 * pin drops into holes somebody else drilled.
 *
 * Bodyweight and other still take the bar's, and so does any kind the catalogue
 * grows later: their load, when they have one, is plates or bells, both of which
 * are described already.
 *
 * This mirrors progression.stepFor on the API side, which is what actually moves
 * the weight — the switch below and that one have to keep agreeing, because one
 * draws the number and the other delivers it. It exists here so the copy that
 * promises a lifter a number can promise the one they will get.
 */
export function equipmentStepLb(equipment: string, steps: GymSteps = {}): number {
  switch (equipment) {
    case "dumbbell":
      return steps.dumbbellLb || DEFAULT_DUMBBELL_STEP_LB;
    case "machine":
      return steps.machineLb || DEFAULT_EQUIPMENT_STEP_LB;
    case "cable":
      return steps.cableLb || DEFAULT_EQUIPMENT_STEP_LB;
    case "band":
      return steps.bandLb || DEFAULT_EQUIPMENT_STEP_LB;
    default:
      return steps.barLb || DEFAULT_BAR_STEP_LB;
  }
}

/** Display name for an equipment kind; unknown values pass through unchanged. */
export function equipmentLabel(equipment: string): string {
  return EQUIPMENT_LABELS[equipment as Equipment] ?? equipment;
}

/**
 * Whether an exercise matches a search box. Case- and whitespace-insensitive
 * substring matching on the name, which is what a lifter typing "curl" means;
 * an empty query matches everything so the caller needn't special-case it.
 */
export function matchesSearch(exercise: Pick<Exercise, "name">, query: string): boolean {
  const needle = query.trim().toLowerCase();
  if (needle === "") return true;
  return exercise.name.toLowerCase().includes(needle);
}

export type ExerciseGroup = {
  group: MuscleGroup;
  label: string;
  exercises: Exercise[];
};

/**
 * Filter the library by search text and (optionally) one muscle group, then
 * group what's left under its muscle group in MUSCLE_GROUPS order.
 *
 * Empty groups are dropped rather than rendered as bare headings, so a search
 * that matches two lifts shows two lifts and not five empty sections. Exercises
 * keep the order they arrived in, which is alphabetical from the API.
 */
export function groupExercises(
  exercises: Exercise[],
  options: { query?: string; group?: MuscleGroup | null } = {},
): ExerciseGroup[] {
  const { query = "", group = null } = options;

  const buckets = new Map<MuscleGroup, Exercise[]>();
  for (const exercise of exercises) {
    if (group !== null && exercise.muscleGroup !== group) continue;
    if (!matchesSearch(exercise, query)) continue;
    const key = exercise.muscleGroup;
    const bucket = buckets.get(key);
    if (bucket) bucket.push(exercise);
    else buckets.set(key, [exercise]);
  }

  return MUSCLE_GROUPS.filter((g) => buckets.has(g)).map((g) => ({
    group: g,
    label: muscleGroupLabel(g),
    exercises: buckets.get(g) ?? [],
  }));
}

/**
 * How many exercises sit in each muscle group, for the filter chips. Counts the
 * whole library rather than the current search, so the chips hold still while
 * you type instead of collapsing toward zero under you.
 */
export function countByGroup(exercises: Exercise[]): Record<MuscleGroup, number> {
  const counts = Object.fromEntries(
    MUSCLE_GROUPS.map((g) => [g, 0]),
  ) as Record<MuscleGroup, number>;
  for (const exercise of exercises) {
    if (exercise.muscleGroup in counts) counts[exercise.muscleGroup] += 1;
  }
  return counts;
}

/**
 * How many movements the assistance picker's "Recent" section holds. Six fits
 * inside the list's scroll box, so the muscle groups underneath stay visible and
 * the section reads as a shortcut rather than as the whole list.
 */
export const RECENT_LIMIT = 6;

/**
 * The accessories this lifter trains, most recently performed first — what the
 * assistance picker leads with so the same handful of movements needn't be
 * searched for every time one is added to a day.
 *
 * Accessory work only. A program lift is performed every week, so it would hold
 * the top of the section permanently while being the one thing nobody adds
 * there: the program already prescribes it, and the picker's caller excludes it
 * for exactly that reason.
 *
 * The date test is truthiness rather than `!== null`, which matters more than it
 * looks: a caller whose exercises predate the field would have every one of them
 * promoted, since `undefined !== null`.
 *
 * Ordering is total, so the section holds still between loads: last performed,
 * then the number of sessions the lift has been trained in — a whole workout's
 * accessories share one date, and how often is the better tie-break for "the
 * ones I like" — then the name.
 */
export function recentExercises(
  exercises: Exercise[],
  limit: number = RECENT_LIMIT,
): Exercise[] {
  return exercises
    .filter((exercise) => exercise.isAccessory && Boolean(exercise.lastPerformedOn))
    .sort(
      (a, b) =>
        (b.lastPerformedOn ?? "").localeCompare(a.lastPerformedOn ?? "") ||
        b.performedSessions - a.performedSessions ||
        a.name.localeCompare(b.name),
    )
    .slice(0, limit);
}

/**
 * The line under an exercise's name in the library: its equipment, plus a note
 * for the lifts a program prescribes. Marking those matters because they are the
 * ones the progression engine drives — adding a squat as assistance to a program
 * that already squats is a thing worth thinking twice about.
 */
export function exerciseSubtitle(
  exercise: Pick<Exercise, "equipment" | "isAccessory" | "isCustom">,
): string {
  const parts = [equipmentLabel(exercise.equipment)];
  if (!exercise.isAccessory) parts.push("Program lift");
  if (exercise.isCustom) parts.push("Yours");
  return parts.join(" · ");
}
