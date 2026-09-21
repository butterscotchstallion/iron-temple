import { STREAK_DISPLAY_THRESHOLD } from "./streak";

/**
 * How hot a streak looks. `streak.ts` decides what counts; this decides what the
 * card does about it.
 *
 * The ladder has three rungs, and each one has to earn its place — a card that
 * looks the same at 3 sessions as at 30 stops rewarding you the moment it
 * appears:
 *
 *   3       the card shows up at all, faint (STREAK_DISPLAY_THRESHOLD)
 *   4-5     brightening, colour shifting toward magenta
 *   6       ignition — flames, embers, hot orange
 *   7-11    flames taller, faster, brighter
 *   12+     full blaze, and it holds there
 */

/** Where the card catches fire. */
export const STREAK_FIRE_THRESHOLD = 6;

/**
 * Where the blaze tops out. Past this the card stops escalating: there has to be
 * a ceiling, and a lifter forty sessions in should not be reading text through a
 * wall of flame.
 */
export const STREAK_PEAK = 12;

export type StreakHeat = {
  /** 0 at the display threshold, 1 at ignition. Drives brightness and tint. */
  glow: number;
  /** 0 at ignition, 1 at the peak. Drives flame height, speed and opacity. */
  blaze: number;
  /** Whether the card is on fire at all. */
  ablaze: boolean;
};

/** Where `value` sits between `from` and `to`, clamped to the ends. */
function ramp(value: number, from: number, to: number): number {
  return Math.min(1, Math.max(0, (value - from) / (to - from)));
}

export function streakHeat(streak: number): StreakHeat {
  return {
    glow: ramp(streak, STREAK_DISPLAY_THRESHOLD, STREAK_FIRE_THRESHOLD),
    blaze: ramp(streak, STREAK_FIRE_THRESHOLD, STREAK_PEAK),
    ablaze: streak >= STREAK_FIRE_THRESHOLD,
  };
}

/**
 * The two ramps as a single 0-1 figure: 0 at the display threshold, 0.5 at
 * ignition, 1 at the peak. This is what the colour reads from, so that hue moves
 * continuously across the whole ladder rather than jumping at 6.
 */
export function combinedHeat(heat: StreakHeat): number {
  return (heat.glow + heat.blaze) / 2;
}

/**
 * The colour ramp, in OKLCH: the app's neon purple through magenta into fire.
 *
 * OKLCH rather than sRGB because a channel-wise blend from purple to orange
 * travels through the middle of the cube and spends the trip looking like mud.
 * Interpolating hue and chroma separately keeps every intermediate step a
 * saturated colour someone chose.
 *
 * The hues run 308 -> 346 -> 409 -> 435, which is the same arc as 308 -> 346 ->
 * 49 -> 75 written so it never wraps. That is deliberate: a monotonically
 * increasing sequence makes the interpolation plain arithmetic and guarantees
 * the short way round (through red) rather than back down through green.
 * The emitted string is reduced mod 360 — CSS Color 4 says a UA normalizes an
 * out-of-range hue itself, but a colour the whole card is keyed to is not the
 * place to lean on that, and the modulo costs nothing.
 *
 * Coordinates are converted from the palette hexes, not eyeballed:
 * #b026ff (--color-neon), #ff2fb9 (--color-magenta), #ff7a18, #ffb020.
 */
const STOPS = [
  { at: 0, L: 0.601, C: 0.286, H: 307.98 }, // neon purple — the card today
  { at: 0.34, L: 0.68, C: 0.264, H: 346.22 }, // magenta, around streak 5
  { at: 0.5, L: 0.724, C: 0.187, H: 409.09 }, // hot orange — ignition at 6
  { at: 1, L: 0.813, C: 0.165, H: 435.04 }, // amber, the peak at 12
];

export function heatColor(heat: number): string {
  const t = Math.min(1, Math.max(0, heat));
  // The last stop whose position we've passed, capped so the final segment has
  // somewhere to interpolate to at t = 1.
  let i = 0;
  while (i < STOPS.length - 2 && t > STOPS[i + 1].at) i += 1;
  const a = STOPS[i];
  const b = STOPS[i + 1];
  const f = (t - a.at) / (b.at - a.at);
  const mix = (from: number, to: number) => from + (to - from) * f;
  const round = (n: number, places: number) => Number(n.toFixed(places));
  return `oklch(${round(mix(a.L, b.L), 4)} ${round(mix(a.C, b.C), 4)} ${round(
    mix(a.H, b.H) % 360,
    2,
  )})`;
}
