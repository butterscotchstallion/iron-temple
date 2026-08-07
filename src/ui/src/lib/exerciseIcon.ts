import type { ExerciseHistoryPoint } from "./api";

// Known lift name → emoji. Matched case-insensitively as a substring of the
// exercise name, so "Barbell Bench Press" still resolves to the bench icon.
// Order matters: the first match wins, so more specific keys come first.
const EMOJI_BY_KEYWORD: ReadonlyArray<[string, string]> = [
  ["squat", "🦵"],
  ["deadlift", "🏋️"],
  ["bench", "💪"],
  ["row", "🚣"],
  ["overhead", "🙌"],
  ["press", "🙌"],
];

// Fallback for anything we don't recognise.
const DEFAULT_EMOJI = "🏋️";

/** An emoji roughly representing the exercise, keyed off its name. */
export function exerciseEmoji(name: string): string {
  const lower = name.toLowerCase();
  for (const [keyword, emoji] of EMOJI_BY_KEYWORD) {
    if (lower.includes(keyword)) return emoji;
  }
  return DEFAULT_EMOJI;
}

/**
 * The heaviest top set in an exercise's history, or null when there is none.
 * On ties the first occurrence wins; since history is oldest-first that is the
 * session that originally set the PR.
 */
export function topSet(
  points: ExerciseHistoryPoint[],
): { weightLb: number; performedOn: string } | null {
  let best: ExerciseHistoryPoint | null = null;
  for (const p of points) {
    if (!best || p.weightLb > best.weightLb) best = p;
  }
  return best ? { weightLb: best.weightLb, performedOn: best.performedOn } : null;
}
