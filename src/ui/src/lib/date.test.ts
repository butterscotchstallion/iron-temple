import { describe, it, expect } from "vitest";
import {
  formatLongDate,
  formatLongDateAt,
  formatLongDateTime,
  relativeDate,
  relativeTime,
} from "./date";

// Every `now` and every instant in this file is built from LOCAL components
// rather than parsed from a "…Z" literal. The rungs from a day up count calendar
// days, and the tooltip forms read local time, so a UTC literal would make these
// expectations depend on the timezone of whatever machine ran them.
const local = (
  year: number,
  month: number,
  day: number,
  hour = 12,
  minute = 0,
) => new Date(year, month, day, hour, minute, 0);

describe("formatLongDate", () => {
  it("formats an ISO date as 'Month D YYYY'", () => {
    expect(formatLongDate("2026-08-06")).toBe("August 6 2026");
    expect(formatLongDate("2026-01-01")).toBe("January 1 2026");
    expect(formatLongDate("2026-12-31")).toBe("December 31 2026");
  });

  it("drops the leading zero on the day", () => {
    expect(formatLongDate("2026-03-09")).toBe("March 9 2026");
  });

  it("returns the input unchanged when it can't parse", () => {
    expect(formatLongDate("not-a-date")).toBe("not-a-date");
  });

  it("cannot read an instant, which is why formatLongDateAt exists", () => {
    // Pinning the trap rather than endorsing it. The day parses as NaN, the
    // guard trips, and the raw string comes back — which four call sites used to
    // render to users verbatim. Anything holding an instant wants
    // formatLongDateAt, or a relative form.
    expect(formatLongDate("2026-09-24T21:16:00Z")).toBe("2026-09-24T21:16:00Z");
  });
});

describe("formatLongDateAt", () => {
  it("formats the local day an instant fell on", () => {
    expect(formatLongDateAt(local(2026, 7, 6, 14, 30).toISOString())).toBe(
      "August 6 2026",
    );
  });

  it("reads the local day, not the UTC one", () => {
    // Eleven at night. Slicing the first ten characters off the ISO string —
    // which is what this replaced — takes the UTC day, and so names tomorrow
    // anywhere west of Greenwich.
    expect(formatLongDateAt(local(2026, 7, 6, 23, 30).toISOString())).toBe(
      "August 6 2026",
    );
  });

  it("returns the input unchanged when it can't parse", () => {
    expect(formatLongDateAt("not-a-timestamp")).toBe("not-a-timestamp");
  });
});

describe("formatLongDateTime", () => {
  it("formats an instant as the tooltip form", () => {
    expect(formatLongDateTime(local(2026, 8, 24, 21, 16).toISOString())).toBe(
      "Sep 24 2026 09:16PM",
    );
  });

  it("pads the hour, so a column of tooltips lines up", () => {
    expect(formatLongDateTime(local(2026, 0, 1, 9, 7).toISOString())).toBe(
      "Jan 1 2026 09:07AM",
    );
  });

  it("calls midnight 12AM and noon 12PM", () => {
    expect(formatLongDateTime(local(2026, 0, 1, 0, 5).toISOString())).toBe(
      "Jan 1 2026 12:05AM",
    );
    expect(formatLongDateTime(local(2026, 0, 1, 12, 0).toISOString())).toBe(
      "Jan 1 2026 12:00PM",
    );
  });

  it("returns the input unchanged when it can't parse", () => {
    expect(formatLongDateTime("not-a-timestamp")).toBe("not-a-timestamp");
  });
});

describe("relativeTime", () => {
  // Fixed, so these say what they mean rather than depending on when they run.
  const now = local(2026, 2, 17);
  const ago = (ms: number) => new Date(now.getTime() - ms).toISOString();

  const SECOND = 1000;
  const MINUTE = 60 * SECOND;
  const HOUR = 60 * MINUTE;
  const DAY = 24 * HOUR;
  const WEEK = 7 * DAY;

  it("calls anything under a minute 'just now'", () => {
    expect(relativeTime(ago(0), now)).toBe("just now");
    expect(relativeTime(ago(59 * SECOND), now)).toBe("just now");
  });

  it("counts minutes and hours in words", () => {
    expect(relativeTime(ago(MINUTE), now)).toBe("1 minute ago");
    expect(relativeTime(ago(45 * MINUTE), now)).toBe("45 minutes ago");
    expect(relativeTime(ago(HOUR), now)).toBe("1 hour ago");
    expect(relativeTime(ago(5 * HOUR), now)).toBe("5 hours ago");
  });

  it("rounds down rather than to nearest", () => {
    // 119 minutes is an hour and fifty-nine, and saying "2 hours ago" would be
    // claiming more time has passed than has.
    expect(relativeTime(ago(119 * MINUTE), now)).toBe("1 hour ago");
  });

  it("counts calendar days past a day, not 24-hour blocks", () => {
    // The words have to mean what they say: "yesterday" is the previous date.
    expect(relativeTime(ago(DAY), now)).toBe("yesterday");
    // 25 hours before noon is still the previous day.
    expect(relativeTime(ago(25 * HOUR), now)).toBe("yesterday");
    // And 47 hours before noon is the day before that — where measuring elapsed
    // time would floor to a single day and call it yesterday.
    expect(relativeTime(ago(47 * HOUR), now)).toBe("2 days ago");
    expect(relativeTime(ago(3 * DAY), now)).toBe("3 days ago");
  });

  it("climbs to weeks, then to months", () => {
    expect(relativeTime(ago(WEEK), now)).toBe("1 week ago");
    // Eight weeks is the last rung weeks get: "51 weeks ago" is arithmetic
    // rather than something anybody says.
    expect(relativeTime(ago(8 * WEEK), now)).toBe("8 weeks ago");
    expect(relativeTime(ago(9 * WEEK), now)).toBe("2 months ago");
  });

  it("gives the date once the relative form stops being information", () => {
    // Eighteen months still counts; nineteen is where it stops.
    expect(relativeTime(local(2024, 8, 17).toISOString(), now)).toBe(
      "18 months ago",
    );
    expect(relativeTime(local(2024, 7, 17).toISOString(), now)).toBe(
      "August 17 2024",
    );
  });

  it("reads a clock skewed ahead of the server as 'just now'", () => {
    // Two different machines; a future timestamp is ordinary rather than a bug,
    // and must not render as a negative count.
    const future = new Date(now.getTime() + 30 * SECOND).toISOString();
    expect(relativeTime(future, now)).toBe("just now");
  });

  it("returns the input unchanged when it can't parse", () => {
    expect(relativeTime("not-a-timestamp", now)).toBe("not-a-timestamp");
  });
});

describe("relativeDate", () => {
  const now = local(2026, 2, 17);

  it("names the day, having no time of day to count from", () => {
    expect(relativeDate("2026-03-17", now)).toBe("today");
    expect(relativeDate("2026-03-16", now)).toBe("yesterday");
    expect(relativeDate("2026-03-14", now)).toBe("3 days ago");
  });

  it("climbs the same ladder as relativeTime", () => {
    expect(relativeDate("2026-03-10", now)).toBe("1 week ago");
    expect(relativeDate("2026-01-20", now)).toBe("8 weeks ago");
    expect(relativeDate("2026-01-13", now)).toBe("2 months ago");
    expect(relativeDate("2024-08-17", now)).toBe("August 17 2024");
  });

  it("anchors the date to local midnight rather than UTC", () => {
    // Eleven at night. Handing "2026-03-17" to `new Date()` reads it as UTC
    // midnight, which is a different day's worth of offset for everyone west of
    // Greenwich — the bug date.ts is shaped around.
    expect(relativeDate("2026-03-17", local(2026, 2, 17, 23, 0))).toBe("today");
  });

  it("reads a date ahead of this device's day as 'today'", () => {
    // Same reasoning as the skewed clock above: the server's idea of today can
    // be a few hours off this device's, which is not worth a negative count.
    expect(relativeDate("2026-03-18", now)).toBe("today");
  });

  it("returns the input unchanged when it can't parse", () => {
    expect(relativeDate("not-a-date", now)).toBe("not-a-date");
  });
});
