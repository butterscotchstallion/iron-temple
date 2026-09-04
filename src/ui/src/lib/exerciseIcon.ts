// Known lift name → emoji. Matched case-insensitively as a substring of the
// exercise name, so "Barbell Bench Press" still resolves to the bench icon.
// Order matters: the first match wins, so more specific keys come first.
const EMOJI_BY_KEYWORD: ReadonlyArray<[string, string]> = [
  // The barbell lifts the programs prescribe.
  ["squat", "🦵"],
  ["deadlift", "🏋️"],
  ["bench", "💪"],
  // Assistance work from the exercise library. Ordering inside this block is
  // load-bearing for the same reason as the block itself: "Leg Curl" contains
  // "curl", and it is a hamstring movement, so the leg keys have to be read
  // first. Likewise "Standing Calf Raise" must reach "calf" before "raise".
  ["calf", "🦶"],
  ["leg press", "🦵"],
  ["leg curl", "🦵"],
  ["leg extension", "🦵"],
  ["lunge", "🦵"],
  ["curl", "💪"],
  ["triceps", "💪"],
  ["skull crusher", "💪"],
  ["dip", "🤸"],
  ["push-up", "🤸"],
  ["pull-up", "🧗"],
  ["chin-up", "🧗"],
  ["pulldown", "🧗"],
  ["face pull", "🎯"],
  ["shrug", "🤷"],
  ["fly", "🦅"],
  ["plank", "🧘"],
  ["crunch", "🧘"],
  ["twist", "🧘"],
  ["ab wheel", "🎡"],
  ["raise", "🙆"],
  ["extension", "🙆"],
  // Generic fallbacks, last so every specific movement above wins: a face pull
  // is not a barbell row, and a leg press is not an overhead press.
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
