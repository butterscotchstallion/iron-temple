import { describe, it, expect } from "vitest";
import { formatLongDate, relativeTime } from "./date";

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
});

describe("relativeTime", () => {
  // Fixed, so these say what they mean rather than depending on when they run.
  const now = new Date("2026-03-17T12:00:00Z");
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

  it("counts minutes, hours, days and weeks", () => {
    expect(relativeTime(ago(MINUTE), now)).toBe("1m ago");
    expect(relativeTime(ago(45 * MINUTE), now)).toBe("45m ago");
    expect(relativeTime(ago(HOUR), now)).toBe("1h ago");
    expect(relativeTime(ago(5 * HOUR), now)).toBe("5h ago");
    expect(relativeTime(ago(DAY), now)).toBe("1d ago");
    expect(relativeTime(ago(3 * DAY), now)).toBe("3d ago");
    expect(relativeTime(ago(WEEK), now)).toBe("1w ago");
  });

  it("rounds down rather than to nearest", () => {
    // 119 minutes is an hour and fifty-nine, and saying "2h ago" would be
    // claiming more time has passed than has.
    expect(relativeTime(ago(119 * MINUTE), now)).toBe("1h ago");
    expect(relativeTime(ago(47 * HOUR), now)).toBe("1d ago");
  });

  it("gives the date once the relative form stops being information", () => {
    expect(relativeTime(ago(60 * WEEK), now)).toMatch(/^January \d+ 2025$/);
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
