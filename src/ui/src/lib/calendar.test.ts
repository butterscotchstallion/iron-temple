import { describe, it, expect } from "vitest";
import { addDaysIso, buildCalendar, volumeLevel } from "./calendar";

describe("buildCalendar", () => {
  // 2026-08-06 is a Thursday.
  it("returns a weeks × 7 grid ending in the week of endIso", () => {
    const grid = buildCalendar([], "2026-08-06", 4);
    expect(grid).toHaveLength(4);
    expect(grid.every((w) => w.length === 7)).toBe(true);
    // Last column ends on the Saturday of that week (2026-08-08).
    expect(grid[3][6].date).toBe("2026-08-08");
    // First column starts on the Sunday 4 weeks back.
    expect(grid[0][0].date).toBe("2026-07-12");
  });

  it("counts sessions on their day", () => {
    const grid = buildCalendar(
      ["2026-08-06", "2026-08-06", "2026-08-04"],
      "2026-08-06",
      4,
    );
    const flat = grid.flat();
    expect(flat.find((d) => d.date === "2026-08-06")?.count).toBe(2);
    expect(flat.find((d) => d.date === "2026-08-04")?.count).toBe(1);
    expect(flat.find((d) => d.date === "2026-08-05")?.count).toBe(0);
  });
});

describe("volumeLevel", () => {
  it("steps by share of the heaviest day", () => {
    expect(volumeLevel(10_000, 10_000)).toBe(3);
    expect(volumeLevel(5_000, 10_000)).toBe(2);
    expect(volumeLevel(1_000, 10_000)).toBe(1);
  });

  // Relative rather than absolute, so the grid reads the same for a beginner
  // and for someone moving ten times the tonnage.
  it("scales with the lifter rather than fixed thresholds", () => {
    expect(volumeLevel(3_000, 3_000)).toBe(volumeLevel(30_000, 30_000));
    expect(volumeLevel(1_500, 3_000)).toBe(volumeLevel(15_000, 30_000));
  });

  // A day with work done must never render as an empty day.
  it("never hides a light session", () => {
    expect(volumeLevel(1, 100_000)).toBe(1);
  });

  it("reads nothing as nothing", () => {
    expect(volumeLevel(0, 10_000)).toBe(0);
    expect(volumeLevel(500, 0)).toBe(0);
    expect(volumeLevel(Number.NaN, 100)).toBe(0);
  });

  it("clamps a day heavier than the stated maximum", () => {
    expect(volumeLevel(200, 100)).toBe(3);
  });
});

describe("addDaysIso", () => {
  it("adds days within a month", () => {
    expect(addDaysIso(new Date(2026, 8, 11), 7)).toBe("2026-09-18");
  });

  it("returns the same date for 0", () => {
    expect(addDaysIso(new Date(2026, 8, 11), 0)).toBe("2026-09-11");
  });

  it("crosses a month boundary", () => {
    expect(addDaysIso(new Date(2026, 8, 28), 7)).toBe("2026-10-05");
  });

  it("crosses a year boundary", () => {
    expect(addDaysIso(new Date(2026, 11, 30), 7)).toBe("2027-01-06");
  });

  it("handles a leap day", () => {
    expect(addDaysIso(new Date(2028, 1, 26), 7)).toBe("2028-03-04");
  });
});
