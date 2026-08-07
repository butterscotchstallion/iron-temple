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
