import { describe, it, expect } from "vitest";
import { buildCalendar } from "./calendar";

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
