import { render, screen, waitFor } from "@testing-library/svelte";
import { beforeEach, describe, expect, it, vi } from "vitest";
import Racked from "./Racked.svelte";

// A render test for a route, which the suite otherwise leaves to Playwright.
//
// It earns its place because the recap page branches hard on nullable fields —
// a period with no finished session has no fastest session, a lift performed
// once has no trend — and every one of those branches is a chance to read a
// property off null. jsdom catches that in a second; the browser suite needs a
// build and a server, and cannot run at all in the devcontainer, where
// Playwright's browser download is unreachable.

const getRacked = vi.hoisted(() => vi.fn());
vi.mock("../lib/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../lib/api")>()),
  getRacked,
}));

/** A recap with every optional section absent — the shape of a quiet month. */
function emptyReport() {
  return {
    period: { kind: "month", start: "2026-03-01", end: "2026-03-31", label: "March 2026" },
    totals: { volumeLb: 0, sessions: 0, sets: 0, reps: 0 },
    change: null,
    comparison: { count: 0, label: "", unitLb: 0 },
    lifts: [],
    series: [],
    mostImproved: null,
    days: [],
    weekdays: [0, 0, 0, 0, 0, 0, 0],
    bestWeekday: -1,
    hours: Array.from({ length: 24 }, () => 0),
    hourLabel: "",
    streak: { longestWeeks: 0, currentWeeks: 0 },
    attendance: { basis: "none", expected: 0, actual: 0, rate: 0 },
    prs: [],
    milestones: [],
    heaviestSet: null,
    fastestSession: null,
    deloads: [],
    archetype: { name: "", description: "" },
  };
}

/** A recap with every section populated. */
function fullReport() {
  return {
    ...emptyReport(),
    totals: { volumeLb: 84_000, sessions: 12, sets: 180, reps: 900 },
    change: { volumeLb: 9_000, volumePct: 0.12, sessions: 2, sessionsPct: 0.2 },
    comparison: { count: 3, label: "school buses", unitLb: 24_000 },
    lifts: [
      { exerciseId: 1, exerciseName: "Squat", volumeLb: 50_000, sets: 90, reps: 450, share: 0.6 },
      { exerciseId: 2, exerciseName: "Bench Press", volumeLb: 34_000, sets: 90, reps: 450, share: 0.4 },
    ],
    series: [
      {
        exerciseId: 1,
        exerciseName: "Squat",
        points: [
          { performedOn: "2026-03-02", topWeightLb: 200, e1rmLb: 233 },
          { performedOn: "2026-03-16", topWeightLb: 220, e1rmLb: 256 },
        ],
      },
    ],
    mostImproved: {
      exerciseId: 1,
      exerciseName: "Squat",
      fromLb: 233,
      toLb: 256,
      gainLb: 23,
      gainPct: 0.0987,
    },
    days: [
      { date: "2026-03-02", volumeLb: 7_000, sessions: 1 },
      { date: "2026-03-16", volumeLb: 7_500, sessions: 1 },
    ],
    weekdays: [0, 42_000, 0, 30_000, 0, 12_000, 0],
    bestWeekday: 1,
    hours: Array.from({ length: 24 }, (_, h) => (h === 6 ? 9 : 0)),
    hourLabel: "Early bird",
    streak: { longestWeeks: 5, currentWeeks: 3 },
    attendance: { basis: "cadence", expected: 13, actual: 12, rate: 0.923 },
    prs: [
      {
        kind: "weight",
        performedOn: "2026-03-16",
        exerciseId: 1,
        exerciseName: "Squat",
        weightLb: 220,
        reps: 5,
        valueLb: 220,
        previousLb: 215,
      },
    ],
    milestones: [
      {
        kind: "plate",
        performedOn: "2026-03-16",
        label: "First 225 lb Squat",
        valueLb: 225,
        exerciseId: 1,
        exerciseName: "Squat",
      },
    ],
    heaviestSet: {
      performedOn: "2026-03-16",
      exerciseId: 1,
      exerciseName: "Squat",
      weightLb: 220,
      reps: 5,
    },
    fastestSession: {
      sessionId: 7,
      performedOn: "2026-03-09",
      programDayName: "Workout A",
      durationSeconds: 2_880,
      volumeLb: 6_800,
      sets: 15,
    },
    deloads: [
      {
        exerciseId: 2,
        exerciseName: "Bench Press",
        performedOn: "2026-03-09",
        fromLb: 160,
        toLb: 145,
        recovered: false,
        recoveredOn: null,
      },
    ],
    archetype: { name: "The Grinder", description: "Long sessions, no rush." },
  };
}

beforeEach(() => {
  getRacked.mockReset();
});

describe("Racked", () => {
  it("renders the headline and its restatement in objects", async () => {
    getRacked.mockResolvedValue({ data: fullReport(), error: undefined });
    render(Racked);

    await waitFor(() => expect(screen.getByText("84,000")).toBeInTheDocument());
    expect(screen.getByText("That's 3 school buses.")).toBeInTheDocument();
    expect(screen.getByText("+12% vs the previous month")).toBeInTheDocument();
    expect(screen.getByText("The Grinder")).toBeInTheDocument();
  });

  it("renders every populated section without touching a null", async () => {
    getRacked.mockResolvedValue({ data: fullReport(), error: undefined });
    render(Racked);

    await waitFor(() =>
      expect(screen.getByRole("heading", { name: "Most improved" })).toBeInTheDocument(),
    );
    expect(screen.getByRole("heading", { name: "Heaviest set" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Fastest session" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Where the weight went" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "1 personal record" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Milestones" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Stalls and comebacks" })).toBeInTheDocument();
    expect(screen.getByText("48m")).toBeInTheDocument();
  });

  // The branch that would throw if any of the nullable sections were read
  // unconditionally. A quiet month must render as a sentence, not a stack trace
  // and not a page of confident zeroes.
  it("says nothing was logged rather than rendering empty statistics", async () => {
    getRacked.mockResolvedValue({ data: emptyReport(), error: undefined });
    render(Racked);

    await waitFor(() =>
      expect(screen.getByText(/Nothing logged in March 2026/)).toBeInTheDocument(),
    );
    expect(screen.queryByRole("heading", { name: "Most improved" })).not.toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Fastest session" })).not.toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Attendance" })).not.toBeInTheDocument();
  });

  it("labels an estimated attendance rate as an estimate", async () => {
    getRacked.mockResolvedValue({ data: fullReport(), error: undefined });
    render(Racked);

    await waitFor(() => expect(screen.getByText("92%")).toBeInTheDocument());
    expect(screen.getByText(/estimated from your program's shape/)).toBeInTheDocument();
  });

  it("offers a retry when the report fails to load", async () => {
    getRacked.mockResolvedValue({ data: undefined, error: { code: "internal" } });
    render(Racked);

    await waitFor(() =>
      expect(screen.getByText("Couldn't load your stats.")).toBeInTheDocument(),
    );
  });

  it("asks for the year when the year is selected", async () => {
    getRacked.mockResolvedValue({ data: fullReport(), error: undefined });
    const { getByRole } = render(Racked);

    await waitFor(() => expect(getRacked).toHaveBeenCalled());
    expect(getRacked).toHaveBeenLastCalledWith({ query: { period: "month" } });

    getByRole("radio", { name: "This year" }).click();
    await waitFor(() =>
      expect(getRacked).toHaveBeenLastCalledWith({ query: { period: "year" } }),
    );
  });
});
