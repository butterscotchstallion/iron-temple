/**
 * Presentation helpers for the Racked recap.
 *
 * The statistics themselves are computed server-side (internal/racked) so the
 * page and the monthly email cannot disagree. Nothing here recomputes a number —
 * these only decide how one reads, and how a lift maps onto a chart colour.
 */

/** How many categorical colours the theme defines (--chart-1 … --chart-5). */
export const CHART_SLOTS = 5;

export type SeriesPoint = { performedOn: string; topWeightLb: number; e1rmLb: number };
export type LiftSeries = { exerciseId: number; exerciseName: string; points: SeriesPoint[] };

export type IndexedPoint = { performedOn: string; pct: number };
export type IndexedSeries = {
  exerciseId: number;
  exerciseName: string;
  color: string;
  points: IndexedPoint[];
};

/**
 * Turn per-lift estimated-max series into percentage change from each lift's
 * first session in the period.
 *
 * Indexed rather than absolute because these lifts share one axis and do not
 * share a scale: a deadlift works near 400 lb while a press works near 100, so
 * plotting pounds squashes the press into a flat line at the bottom of the
 * chart and hides exactly the progress the recap is about. A second y-axis
 * would be the usual escape and is worse — two scales on one plot invite
 * comparing slopes that are not comparable. Percent puts every lift on the same
 * axis honestly, and "improvement" is what the chart claims to show anyway.
 *
 * A lift needs two sessions to have a trend, and a first estimate above zero to
 * have a denominator; anything else is dropped rather than drawn as a point
 * with no line.
 *
 * `limit` caps how many lifts are drawn, because the theme defines a fixed
 * number of categorical colours and cycling them would put one colour on two
 * lifts. The caller is told how many were left out so the page can say so
 * rather than quietly showing a subset.
 *
 * `volumeByExercise` decides which lifts survive that cap: the ones the lifter
 * did the most work on. Ranking by the weight on the bar instead — which this
 * used to do — meant the deadlift and squat always survived and the overhead
 * press was always cut, on a chart whose entire subject is percentage gain and
 * where the press is usually the biggest gainer. Volume asks "what did you
 * actually spend the month doing", which is the right question for what to show,
 * and it leaves a stalled main lift on the chart where a gain-ranked selection
 * would drop it just for being flat.
 */
export function indexedSeries(
  series: LiftSeries[],
  volumeByExercise: Map<number, number> = new Map(),
  limit: number = CHART_SLOTS,
): { shown: IndexedSeries[]; hidden: number } {
  const chartable = series.filter((s) => s.points.length >= 2 && s.points[0].e1rmLb > 0);

  // Selected by volume, then re-sorted by exercise id before colours are handed
  // out, so a lift keeps its colour when the period changes underneath it.
  // Colour follows the lift, never its rank in a list.
  const volume = (s: LiftSeries) => volumeByExercise.get(s.exerciseId) ?? 0;
  const selected = [...chartable]
    .sort((a, b) => volume(b) - volume(a) || a.exerciseId - b.exerciseId)
    .slice(0, limit)
    .sort((a, b) => a.exerciseId - b.exerciseId);

  const shown = selected.map((s, i) => ({
    exerciseId: s.exerciseId,
    exerciseName: s.exerciseName,
    color: seriesColor(i),
    points: s.points.map((p) => ({
      performedOn: p.performedOn,
      pct: (p.e1rmLb - s.points[0].e1rmLb) / s.points[0].e1rmLb,
    })),
  }));

  return { shown, hidden: chartable.length - shown.length };
}

/**
 * The CSS variable for a categorical slot. Slots are never cycled — a caller
 * that needs more series than the theme has colours must drop or group them
 * instead, so an out-of-range index is clamped rather than wrapped.
 */
export function seriesColor(index: number): string {
  const slot = Math.min(Math.max(index, 0), CHART_SLOTS - 1) + 1;
  return `var(--chart-${slot})`;
}

/**
 * A session length, in the units a lifter would say out loud: "48m", "1h 12m".
 * time.ts's formatTime is M:SS for the rest timer, where seconds matter; here
 * they are noise, and "48:00" would read as 48 hours as easily as 48 minutes.
 */
export function formatSessionLength(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return "—";
  const total = Math.round(seconds / 60);
  const hours = Math.floor(total / 60);
  const minutes = total % 60;
  if (hours === 0) return `${minutes}m`;
  if (minutes === 0) return `${hours}h`;
  return `${hours}h ${minutes}m`;
}

/**
 * A signed percentage from a fraction: 0.083 -> "+8%". Uses a real minus sign
 * rather than a hyphen so the figure lines up in tabular-nums columns.
 */
export function formatDelta(fraction: number): string {
  if (!Number.isFinite(fraction)) return "—";
  const pct = Math.round(fraction * 100);
  if (pct === 0) return "0%";
  return pct > 0 ? `+${pct}%` : `−${Math.abs(pct)}%`;
}

/** An unsigned percentage from a fraction: 0.86 -> "86%". */
export function formatPercent(fraction: number): string {
  if (!Number.isFinite(fraction) || fraction < 0) return "0%";
  return `${Math.round(fraction * 100)}%`;
}

/** An hour of the day as a label: 0 -> "12am", 6 -> "6am", 18 -> "6pm". */
export function formatHour(hour: number): string {
  const h = ((Math.round(hour) % 24) + 24) % 24;
  const suffix = h < 12 ? "am" : "pm";
  const display = h % 12 === 0 ? 12 : h % 12;
  return `${display}${suffix}`;
}

/**
 * Scale a value to a 0–1 share of the largest in a set, for bar lengths. An
 * all-zero set yields all zeros rather than dividing by zero and painting every
 * bar full width.
 */
export function barFraction(value: number, max: number): number {
  if (!Number.isFinite(value) || !Number.isFinite(max) || max <= 0 || value <= 0) return 0;
  return Math.min(value / max, 1);
}

/**
 * A training frequency, to one decimal: 2.75 -> "2.8". The whole number loses
 * the difference between twice a week and nearly three times, which is most of
 * what the figure is for.
 */
export function formatPerWeek(perWeek: number): string {
  if (!Number.isFinite(perWeek) || perWeek <= 0) return "0";
  return perWeek.toFixed(1);
}
