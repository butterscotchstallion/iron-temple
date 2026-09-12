import { todayIso } from "./calendar";
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

/**
 * How a program day's next occurrence reads on its card: "Today" when it is due
 * today, otherwise the weekday and date it lands on — "Friday, September 18".
 *
 * The weekday is spelled out rather than left to the date alone because the
 * whole point of the label is to distinguish two cards for the SAME program day
 * a week apart, and on a one-day-a-week program that is all there is to go on.
 *
 * Returns the input unchanged if it can't be parsed, matching formatLongDate:
 * a malformed date should show as itself, not as "Invalid Date".
 */
export function dueLabel(iso: string, today: string = todayIso()): string {
  if (iso === today) return "Today";
  const [year, month, day] = iso.split("-").map(Number);
  if (!Number.isInteger(year) || !Number.isInteger(month) || !Number.isInteger(day)) {
    return iso;
  }
  const date = new Date(year, month - 1, day);
  return `${WEEKDAYS[date.getDay()]}, ${formatMonthDay(date)}`;
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
