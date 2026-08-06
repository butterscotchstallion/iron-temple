import { describe, it, expect } from "vitest";
import { formatLongDate } from "./date";

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
