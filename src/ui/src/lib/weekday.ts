import { formatMonthDay } from "./date";

export const WEEKDAYS = [
  "Sunday",
  "Monday",
  "Tuesday",
  "Wednesday",
  "Thursday",
  "Friday",
  "Saturday",
];

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
