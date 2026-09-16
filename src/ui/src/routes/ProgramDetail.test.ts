import { render, screen } from "@testing-library/svelte";
import { beforeEach, describe, expect, it, vi } from "vitest";
import ProgramDetail from "./ProgramDetail.svelte";
import type { Program, SessionSummary } from "../lib/api";
import { clearCache } from "../lib/cache.svelte";
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
vi.mock("../lib/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../lib/api")>()),
  getProgram,
  previewNextSessions,
}));

const loadHomeSessions = vi.hoisted(() => vi.fn());
vi.mock("../lib/homeData", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../lib/homeData")>()),
  loadHomeSessions,
}));

/** A one-day program, since every case here turns on a single card. */
function program(weekday: number | null): Program {
  return {
    id: 1,
    name: "StrongLifts 5x5",
    description: "",
    progressionKind: "linear",
    days: [
      {
        id: 7,
        name: "Workout A",
        position: 1,
        weekday,
        exercises: [],
        assistance: [],
      },
    ],
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
async function show(weekday: number | null, sessions: SessionSummary[]) {
  getProgram.mockResolvedValue({ status: 200, data: program(weekday) });
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
  // The prescription is beside the point here — no exercises keeps the cards to
  // the header row these assertions are about.
  previewNextSessions.mockReset();
  previewNextSessions.mockResolvedValue({
    status: 200,
    data: { programId: 1, layoff: null, days: [] },
  });
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

  it("leads with a filled Start on a day not yet trained", async () => {
    await show(todayWeekday(), []);

    expect(action("Start").className).toContain("bg-primary");
    expect(screen.queryByText("Done today")).not.toBeInTheDocument();
    expect(screen.queryByText(/^Next /)).not.toBeInTheDocument();
  });
});
