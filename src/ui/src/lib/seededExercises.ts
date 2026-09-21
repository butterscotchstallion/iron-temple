/**
 * Every exercise the seed migrations put in the catalogue.
 *
 * A hand-kept mirror of src/api/db/migrations/0002, 0007, 0009 and 0023, used
 * only by tests — nothing in the running app reads this, because the app reads
 * the real catalogue from the API.
 *
 * It exists so the figure coverage test can say "every seeded movement either
 * has a demonstration or is on the record as refused one". That check is worth
 * having even though the mirror can drift: when a future migration adds a
 * movement, this list is what someone has to notice, and a list is easier to
 * notice than an absence.
 */
export const SEEDED_EXERCISES: readonly string[] = [
  // arms
  "Barbell Curl",
  "Close-Grip Bench Press",
  "Dumbbell Curl",
  "Hammer Curl",
  "Overhead Triceps Extension",
  "Preacher Curl",
  "Skull Crusher",
  "Triceps Pushdown",
  // back
  "Back Extension",
  "Barbell Row",
  "Barbell Shrug",
  "Chin-Up",
  "Deadlift",
  "Dumbbell Row",
  "Face Pull",
  "Lat Pulldown",
  "Pause Deadlift",
  "Pull-Up",
  "Seated Cable Row",
  "T-Bar Row",
  // chest
  "Bench Press",
  "Cable Fly",
  "Dip",
  "Dumbbell Bench Press",
  "Dumbbell Fly",
  "Dumbbell Incline Press",
  "Feet-Up Bench Press",
  "Incline Bench Press",
  "Machine Chest Press",
  "Pause Bench Press",
  "Push-Up",
  // core
  "Ab Wheel Rollout",
  "Cable Crunch",
  "Hanging Leg Raise",
  "Plank",
  "Russian Twist",
  // legs
  "Banded Glute Bridge",
  "Banded Hip Abduction",
  "Banded Kickback",
  "Banded Lateral Walk",
  "Barbell Hip Thrust",
  "Bulgarian Split Squat",
  "Front Squat",
  "Goblet Squat",
  "Leg Curl",
  "Leg Extension",
  "Leg Press",
  "Pause Squat",
  "Romanian Deadlift",
  "Squat",
  "Standing Calf Raise",
  "Walking Lunge",
  // shoulders
  "Arnold Press",
  "Dumbbell Shoulder Press",
  "Lateral Raise",
  "Overhead Press",
  "Rear Delt Fly",
  "Upright Row",
];
