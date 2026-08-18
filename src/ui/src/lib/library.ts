import type { Exercise, MuscleGroup, Equipment } from "./api";

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
  other: "Other",
};

/** Display name for a muscle group; unknown values pass through unchanged. */
export function muscleGroupLabel(group: string): string {
  return MUSCLE_GROUP_LABELS[group as MuscleGroup] ?? group;
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
