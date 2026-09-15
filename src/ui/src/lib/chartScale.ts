/**
 * The arithmetic every hand-drawn SVG chart on this app needs: map a number
 * onto a pixel span, and label an axis with dates.
 *
 * Three charts draw themselves from scratch — ProgressChart, BodyweightChart and
 * LiftTrendChart — because a charting library is a lot of bytes to send a phone
 * for three plots that each want something slightly different. What they do NOT
 * want differently is where a point lands or how a date reads on a tick, and
 * those had drifted into three copies apiece. They live here now.
 *
 * Nothing here decides a domain. How far an axis runs, and whether it is anchored
 * to zero, is the one genuinely per-chart judgement — see the notes on each
 * chart's yLo/yHi — so each keeps its own and hands the result to `scale`.
 */

import { isoDayNumber, isoParts } from "./date";

export { isoDayNumber };

/**
 * Map `value` from the data domain [lo, hi] onto the pixel range [from, to].
 *
 * `to` below `from` is normal rather than a mistake: SVG y grows downward, so a
 * y axis is drawn by passing the baseline as `from` and the top as `to`, and the
 * inversion falls out of the arithmetic.
 *
 * A zero-width domain centres the value in the range. That is the single-point
 * case — one weigh-in, one session — where there is no spread to position
 * against, and pinning it to the left edge or the baseline would draw a lone dot
 * in the corner of an otherwise empty plot and imply it sat at the minimum.
 */
export function scale(
  value: number,
  lo: number,
  hi: number,
  from: number,
  to: number,
): number {
  if (hi === lo) return (from + to) / 2;
  return from + ((value - lo) / (hi - lo)) * (to - from);
}

/**
 * A date on an axis tick: "9/18". Month and day only — the year is the same for
 * every point in any period these charts draw, and spelling it out three times
 * across a 320-unit axis costs room the labels do not have.
 */
export function shortDate(iso: string): string {
  const [, month, day] = isoParts(iso);
  return `${month}/${day}`;
}

/**
 * Up to three dates to tick: the ends, and the middle when there is room.
 *
 * Deduplicated and sorted, because two sessions on one day are two points and
 * one date, and a tick drawn twice paints the label over itself.
 */
export function dateTicks(dates: string[]): string[] {
  const all = [...new Set(dates)].sort();
  if (all.length <= 2) return all;
  return [all[0], all[Math.floor((all.length - 1) / 2)], all[all.length - 1]];
}
