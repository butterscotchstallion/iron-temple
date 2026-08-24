import type { Layoff } from "./api";

// Wording for the layoff prompt.
//
// The server decides everything that matters here — how long the lifter has
// been away and how much comes off if they say yes — so this file computes
// nothing. It only has to say the numbers out loud in a way that reads like a
// sentence, which is the part that goes wrong on its own: "1 weeks" and
// "30.000000000000004%" are both one careless template literal away.

/**
 * A percentage fraction as a whole-number label: 0.3 -> "30%".
 *
 * Rounded rather than trusted. The wire carries a fraction and the API computes
 * it in whole percent precisely so this multiplication comes out clean, but a
 * float that has been through JSON is not a promise, and a badge is not the
 * place to find out.
 */
export function pctLabel(pct: number): string {
  return `${Math.round(pct * 100)}%`;
}

/** How long they've been away, in words: "a week", "3 weeks". */
export function weeksLabel(weeks: number): string {
  return weeks === 1 ? "a week" : `${weeks} weeks`;
}

/** The prompt's headline, e.g. "It's been 3 weeks since you trained". */
export function layoffHeadline(layoff: Layoff): string {
  return `It's been ${weeksLabel(layoff.weeks)} since you trained`;
}

/** The label on the accept button, e.g. "Deload 30%". */
export function deloadLabel(layoff: Layoff): string {
  return `Deload ${pctLabel(layoff.deloadPct)}`;
}

/**
 * Whether to put the question on screen. A null layoff is the server saying
 * there is nothing to ask about; `decided` is the lifter having already
 * answered, either way, on this visit.
 *
 * The answer is not remembered any longer than that, and deliberately: training
 * ends the layoff, so the prompt retires itself the moment it is acted on. The
 * one case that survives — saying no and then reloading — asks again, which is
 * the right way round. A dismissal that outlived the page could hide the prompt
 * through the whole layoff it exists to flag.
 */
export function shouldPrompt(
  layoff: Layoff | null | undefined,
  decided: boolean,
): boolean {
  return !decided && layoff != null && layoff.weeks > 0;
}
