import { formatMonthDay, parseIso } from "./date";

export const WEEKDAYS = [
  "Sunday",
  "Monday",
  "Tuesday",
  "Wednesday",
  "Thursday",
  "Friday",
  "Saturday",
];

/**
 * The same seven, abbreviated, for places too narrow to spell them out — the
 * heatmap's row labels are three characters wide at most.
 */
export const SHORT_WEEKDAYS = WEEKDAYS.map((name) => name.slice(0, 3));

/** Full weekday name for 0 = Sunday … 6 = Saturday; "Unscheduled" for null. */
export function weekdayLabel(n: number | null | undefined): string {
  return n == null ? "Unscheduled" : (WEEKDAYS[n] ?? "");
}

/** Today's weekday index (0 = Sunday … 6 = Saturday). */
export function todayWeekday(): number {
  return new Date().getDay();
}

export type WeekdayOption = { value: number; label: string };

/**
 * The 7 weekdays labeled with their next upcoming calendar date — today counts
 * as 0 days away — e.g. `{ value: 5, label: "Friday, August 7" }`. `today`
 * defaults to now and is injectable for testing.
 */
export function weekdayOptions(today: Date = new Date()): WeekdayOption[] {
  const base = today.getDay();
  return WEEKDAYS.map((name, i) => {
    const daysAhead = (i - base + 7) % 7;
    const date = new Date(
      today.getFullYear(),
      today.getMonth(),
      today.getDate() + daysAhead,
    );
    return { value: i, label: `${name}, ${formatMonthDay(date)}` };
  });
}

/**
 * How a due date reads on the one card whose weekday picker names a nearer
 * date: "Next Friday, September 25".
 *
 * That card is the day scheduled for today and already trained. The picker's
 * options are labeled with each weekday's next UPCOMING occurrence and today
 * counts as zero days away, so its selected option reads today's date — while
 * the day itself is not due again until the same weekday next week. Every other
 * card has the two agreeing, and says the date once through the picker; see
 * ProgramDetail for the comparison that decides which is which.
 *
 * The weekday alone would not do, because it names both dates. "Next" is what
 * separates them, and it is only correct because the gap is always exactly a
 * week — nextDueOn adds seven days, and only to a day due today. Don't reach
 * for this to label an arbitrary date.
 *
 * Returns the input unchanged if it can't be parsed, matching formatLongDate: a
 * malformed date should show as itself, not as "Invalid Date".
 */
export function nextWeekLabel(iso: string): string {
  const date = parseIso(iso);
  if (Number.isNaN(date.getTime())) return iso;
  return `Next ${WEEKDAYS[date.getDay()]}, ${formatMonthDay(date)}`;
}
