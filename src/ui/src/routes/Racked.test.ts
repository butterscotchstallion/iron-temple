import { render, screen, waitFor } from "@testing-library/svelte";
import { beforeEach, describe, expect, it, vi } from "vitest";
import Racked from "./Racked.svelte";
import type { RackedReport } from "../lib/api";

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
function emptyReport(): RackedReport {
  return {
    period: {
      kind: "month",
      start: "2026-03-01",
      end: "2026-03-31",
      label: "March 2026",
      inProgress: false,
    },
    totals: { volumeLb: 0, sessions: 0, sets: 0, reps: 0 },
    change: null,
    comparison: { count: 0, label: "", unitLb: 0 },
    split: {
      main: { volumeLb: 0, sets: 0, reps: 0, lifts: 0, share: 0 },
      assistance: { volumeLb: 0, sets: 0, reps: 0, lifts: 0, share: 0 },
    },
    muscles: [
    { group: "chest", volumeLb: 0, sets: 0, reps: 0, lifts: 0, share: 0, trained: false },
    { group: "back", volumeLb: 0, sets: 0, reps: 0, lifts: 0, share: 0, trained: false },
    { group: "legs", volumeLb: 0, sets: 0, reps: 0, lifts: 0, share: 0, trained: false },
    { group: "shoulders", volumeLb: 0, sets: 0, reps: 0, lifts: 0, share: 0, trained: false },
    { group: "arms", volumeLb: 0, sets: 0, reps: 0, lifts: 0, share: 0, trained: false },
    { group: "core", volumeLb: 0, sets: 0, reps: 0, lifts: 0, share: 0, trained: false },
    { group: "other", volumeLb: 0, sets: 0, reps: 0, lifts: 0, share: 0, trained: false },
    ],
    lifts: [],
    series: [],
    mostImproved: null,
    bodyweight: null,
    days: [],
    weekdays: [0, 0, 0, 0, 0, 0, 0],
    bestWeekday: -1,
    hours: Array.from({ length: 24 }, () => 0) as RackedReport["hours"],
    peakHour: -1,
    hourLabel: "",
    streak: { longestWeeks: 0, currentWeeks: 0 },
    attendance: { basis: "none", expected: 0, actual: 0, rate: 0, sessionsPerWeek: 0 },
    prs: [],
    milestones: [],
    heaviestSet: null,
    fastestSession: null,
    deloads: [],
    archetype: { name: "", description: "" },
  };
}

/** A recap with every section populated. */
function fullReport(): RackedReport {
  return {
    ...emptyReport(),
    totals: { volumeLb: 84_000, sessions: 12, sets: 180, reps: 900 },
    change: { volumeLb: 9_000, volumePct: 0.12, sessions: 2, sessionsPct: 0.2 },
    comparison: { count: 3, label: "school buses", unitLb: 24_000 },
    split: {
      main: { volumeLb: 76_000, sets: 160, reps: 800, lifts: 2, share: 0.905 },
      assistance: { volumeLb: 8_000, sets: 20, reps: 100, lifts: 1, share: 0.095 },
    },
    muscles: [
      { group: "legs", volumeLb: 50_000, sets: 90, reps: 450, lifts: 1, share: 0.6, trained: true },
      { group: "chest", volumeLb: 26_000, sets: 70, reps: 350, lifts: 1, share: 0.31, trained: true },
      { group: "arms", volumeLb: 8_000, sets: 20, reps: 100, lifts: 1, share: 0.09, trained: true },
      { group: "back", volumeLb: 0, sets: 0, reps: 0, lifts: 0, share: 0, trained: false },
      { group: "shoulders", volumeLb: 0, sets: 0, reps: 0, lifts: 0, share: 0, trained: false },
      { group: "core", volumeLb: 0, sets: 0, reps: 0, lifts: 0, share: 0, trained: false },
      { group: "other", volumeLb: 0, sets: 0, reps: 0, lifts: 0, share: 0, trained: false },
    ],
    lifts: [
      {
        exerciseId: 1,
        exerciseName: "Squat",
        volumeLb: 50_000,
        sets: 90,
        reps: 450,
        share: 0.6,
        isAssistance: false,
      },
      {
        exerciseId: 2,
        exerciseName: "Bench Press",
        volumeLb: 34_000,
        sets: 90,
        reps: 450,
        share: 0.4,
        isAssistance: false,
      },
      {
        exerciseId: 3,
        exerciseName: "Barbell Curl",
        volumeLb: 8_000,
        sets: 20,
        reps: 100,
        share: 0.095,
        isAssistance: true,
      },
    ],
    series: [
      {
        exerciseId: 1,
        exerciseName: "Squat",
        isAssistance: false,
        points: [
          { performedOn: "2026-03-02", topWeightLb: 200, e1rmLb: 233 },
          { performedOn: "2026-03-16", topWeightLb: 220, e1rmLb: 256 },
        ],
      },
    ],
    bodyweight: {
      points: [
        { performedOn: "2026-03-02", weightLb: 184 },
        { performedOn: "2026-03-09", weightLb: 182.5 },
        { performedOn: "2026-03-16", weightLb: 181.4 },
      ],
      startLb: 184,
      endLb: 181.4,
      lowLb: 181.4,
      highLb: 184,
      changeLb: -2.6,
      changePct: -0.0141,
    },
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
    hours: Array.from({ length: 24 }, (_, h) => (h === 6 ? 9 : 0)) as RackedReport["hours"],
    peakHour: 6,
    hourLabel: "Early bird",
    streak: { longestWeeks: 5, currentWeeks: 3 },
    attendance: { basis: "none", expected: 0, actual: 12, rate: 0, sessionsPerWeek: 2.75 },
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
    expect(screen.getByRole("heading", { name: "Bodyweight" })).toBeInTheDocument();
    expect(screen.getByText("48m")).toBeInTheDocument();
  });

  // The split divides the headline rather than qualifying it, so the page has to
  // show both halves against the same total the card above it prints.
  it("breaks the headline volume into main work and assistance", async () => {
    getRacked.mockResolvedValue({ data: fullReport(), error: undefined });
    render(Racked);

    const split = await screen.findByTestId("work-split");
    expect(split).toHaveTextContent("91%");
    expect(split).toHaveTextContent("main lifts");
    expect(split).toHaveTextContent("10%");
    expect(split).toHaveTextContent("across 1 movement");
  });

  // A lifter who does no assistance should not read a line telling them all
  // their work was the program's. That is the default, not news.
  it("says nothing about a split when there is no assistance", async () => {
    const report = fullReport();
    report.split = {
      main: { volumeLb: 84_000, sets: 180, reps: 900, lifts: 2, share: 1 },
      assistance: { volumeLb: 0, sets: 0, reps: 0, lifts: 0, share: 0 },
    };
    getRacked.mockResolvedValue({ data: report, error: undefined });
    render(Racked);

    await waitFor(() =>
      expect(screen.getByRole("heading", { name: "Where the weight went" })).toBeInTheDocument(),
    );
    expect(screen.queryByTestId("work-split")).not.toBeInTheDocument();
  });

  it("tags the assistance rows in the volume breakdown", async () => {
    getRacked.mockResolvedValue({ data: fullReport(), error: undefined });
    render(Racked);

    await waitFor(() => expect(screen.getByText("Barbell Curl")).toBeInTheDocument());
    // One tag, on the one lift that was only ever assistance.
    expect(screen.getAllByText("assistance")).toHaveLength(1);
  });

  // The muscle card answers "where the weight went" from the other side: that
  // one ranks the lifts, this one accounts for the body — including the parts of
  // it that went untouched, which no ranking of lifts can show.
  it("accounts for every muscle group, trained or not", async () => {
    getRacked.mockResolvedValue({ data: fullReport(), error: undefined });
    render(Racked);

    const card = await screen.findByTestId("stat-muscles");
    expect(card).toHaveTextContent("Legs");
    expect(card).toHaveTextContent("60%");
    // The groups with nothing against them keep their row and say so, rather
    // than being dropped or drawn as a very short bar.
    expect(card).toHaveTextContent("Core");
    expect(card).toHaveTextContent("not trained");
  });

  it("names the untrained groups in a sentence", async () => {
    getRacked.mockResolvedValue({ data: fullReport(), error: undefined });
    render(Racked);

    const card = await screen.findByTestId("stat-muscles");
    expect(card).toHaveTextContent(
      "Nothing logged for back, shoulders, core and other this month",
    );
  });

  // Every group trained is a good month, not an occasion for an empty sentence.
  it("says nothing about gaps when there are none", async () => {
    const report = fullReport();
    report.muscles = report.muscles.map((m) => ({
      ...m,
      trained: true,
      volumeLb: m.volumeLb || 1_000,
      sets: m.sets || 1,
    }));
    getRacked.mockResolvedValue({ data: report, error: undefined });
    render(Racked);

    const card = await screen.findByTestId("stat-muscles");
    expect(card).not.toHaveTextContent("Nothing logged for");
  });

  // A period with no work has no body to account for. The API sends null there
  // rather than seven groups the lifter failed to train, and the card goes with
  // it — telling a quiet month it trained none of seven things is piling on.
  it("drops the card entirely for a period with no work", async () => {
    getRacked.mockResolvedValue({ data: emptyReport(), error: undefined });
    render(Racked);

    await waitFor(() =>
      expect(screen.getByRole("heading", { name: "Racked" })).toBeInTheDocument(),
    );
    expect(screen.queryByTestId("stat-muscles")).not.toBeInTheDocument();
  });

  it("reports bodyweight as an end value and a change", async () => {
    getRacked.mockResolvedValue({ data: fullReport(), error: undefined });
    render(Racked);

    const card = await screen.findByTestId("stat-bodyweight");
    expect(card).toHaveTextContent("181.4");
    // A real minus sign, and the whole-number pound trimmed of its ".0".
    expect(card).toHaveTextContent("−2.6 lb");
    expect(card).toHaveTextContent("from 184 lb");
    expect(card).toHaveTextContent("Range 181.4–184 lb");
  });

  // Recording a bodyweight is optional on every session, so most periods hold
  // none — and an absent card is the right answer, not an empty one.
  it("omits the bodyweight card when the period holds no weigh-in", async () => {
    const report = fullReport();
    report.bodyweight = null;
    getRacked.mockResolvedValue({ data: report, error: undefined });
    render(Racked);

    await waitFor(() => expect(screen.getByText("84,000")).toBeInTheDocument());
    expect(screen.queryByTestId("stat-bodyweight")).not.toBeInTheDocument();
  });

  // One reading is a fact, not a trend. The card says the number and declines to
  // quote a change — reading changeLb as 0 here would claim a stability nobody
  // measured, and reading points[0] off an empty array would throw.
  it("quotes no change from a single weigh-in", async () => {
    const report = fullReport();
    report.bodyweight = {
      points: [{ performedOn: "2026-03-02", weightLb: 181 }],
      startLb: 181,
      endLb: 181,
      lowLb: 181,
      highLb: 181,
      changeLb: null,
      changePct: null,
    };
    getRacked.mockResolvedValue({ data: report, error: undefined });
    render(Racked);

    const card = await screen.findByTestId("stat-bodyweight");
    expect(card).toHaveTextContent("Weighed in once");
    expect(card).not.toHaveTextContent("Range");
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

  // No schedule means no target, so there must be no percentage — a rate against
  // a denominator nobody entered reads as a grade regardless of how it is labelled.
  it("reports frequency, not a rate, when the program carries no schedule", async () => {
    getRacked.mockResolvedValue({ data: fullReport(), error: undefined });
    render(Racked);

    await waitFor(() =>
      expect(screen.getByRole("heading", { name: "How often you trained" })).toBeInTheDocument(),
    );
    expect(screen.getByText("2.8")).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Attendance" })).not.toBeInTheDocument();
    expect(screen.queryByText(/estimated/)).not.toBeInTheDocument();
  });

  it("reports a rate when the program does carry a schedule", async () => {
    const report = fullReport();
    report.attendance = {
      basis: "weekday",
      expected: 13,
      actual: 12,
      rate: 0.923,
      sessionsPerWeek: 2.75,
    };
    getRacked.mockResolvedValue({ data: report, error: undefined });
    render(Racked);

    await waitFor(() =>
      expect(screen.getByRole("heading", { name: "Attendance" })).toBeInTheDocument(),
    );
    expect(screen.getByText("92%")).toBeInTheDocument();
    expect(screen.getByText("12 of 13 scheduled sessions")).toBeInTheDocument();
  });

  // Charts speak through pointer hover and title attributes, which a keyboard
  // and a screen reader never receive. The tables are the way in.
  it("offers every chart's numbers as a table", async () => {
    getRacked.mockResolvedValue({ data: fullReport(), error: undefined });
    render(Racked);

    await waitFor(() =>
      expect(screen.getByText("Change by lift as a table")).toBeInTheDocument(),
    );
    for (const label of [
      "Volume by weekday as a table",
      "Sessions by hour as a table",
      "Training days as a table",
    ]) {
      expect(screen.getByText(label)).toBeInTheDocument();
    }

    // And the rows carry real values, not just headers.
    expect(screen.getByRole("rowheader", { name: "Monday" })).toBeInTheDocument();
    expect(screen.getByRole("rowheader", { name: "6am" })).toBeInTheDocument();
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

  // Week is the cadence a lifter can still act on — a month is something to
  // reflect on, a week is something to correct.
  it("asks for the week when the week is selected", async () => {
    getRacked.mockResolvedValue({ data: fullReport(), error: undefined });
    const { getByRole } = render(Racked);

    await waitFor(() => expect(getRacked).toHaveBeenCalled());

    getByRole("radio", { name: "This week" }).click();
    await waitFor(() =>
      expect(getRacked).toHaveBeenLastCalledWith({ query: { period: "week" } }),
    );
  });

  // Two sections say nothing inside a one-week period, and one of them would
  // say it wrongly: the heatmap's columns are anchored on Sunday while a Racked
  // week runs Monday to Sunday, so a single column is the wrong seven days.
  it("drops the heatmap for a week and counts days trained instead", async () => {
    const report = fullReport();
    report.period = {
      kind: "week",
      start: "2026-03-16",
      end: "2026-03-22",
      label: "March 16–22 2026",
      inProgress: false,
    };
    getRacked.mockResolvedValue({ data: report, error: undefined });
    const { getByRole } = render(Racked);

    await waitFor(() => expect(getRacked).toHaveBeenCalled());
    getByRole("radio", { name: "This week" }).click();

    await waitFor(() =>
      expect(screen.queryByTestId("stat-training-days")).not.toBeInTheDocument(),
    );
    // "Week streak" inside one week reads 1 for anyone who trained at all.
    expect(screen.queryByText("Week streak")).not.toBeInTheDocument();
    expect(screen.getByText("Days trained")).toBeInTheDocument();
  });

  // The month and the year keep it — that is what it is for.
  it("keeps the heatmap and the streak for a month", async () => {
    getRacked.mockResolvedValue({ data: fullReport(), error: undefined });
    render(Racked);

    await screen.findByTestId("stat-training-days");
    expect(screen.getByText("Week streak")).toBeInTheDocument();
  });

  // Accuracy fixes from the recap audit — each pins a figure the page used to
  // read wrong off a report that was already right, or say more than it knew.

  // volumePct is null when the preceding period moved no weight, because growth
  // from nothing is not a ratio. Defaulting it to zero asserted "no change",
  // which is a claim the report explicitly declined to make.
  it("says nothing rather than 0% when there is no ratio to quote", async () => {
    const report = fullReport();
    report.change = { volumeLb: 9_000, volumePct: null, sessions: 2, sessionsPct: null };
    getRacked.mockResolvedValue({ data: report, error: undefined });
    render(Racked);

    await waitFor(() => expect(screen.getByText("84,000")).toBeInTheDocument());
    expect(screen.queryByText(/vs the previous month/)).not.toBeInTheDocument();
    expect(screen.queryByText(/^0% /)).not.toBeInTheDocument();
  });

  // A month three days old is compared against the first three days of the month
  // before it, not the whole of it. The page has to say which, or the figure
  // reads as a comparison against a full month.
  it("names the elapsed comparison while the period is still running", async () => {
    const report = fullReport();
    report.period = { ...report.period, inProgress: true };
    getRacked.mockResolvedValue({ data: report, error: undefined });
    render(Racked);

    await waitFor(() =>
      expect(screen.getByText(/vs the same point last month/)).toBeInTheDocument(),
    );
    expect(screen.queryByText(/vs the previous month/)).not.toBeInTheDocument();
  });

  it("compares against the whole period once it has finished", async () => {
    getRacked.mockResolvedValue({ data: fullReport(), error: undefined });
    render(Racked);

    await waitFor(() =>
      expect(screen.getByText(/vs the previous month/)).toBeInTheDocument(),
    );
    expect(screen.queryByText(/same point/)).not.toBeInTheDocument();
  });

  // The page used to pick its own peak out of the hours array and could land on
  // a different hour than hourLabel described, accenting one bar while the label
  // named another.
  it("accents the hour the report names, not one of its own choosing", async () => {
    const report = fullReport();
    // Two hours tied on sessions; the report has already resolved the tie.
    report.hours = Array.from({ length: 24 }, (_, h) =>
      h === 6 || h === 18 ? 2 : 0,
    ) as RackedReport["hours"];
    report.peakHour = 18;
    report.hourLabel = "Evening lifter";
    getRacked.mockResolvedValue({ data: report, error: undefined });
    render(Racked);

    await waitFor(() => expect(screen.getByText("Evening lifter")).toBeInTheDocument());

    // Every bar carries a title; only the accented one carries data-peak, so
    // this asserts the highlight rather than the mere presence of a 6pm bar.
    const peaks = Array.from(document.querySelectorAll("[data-peak='true']"));
    const titles = peaks.map((el) => el.getAttribute("title"));
    expect(titles).toContain("6pm · 2 sessions");
    expect(titles).not.toContain("6am · 2 sessions");
  });

  // Two figures on this page measure improvement differently — the chart indexes
  // each lift off its own first session, while "most improved" compares the best
  // of the period's start against the best of its end. Both are right; saying so
  // is what stops them reading as a contradiction.
  it("names the basis of each improvement measure", async () => {
    getRacked.mockResolvedValue({ data: fullReport(), error: undefined });
    render(Racked);

    await waitFor(() =>
      expect(screen.getByRole("heading", { name: "Most improved" })).toBeInTheDocument(),
    );
    expect(screen.getByText("start vs end of period")).toBeInTheDocument();

    await screen.getByText("Change by lift as a table").click();
    expect(
      screen.getByRole("columnheader", { name: "Since first session" }),
    ).toBeInTheDocument();
  });

  it("offers the recap as an image once there is something to show", async () => {
    getRacked.mockResolvedValue({ data: fullReport(), error: undefined });
    render(Racked);

    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Share" })).toBeInTheDocument(),
    );
  });

  // A picture of the "nothing logged" card says nothing, and the page already
  // hides every statistic behind the same condition.
  it("offers nothing to share from a period with no sessions", async () => {
    getRacked.mockResolvedValue({ data: emptyReport(), error: undefined });
    render(Racked);

    await waitFor(() =>
      expect(screen.getByText(/Nothing logged in March 2026/)).toBeInTheDocument(),
    );
    expect(screen.queryByRole("button", { name: "Share" })).not.toBeInTheDocument();
  });

  // jsdom has no 2D context, so this exercises the branch a browser reaches
  // only with canvas disabled: the dialog opens, says it could not draw the
  // card, and leaves the page standing.
  it("opens the share dialog and survives a canvas it cannot draw on", async () => {
    getRacked.mockResolvedValue({ data: fullReport(), error: undefined });
    render(Racked);

    await waitFor(() => screen.getByRole("button", { name: "Share" }));
    screen.getByRole("button", { name: "Share" }).click();

    await waitFor(() =>
      expect(screen.getByText("Couldn't draw the card.")).toBeInTheDocument(),
    );
    expect(screen.getByRole("heading", { name: "Racked" })).toBeInTheDocument();
  });
});

// Two sessions of the same lift on one day is ordinary — a morning and an
// evening workout — and it used to crash the page.
//
// Every keyed {#each} whose key was built from a date collided: the trend chart
// keyed its points by performedOn, the records list by date+lift+kind, the
// stalls list by date+lift. Svelte throws each_key_duplicate on the second one,
// so the whole page died rather than degrading. Position is the identity in all
// three lists, so they key by index now, and this pins that.
describe("Racked with two sessions of one lift on the same day", () => {
  function sameDayReport(): RackedReport {
    const r = fullReport();
    const day = "2026-08-06";

    // One lift, two sessions, same date — two points at the same x.
    r.series = [
      {
        exerciseId: 1,
        exerciseName: "Squat",
        isAssistance: false,
        points: [
          { performedOn: day, topWeightLb: 200, e1rmLb: 233 },
          { performedOn: day, topWeightLb: 205, e1rmLb: 239 },
        ],
      },
    ];
    // Both sessions set a weight record, so date + lift + kind repeats.
    r.prs = [200, 205].map((weightLb) => ({
      kind: "weight",
      performedOn: day,
      exerciseId: 1,
      exerciseName: "Squat",
      weightLb,
      reps: 5,
      valueLb: weightLb,
      previousLb: weightLb - 5,
    }));
    // And both dropped back, so date + lift repeats too.
    r.deloads = [
      { exerciseId: 1, exerciseName: "Squat", performedOn: day, fromLb: 225, toLb: 210, recovered: false, recoveredOn: null },
      { exerciseId: 1, exerciseName: "Squat", performedOn: day, fromLb: 210, toLb: 195, recovered: false, recoveredOn: null },
    ];
    return r;
  }

  it("renders instead of throwing on duplicate keys", async () => {
    getRacked.mockResolvedValue({ data: sameDayReport(), error: undefined });
    render(Racked);

    await waitFor(() =>
      expect(screen.getByRole("heading", { name: "2 personal records" })).toBeInTheDocument(),
    );
    expect(screen.getByRole("heading", { name: "Stalls and comebacks" })).toBeInTheDocument();
    // Both entries of each repeated-key list survive rather than one winning.
    expect(screen.getAllByText("Squat").length).toBeGreaterThanOrEqual(2);
  });
});
