import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen } from "@testing-library/svelte";
import CalendarHeatmap from "./CalendarHeatmap.svelte";

// buildCalendar/todayIso are exercised by calendar.test.ts; here we mock them so
// the heatmap's own rendering (grid → cells, intensity, tooltips, month labels)
// is tested against a fixed, deterministic grid.
const { buildCalendarMock } = vi.hoisted(() => ({ buildCalendarMock: vi.fn() }));
vi.mock("./calendar", () => ({
  buildCalendar: buildCalendarMock,
  todayIso: () => "2026-08-14",
}));

type Day = { date: string; count: number };

function week(startDay: number, counts: number[]): Day[] {
  return counts.map((count, i) => ({
    date: `2026-08-${String(startDay + i).padStart(2, "0")}`,
    count,
  }));
}

beforeEach(() => {
  buildCalendarMock.mockReset();
});

describe("CalendarHeatmap", () => {
  it("renders one cell per day in the grid", () => {
    buildCalendarMock.mockReturnValue([
      week(1, [0, 0, 0, 0, 0, 0, 0]),
      week(8, [0, 0, 0, 0, 0, 0, 0]),
    ]);
    const { container } = render(CalendarHeatmap, { sessions: [] });
    expect(container.querySelectorAll("[title]")).toHaveLength(14);
  });

  it("scales cell intensity by workout count", () => {
    buildCalendarMock.mockReturnValue([week(1, [0, 1, 2, 3, 0, 0, 0])]);
    render(CalendarHeatmap, { sessions: [] });

    expect(screen.getByTitle("2026-08-01")).toHaveClass("bg-muted/40"); // none
    expect(screen.getByTitle("2026-08-02")).toHaveClass("bg-primary/50"); // 1
    expect(screen.getByTitle("2026-08-03")).toHaveClass("bg-primary/75"); // 2
    expect(screen.getByTitle("2026-08-04")).toHaveClass("bg-primary"); // 3+
    // Exact-token check: the 3+ cell is not the paler /50 variant.
    expect(screen.getByTitle("2026-08-04")).not.toHaveClass("bg-primary/50");
  });

  it("names the workout(s) performed on a day in its tooltip", () => {
    buildCalendarMock.mockReturnValue([week(1, [0, 1, 0, 0, 0, 0, 0])]);
    render(CalendarHeatmap, {
      sessions: [{ performedOn: "2026-08-02", day: "Workout A" }],
    });

    expect(screen.getByTitle("2026-08-02 · Workout A")).toBeInTheDocument();
    // A day with no session just shows the date.
    expect(screen.getByTitle("2026-08-01")).toBeInTheDocument();
  });

  it("merges multiple workouts on the same day into one tooltip", () => {
    buildCalendarMock.mockReturnValue([week(1, [0, 2, 0, 0, 0, 0, 0])]);
    render(CalendarHeatmap, {
      sessions: [
        { performedOn: "2026-08-02", day: "Workout A" },
        { performedOn: "2026-08-02", day: "Workout B" },
      ],
    });
    expect(screen.getByTitle("2026-08-02 · Workout A, Workout B")).toBeInTheDocument();
  });

  it("labels each month once, at its first column", () => {
    buildCalendarMock.mockReturnValue([
      [{ date: "2026-07-28", count: 0 }],
      [{ date: "2026-08-04", count: 0 }],
    ]);
    render(CalendarHeatmap, { sessions: [] });
    expect(screen.getByText("Jul")).toBeInTheDocument();
    expect(screen.getByText("Aug")).toBeInTheDocument();
  });
});
