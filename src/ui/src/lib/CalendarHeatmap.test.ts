import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen } from "@testing-library/svelte";
import CalendarHeatmap from "./CalendarHeatmap.svelte";

// buildCalendar/todayIso are exercised by calendar.test.ts; here we mock them so
// the heatmap's own rendering (grid → cells, intensity, tooltips, month labels)
// is tested against a fixed, deterministic grid. The rest of the module is the
// real thing — activeWeekdays decides which rows exist, and a stub of it would
// leave the row collapsing untested on the only surface that draws it.
const { buildCalendarMock } = vi.hoisted(() => ({ buildCalendarMock: vi.fn() }));
vi.mock("./calendar", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./calendar")>()),
  buildCalendar: buildCalendarMock,
  todayIso: () => "2026-08-14",
}));

type Day = { date: string; count: number };

// 2026-08-02 is a Sunday, so weeks built from it (or from the 9th) line up with
// buildCalendar's Sunday → Saturday rows. That alignment matters: the heatmap
// now drops rows by weekday, and a column whose cells sat on the wrong weekday
// would be filtered on dates the test never meant.
function week(startDay: number, counts: number[]): Day[] {
  return counts.map((count, i) => ({
    date: `2026-08-${String(startDay + i).padStart(2, "0")}`,
    count,
  }));
}

const EMPTY_WEEK = [0, 0, 0, 0, 0, 0, 0];

beforeEach(() => {
  buildCalendarMock.mockReset();
});

describe("CalendarHeatmap", () => {
  it("renders one cell per day in the grid", () => {
    buildCalendarMock.mockReturnValue([week(2, EMPTY_WEEK), week(9, EMPTY_WEEK)]);
    const { container } = render(CalendarHeatmap, { sessions: [] });
    expect(container.querySelectorAll("[title]")).toHaveLength(14);
  });

  it("scales cell intensity by workout count", () => {
    buildCalendarMock.mockReturnValue([week(2, [0, 1, 2, 3, 0, 0, 0])]);
    render(CalendarHeatmap, { sessions: [] });

    expect(screen.getByTitle("2026-08-02")).toHaveClass("bg-muted/40"); // none
    expect(screen.getByTitle("2026-08-03")).toHaveClass("bg-primary/50"); // 1
    expect(screen.getByTitle("2026-08-04")).toHaveClass("bg-primary/75"); // 2
    expect(screen.getByTitle("2026-08-05")).toHaveClass("bg-primary"); // 3+
    // Exact-token check: the 3+ cell is not the paler /50 variant.
    expect(screen.getByTitle("2026-08-05")).not.toHaveClass("bg-primary/50");
  });

  it("names the workout(s) performed on a day in its tooltip", () => {
    buildCalendarMock.mockReturnValue([
      week(2, [0, 1, 0, 0, 0, 0, 0]),
      week(9, EMPTY_WEEK),
    ]);
    render(CalendarHeatmap, {
      sessions: [{ performedOn: "2026-08-03", day: "Workout A" }],
    });

    expect(screen.getByTitle("2026-08-03 · Workout A")).toBeInTheDocument();
    // A day with no session just shows the date.
    expect(screen.getByTitle("2026-08-10")).toBeInTheDocument();
  });

  it("merges multiple workouts on the same day into one tooltip", () => {
    buildCalendarMock.mockReturnValue([week(2, [0, 2, 0, 0, 0, 0, 0])]);
    render(CalendarHeatmap, {
      sessions: [
        { performedOn: "2026-08-03", day: "Workout A" },
        { performedOn: "2026-08-03", day: "Workout B" },
      ],
    });
    expect(screen.getByTitle("2026-08-03 · Workout A, Workout B")).toBeInTheDocument();
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

  // The rows a two-day lifter can never fill are the bulk of a seven-row grid,
  // so they come out and the remaining blanks mean something.
  describe("collapsed to the weekdays that matter", () => {
    // Mondays and Thursdays.
    const twoDays = [
      { performedOn: "2026-08-03", day: "A" },
      { performedOn: "2026-08-06", day: "B" },
      { performedOn: "2026-08-10", day: "A" },
    ];

    it("draws a row per trained weekday and nothing else", () => {
      buildCalendarMock.mockReturnValue([week(2, EMPTY_WEEK), week(9, EMPTY_WEEK)]);
      const { container } = render(CalendarHeatmap, { sessions: twoDays });
      // Two rows over two columns, not fourteen cells.
      expect(container.querySelectorAll("[title]")).toHaveLength(4);
      expect(screen.getByTitle("2026-08-03 · A")).toBeInTheDocument();
      expect(screen.getByTitle("2026-08-06 · B")).toBeInTheDocument();
      // Tuesday and the rest of the week are gone, not merely blank.
      expect(screen.queryByTitle("2026-08-04")).not.toBeInTheDocument();
    });

    it("names the rows it kept", () => {
      buildCalendarMock.mockReturnValue([week(2, EMPTY_WEEK)]);
      render(CalendarHeatmap, { sessions: twoDays });
      expect(screen.getByText("Mon")).toBeInTheDocument();
      expect(screen.getByText("Thu")).toBeInTheDocument();
      expect(screen.queryByText("Tue")).not.toBeInTheDocument();
    });

    // The whole point of the schedule: a day you were meant to train and
    // didn't leaves an empty cell on a row that is still drawn.
    it("keeps a scheduled weekday that carries no session", () => {
      buildCalendarMock.mockReturnValue([week(2, [0, 1, 0, 0, 0, 0, 0])]);
      render(CalendarHeatmap, {
        sessions: [{ performedOn: "2026-08-03", day: "A" }],
        scheduledWeekdays: [1, 4],
      });
      expect(screen.getByText("Thu")).toBeInTheDocument();
      expect(screen.getByTitle("2026-08-06")).toHaveClass("bg-muted/40");
    });

    it("leaves a full seven-row grid unlabelled", () => {
      buildCalendarMock.mockReturnValue([week(2, EMPTY_WEEK)]);
      render(CalendarHeatmap, { sessions: [] });
      expect(screen.queryByText("Mon")).not.toBeInTheDocument();
    });

    // A Saturday that hasn't happened yet is not a session missed, so it is
    // drawn fainter than a blank inside the window.
    it("shades a day past the end of the window apart from a miss", () => {
      buildCalendarMock.mockReturnValue([week(9, EMPTY_WEEK)]);
      render(CalendarHeatmap, { sessions: [] });
      expect(screen.getByTitle("2026-08-14")).toHaveClass("bg-muted/40");
      expect(screen.getByTitle("2026-08-15")).toHaveClass("bg-muted/20");
    });
  });
});
