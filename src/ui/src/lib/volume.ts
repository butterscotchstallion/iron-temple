/**
 * Volume — the total weight moved, reps × weight summed over a workout or a
 * career. The API computes it (SessionSummary.volumeLb, SessionList
 * .totalVolumeLb); this is only how it reads on screen.
 */

const grouped = new Intl.NumberFormat("en-US");

/**
 * Format a volume in pounds: whole pounds, comma-grouped, no unit suffix — the
 * "lb" belongs in the markup, as it does everywhere else weights are rendered.
 *
 * Rounded because volume is rarely a round number (a 45.5 lb bar for 5 reps is
 * 227.5) and nobody reads a half-pound of a five-figure total. Anything that
 * isn't a finite non-negative number reads as "0" rather than "NaN" or a
 * negative, so a bad or missing field can't put nonsense on the page.
 */
export function formatVolume(lb: number): string {
  if (!Number.isFinite(lb) || lb <= 0) return "0";
  return grouped.format(Math.round(lb));
}
