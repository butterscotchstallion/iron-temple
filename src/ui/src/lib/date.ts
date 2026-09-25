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
 * Abbreviated month names, for the tooltip form only.
 *
 * The long names above are for prose — a caption, a recap header, a sentence
 * somebody reads. The tooltip is a single dense line carrying a date AND a
 * time-of-day, and "September 24 2026 09:16PM" is wide enough to feel like a
 * paragraph in a hover box. Nothing else should reach for these.
 */
const SHORT_MONTHS = [
  "Jan",
  "Feb",
  "Mar",
  "Apr",
  "May",
  "Jun",
  "Jul",
  "Aug",
  "Sep",
  "Oct",
  "Nov",
  "Dec",
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
 * The same "August 6 2026" form, but for an RFC3339 *instant* rather than a
 * date-only string — the day that instant fell on, read in the local timezone.
 *
 * Deliberately not `formatLongDate(iso.slice(0, 10))`, which is what this
 * codebase reached for before. Those leading ten characters are the UTC day, so
 * for anything logged after 5pm in New York they name tomorrow. Going through a
 * Date and reading local components is the whole difference.
 *
 * For an instant somebody can hover, prefer relativeTime + formatLongDateTime.
 * This is for the places that have to be absolute and cannot be hovered — a
 * share card baked into a PNG, and this file's own past-18-months fallback.
 */
export function formatLongDateAt(iso: string): string {
  const at = new Date(iso);
  if (Number.isNaN(at.getTime())) return iso;
  return longDateOf(at);
}

/**
 * An RFC3339 instant as "Sep 24 2026 09:16PM", in the local timezone.
 *
 * The absolute fact behind a relative timestamp — what a `<time>` element's
 * tooltip carries. "2 days ago" is the friendlier read and the one worth
 * showing, but it is lossy in a way that occasionally matters ("was that before
 * or after I changed the program?"), and this is the answer to that question.
 *
 * Twelve-hour with a zero-padded hour, because the tooltips line up under each
 * other in a list and a ragged left edge on the time reads as a rendering bug.
 */
export function formatLongDateTime(iso: string): string {
  const at = new Date(iso);
  if (Number.isNaN(at.getTime())) return iso;

  const hour24 = at.getHours();
  const hour12 = hour24 % 12 === 0 ? 12 : hour24 % 12;
  const hour = String(hour12).padStart(2, "0");
  const minute = String(at.getMinutes()).padStart(2, "0");
  const half = hour24 < 12 ? "AM" : "PM";

  const month = SHORT_MONTHS[at.getMonth()];
  return `${month} ${at.getDate()} ${at.getFullYear()} ${hour}:${minute}${half}`;
}

/**
 * Days since the epoch for a Date, counted in its LOCAL calendar.
 *
 * The local-time twin of isoDayNumber, and it goes through UTC for the same
 * reason that one does: Date.UTC makes every day exactly 86,400,000 ms wide, so
 * the difference between two of these is a count of calendar days that a DST
 * boundary in between cannot bend. Reading y/m/d off the Date first is what
 * keeps it local — the UTC here is arithmetic, not a timezone.
 */
function localDayNumber(date: Date): number {
  const utc = Date.UTC(date.getFullYear(), date.getMonth(), date.getDate());
  return Math.floor(utc / 86_400_000);
}

/** The long "August 6 2026" form of a Date, read in local components. */
function longDateOf(date: Date): string {
  return `${MONTHS[date.getMonth()]} ${date.getDate()} ${date.getFullYear()}`;
}

/** "1 minute ago" / "45 minutes ago" — the unit pluralised against its count. */
function plural(count: number, unit: string): string {
  return `${count} ${unit}${count === 1 ? "" : "s"} ago`;
}

/**
 * The tail shared by both ladders below, from a day up, given a count of
 * calendar days already computed.
 *
 * Shared so an instant and a date-only value the same distance back read
 * identically. "3 days ago" should not depend on whether the server happened to
 * send a time of day alongside the date.
 *
 * Weeks hand over to months at eight rather than at a year: "51 weeks ago" is
 * arithmetic rather than language, and nobody reasons in that unit past a couple
 * of months. Months are counted as *completed calendar* months rather than
 * days/30, so a date in January reads "8 months ago" in September whatever the
 * lengths of the months in between.
 *
 * Past eighteen months it stops counting and gives the date, because by then the
 * relative form has stopped being information — and a tooltip is the wrong place
 * to keep the only legible answer.
 */
function calendarAgo(days: number, then: Date, now: Date): string {
  if (days === 1) return "yesterday";
  if (days < 7) return `${days} days ago`;

  const weeks = Math.floor(days / 7);
  if (weeks <= 8) return plural(weeks, "week");

  // Completed months: the raw month difference, less one if this month has not
  // yet reached the day-of-month it started on.
  const spanned =
    (now.getFullYear() - then.getFullYear()) * 12 +
    (now.getMonth() - then.getMonth());
  const months = now.getDate() < then.getDate() ? spanned - 1 : spanned;
  if (months <= 18) return plural(months, "month");

  return longDateOf(then);
}

/**
 * How long ago an RFC3339 timestamp was, e.g. "2 days ago".
 *
 * Takes a full timestamp, NOT a date-only string — relativeDate below is for
 * those. That distinction is the rule this file exists to enforce (see isoParts)
 * and it is why `new Date()` is safe here and not above it: a date-only string
 * parses as UTC midnight and slips a day in western timezones, where an RFC3339
 * instant carries its offset.
 *
 * Rounds DOWN at every step, so something 119 minutes old reads "1 hour ago".
 * Saying "2 hours ago" would claim more time has passed than has.
 *
 * Under a minute is "just now": a count of seconds changes while it is on screen
 * and reads as precision nobody asked for.
 *
 * Past a day it stops measuring elapsed time and switches to calendar days,
 * because that is what the words mean — "yesterday" is the previous date, not a
 * point 24 to 48 hours back. Floored at one day so this branch cannot say
 * "today": on the 25-hour day that ends DST a timestamp 24 hours old really is
 * still today's, and "today" is relativeDate's word, not this one's.
 */
export function relativeTime(iso: string, now: Date = new Date()): string {
  const then = new Date(iso);
  if (Number.isNaN(then.getTime())) return iso;

  const seconds = Math.floor((now.getTime() - then.getTime()) / 1000);
  // Also catches a clock that disagrees with the server's, which is ordinary —
  // they are different machines. Reads as "just now" rather than as a negative.
  if (seconds < 60) return "just now";

  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return plural(minutes, "minute");

  const hours = Math.floor(minutes / 60);
  if (hours < 24) return plural(hours, "hour");

  const days = Math.max(1, localDayNumber(now) - localDayNumber(then));
  return calendarAgo(days, then, now);
}

/**
 * How long ago a date-only YYYY-MM-DD string was, e.g. "3 days ago".
 *
 * The counterpart to relativeTime for dates that carry no time of day — a
 * training date, a weigh-in. Two things make it its own function rather than a
 * flag on that one.
 *
 * It parses through parseIso, so the date is anchored to LOCAL midnight. Handing
 * "2026-09-24" to `new Date()` instead reads it as UTC midnight and reports a
 * day too many for everyone west of Greenwich — the bug this file is shaped
 * around.
 *
 * And it has a word relativeTime does not: "today". With no time of day there is
 * no ladder of minutes and hours beneath it, so the smallest true thing it can
 * say is which day it was.
 *
 * A date in the future also reads "today", for the same reason a clock skewed
 * ahead does in relativeTime: the date comes from a server whose idea of today
 * can differ from this device's by a few hours, which is ordinary and not worth
 * a negative count.
 */
export function relativeDate(iso: string, now: Date = new Date()): string {
  const then = parseIso(iso);
  if (Number.isNaN(then.getTime())) return iso;

  const days = localDayNumber(now) - localDayNumber(then);
  if (days <= 0) return "today";
  return calendarAgo(days, then, now);
}

/** Month name + day for a Date, e.g. "August 7". */
export function formatMonthDay(date: Date): string {
  return `${MONTHS[date.getMonth()]} ${date.getDate()}`;
}
