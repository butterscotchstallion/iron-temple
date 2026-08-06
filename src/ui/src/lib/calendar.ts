export type CalendarDay = { date: string; count: number };
export type CalendarWeek = CalendarDay[]; // 7 days, Sunday → Saturday

function parseIso(iso: string): Date {
  const [y, m, d] = iso.split("-").map(Number);
  return new Date(y, m - 1, d);
}

function toIso(dt: Date): string {
  const y = dt.getFullYear();
  const m = String(dt.getMonth() + 1).padStart(2, "0");
  const d = String(dt.getDate()).padStart(2, "0");
  return `${y}-${m}-${d}`;
}

function addDays(dt: Date, n: number): Date {
  return new Date(dt.getFullYear(), dt.getMonth(), dt.getDate() + n);
}

/** Today as a YYYY-MM-DD string (local time). */
export function todayIso(): string {
  return toIso(new Date());
}

/**
 * Build a `weeks` × 7 grid (GitHub-style) of per-day session counts, ending at
 * the week containing `endIso`. Columns are weeks (oldest → newest); rows are
 * Sunday → Saturday. Dates are compared as YYYY-MM-DD strings, computed in local
 * time to avoid timezone drift.
 */
export function buildCalendar(
  sessionDates: string[],
  endIso: string,
  weeks = 16,
): CalendarWeek[] {
  const counts = new Map<string, number>();
  for (const d of sessionDates) counts.set(d, (counts.get(d) ?? 0) + 1);

  const end = parseIso(endIso);
  const endOfWeek = addDays(end, 6 - end.getDay()); // Saturday of end's week
  let cursor = addDays(endOfWeek, -(weeks * 7 - 1)); // Sunday, `weeks` back

  const grid: CalendarWeek[] = [];
  for (let w = 0; w < weeks; w++) {
    const week: CalendarDay[] = [];
    for (let d = 0; d < 7; d++) {
      const iso = toIso(cursor);
      week.push({ date: iso, count: counts.get(iso) ?? 0 });
      cursor = addDays(cursor, 1);
    }
    grid.push(week);
  }
  return grid;
}
