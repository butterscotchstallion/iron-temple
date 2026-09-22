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
 * The year, month and day of a YYYY-MM-DD string, as numbers.
 *
 * The one place this repo splits an ISO date-only string. Every other reading of
 * one — a Date, a weekday, a month index, a chart's x quantity — is built on
 * this, so the rule that makes it safe lives in exactly one place: the parts are
 * read off the string rather than handed to `new Date(iso)`, which parses a
 * date-only value as UTC midnight and lands on the previous day for anyone west
 * of Greenwich.
 *
 * Unparseable input yields NaNs rather than throwing. Callers each have their
 * own answer for a bad date — the input unchanged, an empty row, a dropped
 * point — so this reports the failure and lets them decide.
 */
export function isoParts(iso: string): [number, number, number] {
  const [year, month, day] = iso.split("-").map(Number);
  return [year, month, day];
}

/**
 * An ISO date-only string as a Date at local midnight.
 *
 * Local rather than UTC because every date in this app is a date somebody
 * trained on, read in the timezone they trained in. An unparseable string gives
 * an Invalid Date, whose getDay() and getTime() are NaN — which is what lets
 * callers validate with Number.isInteger or Number.isNaN rather than re-parsing.
 */
export function parseIso(iso: string): Date {
  const [year, month, day] = isoParts(iso);
  return new Date(year, month - 1, day);
}

/**
 * Days since the epoch for a YYYY-MM-DD string — the x quantity for a chart
 * plotted against real time rather than against session index.
 *
 * UTC here, deliberately, and it is not a contradiction with parseIso's local
 * midnight: this is a difference between two dates, not a moment. Going through
 * UTC makes every day exactly 86,400,000 ms wide, so a DST boundary inside the
 * plotted range cannot shorten one day and skew the spacing of every point
 * after it.
 */
export function isoDayNumber(iso: string): number {
  const [year, month, day] = isoParts(iso);
  return Math.floor(Date.UTC(year, month - 1, day) / 86_400_000);
}

/**
 * Format an ISO date-only string (YYYY-MM-DD) as e.g. "August 6 2026". Parses
 * the parts directly to avoid timezone shifts from Date on a date-only value.
 * Returns the input unchanged if it can't be parsed.
 */
export function formatLongDate(iso: string): string {
  const [year, month, day] = isoParts(iso);
  const name = MONTHS[month - 1];
  if (!name || !Number.isInteger(year) || !Number.isInteger(day)) {
    return iso;
  }
  return `${name} ${day} ${year}`;
}

/**
 * How long ago an RFC3339 timestamp was, e.g. "2h ago".
 *
 * The one relative formatter in the app, and deliberately narrow. Every other
 * date this app shows is a training date — when a session was performed — and
 * those are absolute on purpose: "March 17 2026" is a fact about a workout that
 * stays true, where "5 days ago" decays the moment you look away. A
 * notification is the opposite. It is about recency, it is read once, and
 * "somebody applauded your squat day on March 17 2026" buries the only part
 * that matters.
 *
 * Takes a full timestamp, NOT a date-only string. That distinction is the rule
 * this file exists to enforce — see isoParts — and it is why `new Date()` is
 * safe here and nowhere above it: a date-only string parses as UTC midnight and
 * slips a day in western timezones, where an RFC3339 instant carries its
 * offset.
 *
 * Rounds down at every step, so something 119 minutes old reads "1h ago".
 * Under a minute is "just now": a count of seconds changes while it is on
 * screen and reads as precision nobody asked for. Past a year it stops counting
 * and gives the date, because by then the relative form has stopped being
 * information.
 */
export function relativeTime(iso: string, now: Date = new Date()): string {
  const then = new Date(iso);
  if (Number.isNaN(then.getTime())) return iso;

  const seconds = Math.floor((now.getTime() - then.getTime()) / 1000);
  // Also catches a clock that disagrees with the server's, which is ordinary —
  // they are different machines. Reads as "just now" rather than as a negative.
  if (seconds < 60) return "just now";

  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;

  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;

  const days = Math.floor(hours / 24);
  if (days < 7) return `${days}d ago`;

  const weeks = Math.floor(days / 7);
  if (weeks < 52) return `${weeks}w ago`;

  return `${MONTHS[then.getMonth()]} ${then.getDate()} ${then.getFullYear()}`;
}

/** Month name + day for a Date, e.g. "August 7". */
export function formatMonthDay(date: Date): string {
  return `${MONTHS[date.getMonth()]} ${date.getDate()}`;
}
