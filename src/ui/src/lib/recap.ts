/**
 * Formatting and view models particular to the session recap. Everything the
 * monthly recap already spells — durations, signed percentages, volumes —
 * comes from racked.ts and volume.ts; only what that page has no equivalent of
 * lives here.
 */

/**
 * One lift's row, as the recap draws it.
 *
 * A view model rather than either wire type, because the recap has two sources:
 * the server's SessionRecap, and localRecap's reconstruction from the session
 * alone when there is no signal. The components take this, and the route maps
 * whichever it has into it — so "we could not ask the server" is decided once,
 * in the route, instead of every component having to guess what a null means.
 */
export type RecapLiftRow = {
  exerciseId: number;
  exerciseName: string;
  kind: "main" | "assistance";
  topWeightLb: number;
  topReps: number;
  setsLogged: number;
  setsPrescribed: number;
  repsLogged: number;
  repsTargeted: number;
  hitEveryTarget: boolean;
  /** Null when the lift is new to this day, and null offline. */
  previousTopWeightLb: number | null;
  weightDeltaPct: number | null;
};

/** One record, from either source. `kind` is always "weight" offline. */
export type RecapPRRow = {
  exerciseName: string;
  kind: "weight" | "e1rm";
  valueLb: number;
  previousLb: number;
};

/**
 * How this session's pace read: "12% faster", "8% slower", "about the same".
 *
 * Words rather than a signed percentage, and deliberately NOT formatDelta.
 * Pace is the one figure in the app where a negative number is the good news —
 * deltaPct is (this − median) / median, so a quick workout is negative — and
 * formatDelta would print "−12%" next to it. In a column where every other
 * minus sign means a loss, that reads as one.
 *
 * The dead band is not cosmetic either. A 48-minute workout swinging 2% is
 * under a minute, which is one conversation by the rack; calling that "faster"
 * would make the line meaningless by making it always say something.
 */
export function formatPace(deltaPct: number): string {
  if (!Number.isFinite(deltaPct)) return "about the same";
  const pct = Math.round(Math.abs(deltaPct) * 100);
  if (Math.abs(deltaPct) < 0.05 || pct === 0) return "about the same";
  return deltaPct < 0 ? `${pct}% faster` : `${pct}% slower`;
}

/**
 * A placing: 1 -> "1st", 3 -> "3rd", 11 -> "11th".
 *
 * Hand-rolled rather than Intl.PluralRules with ordinal type, which needs a
 * locale and a rule table to produce four English suffixes. The teens are the
 * only trap — 11th, 12th and 13th do not follow their last digit — and they are
 * one condition.
 */
export function formatOrdinal(n: number): string {
  if (!Number.isFinite(n) || n < 1) return "—";
  const i = Math.floor(n);
  const tens = i % 100;
  if (tens >= 11 && tens <= 13) return `${i}th`;
  switch (i % 10) {
    case 1:
      return `${i}st`;
    case 2:
      return `${i}nd`;
    case 3:
      return `${i}rd`;
    default:
      return `${i}th`;
  }
}

/**
 * Reps hit against reps asked for: "23 / 25".
 *
 * Its own function rather than interpolation at each call site because the
 * recap draws this pair four times over — once for the session and once per
 * lift — and the spacing around the slash is the sort of thing that drifts.
 */
export function formatOutOf(done: number, total: number): string {
  if (!Number.isFinite(done) || !Number.isFinite(total) || total <= 0) return "—";
  return `${Math.max(0, Math.round(done))} / ${Math.round(total)}`;
}
