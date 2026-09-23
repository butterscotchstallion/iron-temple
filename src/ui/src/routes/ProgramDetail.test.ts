import { fireEvent, render, screen, waitFor } from "@testing-library/svelte";
import { beforeEach, describe, expect, it, vi } from "vitest";
import ProgramDetail from "./ProgramDetail.svelte";
import type {
  Program,
  ProgramDayAssistance,
  SessionSummary,
} from "../lib/api";
import { clearCache } from "../lib/cache.svelte";
import { testProgramSummary } from "../lib/testFixtures";
import { todayIso } from "../lib/calendar";
import { todayWeekday } from "../lib/weekday";

// A render test for a route, which the suite otherwise leaves to Playwright.
//
// It earns its place on one card: the day scheduled for today and already
// trained, which is the only one whose date moves. nextDueOn pushes it a week
// out, and from that moment nothing on it may speak about the session just
// finished — a "Done today" badge or a View link beside next week's date reads
// as a claim about the workout the card is dated for. That rule lives entirely
// in template branches, where the lib tests below cannot see it, and the whole
// point of them is what is ABSENT: a refactor that puts the badge back breaks
// no assertion in trainedToday.test.ts.
//
// Dates come off the real clock rather than a frozen one, because the thing
// under test is "today" and the component reads it three ways — todayStatus,
// nextDueOn and weekdayOptions. Every fixture is stated relative to today, so
// the suite is honest on any day of the week.

const getProgram = vi.hoisted(() => vi.fn());
const previewNextSessions = vi.hoisted(() => vi.fn());
const previewNextSession = vi.hoisted(() => vi.fn());
const updateAssistance = vi.hoisted(() => vi.fn());
vi.mock("../lib/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../lib/api")>()),
  getProgram,
  previewNextSessions,
  previewNextSession,
  updateAssistance,
}));

const loadHomeSessions = vi.hoisted(() => vi.fn());
const watchHomeSessions = vi.hoisted(() => vi.fn());
vi.mock("../lib/homeData", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../lib/homeData")>()),
  loadHomeSessions,
  watchHomeSessions,
}));

/** A one-day program, since every case here turns on a single card. */
function program(
  weekday: number | null,
  assistance: ProgramDayAssistance[] = [],
): Program {
  return {
    ...testProgramSummary(),
    days: [
      {
        id: 7,
        name: "Workout A",
        position: 1,
        weekday,
        exercises: [],
        assistance,
      },
    ],
  };
}

/** One accessory on the day, carrying no rep range unless given one. */
function curl(over: Partial<ProgramDayAssistance> = {}): ProgramDayAssistance {
  return {
    id: 3,
    exerciseId: 9,
    exerciseName: "Dumbbell Curl",
    equipment: "dumbbell",
    position: 1,
    sets: 3,
    reps: 10,
    weightLb: 30,
    ...over,
  };
}

/** A session performed today on that day, finished unless told otherwise. */
function today(over: Partial<SessionSummary> = {}): SessionSummary {
  return {
    id: 42,
    programId: 1,
    programName: "StrongLifts 5x5",
    programDayId: 7,
    programDayName: "Workout A",
    performedOn: todayIso(),
    setCount: 5,
    completedSetCount: 5,
    volumeLb: 5000,
    isOver: true,
    exercises: [],
    ...over,
  };
}

/** Render the screen for `weekday` with `sessions` behind it, once loaded. */
async function show(
  weekday: number | null,
  sessions: SessionSummary[],
  assistance: ProgramDayAssistance[] = [],
) {
  getProgram.mockResolvedValue({
    status: 200,
    data: program(weekday, assistance),
  });
  loadHomeSessions.mockResolvedValue({ status: 200, data: { items: sessions } });
  render(ProgramDetail, { params: { id: "1" } });
  await screen.findByRole("heading", { name: "Workout A" });
}

/** The day's one action button, whichever of the three labels it is wearing. */
function action(name: string): HTMLElement {
  return screen.getByRole("button", { name });
}

beforeEach(() => {
  clearCache();
  getProgram.mockReset();
  loadHomeSessions.mockReset();
  // Mocked rather than driven through its own timer: a call from inside
  // homeData.ts does not go through vi.mock, so the real watcher here would
  // reach the real client. Its mechanics are pinned in homeData.test.ts; what
  // this file is for is what the card does with the list it is handed.
  watchHomeSessions.mockReset();
  watchHomeSessions.mockReturnValue(() => {});
  // The prescription is beside the point here — no exercises keeps the cards to
  // the header row these assertions are about.
  previewNextSessions.mockReset();
  previewNextSessions.mockResolvedValue({
    status: 200,
    data: { programId: 1, layoff: null, days: [] },
  });
  previewNextSession.mockReset();
  previewNextSession.mockResolvedValue({
    status: 200,
    data: {
      programId: 1,
      programDayId: 7,
      programDayName: "Workout A",
      exercises: [],
      layoff: null,
    },
  });
  updateAssistance.mockReset();
});

describe("ProgramDetail day card", () => {
  it("says nothing about today once the card is dated next week", async () => {
    await show(todayWeekday(), [today()]);

    // The date it moved to, which is the only thing on the card that accounts
    // for the move now that the badge is gone.
    expect(screen.getByText(/^Next /)).toBeInTheDocument();
    expect(screen.queryByText("Done today")).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "View" })).not.toBeInTheDocument();
    // "Start", not "Start again": what this card offers is its own next session
    // begun early, not a repeat of one it no longer mentions.
    expect(action("Start")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Start again" })).not
      .toBeInTheDocument();
    // Demoted all the same — the lifter HAS trained this day today, and the
    // filled Start belongs to whichever card is actually due. The class is the
    // only handle on that: the variant reaches the DOM as nothing else.
    expect(action("Start").className).toContain("border-border");
    expect(action("Start").className).not.toContain("bg-primary");
  });

  it("keeps the badge on a day trained off its own weekday", async () => {
    // Two days from now, so the card is due later this week and its date never
    // moved. There the badge is the only thing explaining why a day due later
    // was worked today.
    await show((todayWeekday() + 2) % 7, [today()]);

    expect(screen.getByText("Done today")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "View" })).toBeInTheDocument();
    expect(action("Start again")).toBeInTheDocument();
    expect(screen.queryByText(/^Next /)).not.toBeInTheDocument();
  });

  it("offers Resume while today's session is still open", async () => {
    // An unfinished session is still due TODAY — nextDueOn only moves a day
    // that was finished — so this card keeps every word about today.
    await show(todayWeekday(), [today({ completedSetCount: 2, isOver: false })]);

    expect(screen.getByText("In progress")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Resume" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Start/ })).not
      .toBeInTheDocument();
    expect(screen.queryByText(/^Next /)).not.toBeInTheDocument();
  });

  // The whole point of watching the list: this screen used to go on offering
  // Resume for a workout finished at the rack on a phone, for as long as the tab
  // stayed open. Nothing here re-mounts or reloads the program — one fresh list
  // arrives and every claim the card makes about today corrects itself.
  it("stops offering Resume once the workout is finished elsewhere", async () => {
    await show(todayWeekday(), [today({ completedSetCount: 2, isOver: false })]);
    expect(screen.getByRole("link", { name: "Resume" })).toBeInTheDocument();

    const [[onFresh]] = watchHomeSessions.mock.calls;
    onFresh({ items: [today()] });

    await waitFor(() =>
      expect(screen.queryByRole("link", { name: "Resume" })).not
        .toBeInTheDocument(),
    );
    expect(screen.queryByText("In progress")).not.toBeInTheDocument();
    // Finished, so nextDueOn has moved the card to next week — where it says
    // nothing about the session just completed and offers its own next one.
    expect(screen.getByText(/^Next /)).toBeInTheDocument();
    expect(action("Start")).toBeInTheDocument();
  });

  it("leads with a filled Start on a day not yet trained", async () => {
    await show(todayWeekday(), []);

    expect(action("Start").className).toContain("bg-primary");
    expect(screen.queryByText("Done today")).not.toBeInTheDocument();
    expect(screen.queryByText(/^Next /)).not.toBeInTheDocument();
  });
});

// The control that unsticks a lift already on a day.
//
// Its absence is why double progression shipped switched off in practice: the
// rep range is what decides whether an assistance weight ever moves, the picker
// was the only place it could be set, and the picker only runs while a lift is
// being ADDED. So a curl added without one was frozen at its first weight, and
// the only way out was to delete it and add it back. The endpoint accepted this
// patch the whole time; nothing called it.
describe("ProgramDetail assistance editing", () => {
  const day = 7;

  it("puts a rep range on a lift that has none", async () => {
    await show(todayWeekday(), [], [curl()]);
    updateAssistance.mockResolvedValue({
      status: 200,
      data: curl({ reps: 8, repMin: 8, repMax: 12 }),
    });

    await fireEvent.click(
      screen.getByRole("button", { name: "Edit Dumbbell Curl on Workout A" }),
    );
    await fireEvent.click(await screen.findByLabelText("Use a rep range"));
    await fireEvent.click(screen.getByRole("button", { name: "Save" }));

    // reps is the BOTTOM of the range: a set is complete at the bottom and the
    // weight moves at the top, which is what lets a lifter finish at 8s without
    // the app scoring it a failure.
    //
    // No weightLb: the lifter came here for the rep range and never touched the
    // weight box, and naming a weight is what pins the lift to it.
    await waitFor(() =>
      expect(updateAssistance).toHaveBeenCalledWith(1, day, 3, {
        sets: 3,
        reps: 8,
        repMin: 8,
        repMax: 12,
      }),
    );
  });

  // null, not omitted. Absent means "leave it alone" on this endpoint, so
  // turning the range off has to be said out loud.
  it("clears the range with an explicit null", async () => {
    await show(todayWeekday(), [], [curl({ reps: 8, repMin: 8, repMax: 12 })]);
    updateAssistance.mockResolvedValue({ status: 200, data: curl() });

    await fireEvent.click(
      screen.getByRole("button", { name: "Edit Dumbbell Curl on Workout A" }),
    );
    await fireEvent.click(await screen.findByLabelText("Use a rep range"));
    await fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(updateAssistance).toHaveBeenCalledWith(1, day, 3, {
        sets: 3,
        // The reps the row already carried, which while ranged was the BOTTOM
        // of the range. Turning the range off keeps the number that was on
        // screen rather than inventing a fresh default.
        reps: 8,
        // Still no weightLb — untouched here too. repMin/repMax ARE sent as
        // null because absent and null mean different things for them; the
        // weight has no such distinction to draw.
        repMin: null,
        repMax: null,
      }),
    );
  });

  // The stepper offers the jump this lifter's rack can actually make. A pair of
  // bells moves 10 with no profile loaded, not the bar's 5 — asking for 5 is
  // asking for a bell that is not on the rack.
  it("steps the weight by what the movement's equipment builds", async () => {
    await show(todayWeekday(), [], [curl()]);

    await fireEvent.click(
      screen.getByRole("button", { name: "Edit Dumbbell Curl on Workout A" }),
    );
    expect(await screen.findByLabelText("Weight (lb)")).toHaveAttribute("step", "10");
  });
});

// The weight box opens on the number the row is showing, and an untouched box
// says nothing to the server.
//
// Both halves of "I can't seem to change the weight" lived here. The row renders
// the PRESCRIBED weight — carried forward from the last session — while the
// editor prefilled the STORED one, so a curl reading 50 opened a box saying 30.
// And because naming a weight is what pins the lift to it, an editor that always
// sent the box would pin every sets-only edit to whatever that stale number was.
describe("ProgramDetail assistance weight editing", () => {
  const day = 7;

  /** The day's preview, with the curl carried forward past its stored weight. */
  function prescribedAt(weightLb: number) {
    previewNextSessions.mockResolvedValue({
      status: 200,
      data: {
        programId: 1,
        layoff: null,
        days: [
          {
            programDayId: day,
            exercises: [
              {
                exerciseId: 9,
                exerciseName: "Dumbbell Curl",
                kind: "assistance",
                sets: 3,
                reps: 10,
                weightLb,
                restSeconds: 90,
                progression: {
                  status: "advance",
                  failureCount: 0,
                  previousWeightLb: weightLb - 10,
                },
              },
            ],
          },
        ],
      },
    });
  }

  it("opens the weight box on the weight actually in force", async () => {
    prescribedAt(50);
    // Stored at 30 — what it was added at, and not what the row says.
    await show(todayWeekday(), [], [curl({ weightLb: 30 })]);

    await fireEvent.click(
      screen.getByRole("button", { name: "Edit Dumbbell Curl on Workout A" }),
    );

    expect(await screen.findByLabelText("Weight (lb)")).toHaveValue(50);
  });

  it("sends a weight the lifter changed", async () => {
    prescribedAt(50);
    await show(todayWeekday(), [], [curl({ weightLb: 30 })]);
    updateAssistance.mockResolvedValue({ status: 200, data: curl({ weightLb: 30 }) });

    await fireEvent.click(
      screen.getByRole("button", { name: "Edit Dumbbell Curl on Workout A" }),
    );
    await fireEvent.input(await screen.findByLabelText("Weight (lb)"), {
      target: { value: "30" },
    });
    await fireEvent.click(screen.getByRole("button", { name: "Save" }));

    // 30 reaches the server even though the STORED weight was already 30 — it
    // is the prescribed 50 the lifter was changing, and presence is what pins.
    await waitFor(() =>
      expect(updateAssistance).toHaveBeenCalledWith(
        1,
        day,
        3,
        expect.objectContaining({ weightLb: 30 }),
      ),
    );
  });

  it("omits the weight when the lifter only changed the sets", async () => {
    prescribedAt(50);
    await show(todayWeekday(), [], [curl({ weightLb: 30 })]);
    updateAssistance.mockResolvedValue({ status: 200, data: curl({ sets: 4 }) });

    await fireEvent.click(
      screen.getByRole("button", { name: "Edit Dumbbell Curl on Workout A" }),
    );
    await fireEvent.input(await screen.findByLabelText("Sets"), {
      target: { value: "4" },
    });
    await fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(updateAssistance).toHaveBeenCalled());
    // Absent, not 50. Sending it would pin the lift to the weight it happened
    // to be prescribed and stop the carry-forward moving it again.
    const body = updateAssistance.mock.calls[0][3];
    expect(body).not.toHaveProperty("weightLb");
  });
});
