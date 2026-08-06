/**
 * Estimated one-rep max via the Epley formula: weight × (1 + reps/30), rounded
 * to the nearest pound. Returns 0 for non-positive inputs.
 */
export function estimateOneRepMax(weightLb: number, reps: number): number {
  if (weightLb <= 0 || reps <= 0) return 0;
  return Math.round(weightLb * (1 + reps / 30));
}
