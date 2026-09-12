import { describe, it, expect } from "vitest";
import { dueLabel, weekdayLabel, weekdayOptions } from "./weekday";

describe("weekdayLabel", () => {
  it("names weekdays 0..6", () => {
    expect(weekdayLabel(0)).toBe("Sunday");
    expect(weekdayLabel(2)).toBe("Tuesday");
    expect(weekdayLabel(6)).toBe("Saturday");
  });

  it("is Unscheduled for null/undefined", () => {
    expect(weekdayLabel(null)).toBe("Unscheduled");
    expect(weekdayLabel(undefined)).toBe("Unscheduled");
  });
});

describe("weekdayOptions", () => {
  // Friday, August 7 2026.
  const friday = new Date(2026, 7, 7);

  it("labels each weekday with its upcoming date, today counting as 0 days", () => {
    const opts = weekdayOptions(friday);
    const byValue = Object.fromEntries(opts.map((o) => [o.value, o.label]));
    expect(byValue[5]).toBe("Friday, August 7"); // today
    expect(byValue[6]).toBe("Saturday, August 8");
    expect(byValue[0]).toBe("Sunday, August 9");
    expect(byValue[4]).toBe("Thursday, August 13"); // furthest out
  });

  it("returns all seven weekdays in Sunday..Saturday order", () => {
    const opts = weekdayOptions(friday);
    expect(opts.map((o) => o.value)).toEqual([0, 1, 2, 3, 4, 5, 6]);
  });

  it("rolls upcoming dates across a month boundary", () => {
    // Thursday, August 27 2026: Sunday is Aug 30, Monday spills to Sep 1.
    const opts = weekdayOptions(new Date(2026, 7, 27));
    const byValue = Object.fromEntries(opts.map((o) => [o.value, o.label]));
    expect(byValue[0]).toBe("Sunday, August 30");
    expect(byValue[1]).toBe("Monday, August 31");
    expect(byValue[2]).toBe("Tuesday, September 1");
  });
});

describe("dueLabel", () => {
  const TODAY = "2026-09-11"; // a Friday

  it("says Today for today", () => {
    expect(dueLabel(TODAY, TODAY)).toBe("Today");
  });

  it("names the weekday and date for any other day", () => {
    expect(dueLabel("2026-09-18", TODAY)).toBe("Friday, September 18");
    expect(dueLabel("2026-09-15", TODAY)).toBe("Tuesday, September 15");
  });

  // The case the label exists for: two cards for the same program day a week
  // apart have to be tellable apart, and only the date does that.
  it("distinguishes the same weekday a week apart", () => {
    expect(dueLabel("2026-09-18", TODAY)).not.toBe(dueLabel("2026-09-25", TODAY));
  });

  it("crosses a month boundary", () => {
    expect(dueLabel("2026-10-02", TODAY)).toBe("Friday, October 2");
  });

  it("returns a malformed date unchanged rather than Invalid Date", () => {
    expect(dueLabel("not-a-date", TODAY)).toBe("not-a-date");
  });
});
