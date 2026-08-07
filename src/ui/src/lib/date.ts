const MONTHS = [
  "January",
  "February",
  "March",
  "April",
  "May",
  "June",
  "July",
  "August",
  "September",
  "October",
  "November",
  "December",
];

/**
 * Format an ISO date-only string (YYYY-MM-DD) as e.g. "August 6 2026". Parses
 * the parts directly to avoid timezone shifts from Date on a date-only value.
 * Returns the input unchanged if it can't be parsed.
 */
export function formatLongDate(iso: string): string {
  const [year, month, day] = iso.split("-").map(Number);
  const name = MONTHS[month - 1];
  if (!name || !Number.isInteger(year) || !Number.isInteger(day)) {
    return iso;
  }
  return `${name} ${day} ${year}`;
}

/** Month name + day for a Date, e.g. "August 7". */
export function formatMonthDay(date: Date): string {
  return `${MONTHS[date.getMonth()]} ${date.getDate()}`;
}
