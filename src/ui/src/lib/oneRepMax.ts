/**
 * Estimated one-rep max via the Epley formula: weight × (1 + reps/30), rounded
 * to the nearest pound. Returns 0 for non-positive inputs.
 *
 * A single is worth what was on the bar and nothing more. Epley extrapolates
 * from a set carried past one rep, and at exactly one rep it inflated a known
 * number by a thirtieth — a genuine 225 single read as a 233 estimated max, and
 * ranked above a 225 double, which is plainly the harder set. Set.E1RM in the
 * API and the RackedExerciseBaseline query carry the same exception, because a
 * record is decided by comparing one against the other.
 */
export function estimateOneRepMax(weightLb: number, reps: number): number {
  if (weightLb <= 0 || reps <= 0) return 0;
  if (reps === 1) return Math.round(weightLb);
  return Math.round(weightLb * (1 + reps / 30));
}
