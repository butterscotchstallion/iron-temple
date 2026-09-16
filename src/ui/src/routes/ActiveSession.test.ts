import { fireEvent, render, screen, waitFor } from "@testing-library/svelte";
import { afterAll, beforeEach, describe, expect, it, vi } from "vitest";
import ActiveSession from "./ActiveSession.svelte";
import type { Exercise, Session, SessionSet } from "../lib/api";
import { clearQueue, queuedCount } from "../lib/writeQueue.svelte";
import { markUnreachable, resetConnectivity } from "../lib/connectivity.svelte";

// A render test for a route, which the suite otherwise leaves to Playwright.
//
// It covers adding a whole lift mid-workout, and it earns its place on the
// offline half: that path never touches the network, so Playwright would be
// asserting on the same optimistic rows through a much heavier harness — and
// the thing worth pinning is that the sets appear at all when nothing answers.

const getSession = vi.hoisted(() => vi.fn());
const listExercises = vi.hoisted(() => vi.fn());
const addSessionAssistance = vi.hoisted(() => vi.fn());
vi.mock("../lib/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../lib/api")>()),
  getSession,
  listExercises,
  addSessionAssistance,
}));

// Confetti needs a 2D context jsdom does not have, and it throws inside a
// requestAnimationFrame callback rather than as a test failure.
vi.mock("../lib/celebrate", () => ({ celebrate: vi.fn() }));

const props = { params: { id: "1" } };

function mkSet(over: Partial<SessionSet> & Pick<SessionSet, "id">): SessionSet {
  return {
    exerciseId: 1,
    exerciseName: "Squat",
    kind: "main",
    setNumber: 1,
    targetReps: 5,
    actualReps: null,
    weightLb: 200,
    completed: false,
    restSeconds: 180,
    equipment: "barbell",
    ...over,
  };
}

function mkSession(over: Partial<Session> = {}): Session {
  return {
    id: 1,
    programId: 1,
    programName: "StrongLifts 5x5",
    programDayId: 7,
    programDayName: "Workout A",
    performedOn: "2026-09-13",
    notes: "",
    createdAt: "2026-09-13T18:00:00Z",
    finishedAt: null,
    isOver: false,
    previousBests: [],
    sets: [1, 2].map((n) => mkSet({ id: n, setNumber: n })),
    ...over,
  };
}

const curl: Exercise = {
  id: 9,
  name: "Barbell Curl",
  muscleGroup: "arms",
  equipment: "barbell",
  isAccessory: true,
  isCustom: false,
  restSeconds: 90,
  topSet: { weightLb: 30, performedOn: "2026-09-01" },
  lastPerformedOn: "2026-09-01",
  performedSessions: 4,
};

const ok = (data: unknown, status = 200) => ({ status, data, headers: new Headers() });

beforeEach(() => {
  getSession.mockReset();
  listExercises.mockReset();
  addSessionAssistance.mockReset();
  clearQueue();
  resetConnectivity();
  localStorage.clear();

  getSession.mockResolvedValue(ok(mkSession()));
  listExercises.mockResolvedValue(ok([curl]));
});

// bits-ui's scroll lock resets document styles on a timer at unmount, which can
// fire after jsdom has torn down. Deliberately afterAll.
afterAll(async () => {
  await new Promise((resolve) => setTimeout(resolve, 100));
});

/**
 * Open the picker and choose the one movement the library offers.
 *
 * findAll, not find: a lift the lifter has trained recently is listed twice —
 * once under "Recent" and once in its muscle group — which is the picker doing
 * its job. Either row is the same movement.
 */
async function pickCurl() {
  await fireEvent.click(await screen.findByRole("button", { name: "Add assistance" }));
  const rows = await screen.findAllByRole("button", { name: /Barbell Curl/ });
  await fireEvent.click(rows[0]);
}

/** Confirm the picker's second step. */
async function confirmAdd() {
  await fireEvent.click(await screen.findByRole("button", { name: /Add to this workout/ }));
}

describe("ActiveSession: adding assistance", () => {
  it("offers the control while the workout is open", async () => {
    render(ActiveSession, props);
    expect(await screen.findByRole("button", { name: "Add assistance" })).toBeInTheDocument();
  });

  // A finished session is a record to read. Its shape is fixed, and the server
  // refuses this write anyway.
  it("does not offer it once the workout is over", async () => {
    getSession.mockResolvedValue(ok(mkSession({ isOver: true, finishedAt: "2026-09-13T19:00:00Z" })));
    render(ActiveSession, props);

    await waitFor(() => expect(screen.getByText(/Finished/)).toBeInTheDocument());
    expect(screen.queryByRole("button", { name: "Add assistance" })).not.toBeInTheDocument();
  });

  it("adds the lift's sets to the workout", async () => {
    addSessionAssistance.mockResolvedValue(
      ok(
        [1, 2, 3].map((n) =>
          mkSet({
            id: 100 + n,
            exerciseId: 9,
            exerciseName: "Barbell Curl",
            kind: "assistance",
            setNumber: n,
            targetReps: 10,
            weightLb: 30,
            restSeconds: 90,
          }),
        ),
        201,
      ),
    );

    render(ActiveSession, props);
    await pickCurl();
    await confirmAdd();

    // The card appears, under the divider that separates it from the barbell
    // work, and the picker closes behind it.
    await waitFor(() =>
      expect(screen.getByRole("heading", { name: "Barbell Curl" })).toBeInTheDocument(),
    );
    expect(screen.getByText("Assistance")).toBeInTheDocument();
    // No range, because a lift without one now runs the linear engine — it
    // joins the day with a progression behind it either way.
    expect(addSessionAssistance).toHaveBeenCalledWith(1, {
      exerciseId: 9,
      sets: 3,
      reps: 10,
      weightLb: 30,
    });
  });

  // The rack in the basement. Nothing answers, so nothing is sent — and the
  // lifter still gets their sets to tap through.
  it("shows the sets with no network, and queues the write", async () => {
    render(ActiveSession, props);
    // After the load, not before: a successful GET tells connectivity the
    // network is fine, so going offline first would simply be undone by the
    // session arriving.
    await screen.findByRole("button", { name: "Add assistance" });
    markUnreachable();

    await pickCurl();
    await confirmAdd();

    await waitFor(() =>
      expect(screen.getByRole("heading", { name: "Barbell Curl" })).toBeInTheDocument(),
    );
    expect(addSessionAssistance).not.toHaveBeenCalled();
    expect(queuedCount()).toBe(1);
  });

  // The weight is the one number worth not retyping at the rack.
  it("starts the weight at the lifter's heaviest set of that movement", async () => {
    render(ActiveSession, props);
    await pickCurl();

    expect(await screen.findByText(/Your heaviest so far: 30 lb/)).toBeInTheDocument();
    expect(screen.getByLabelText(/Weight/)).toHaveValue(30);
  });

  // A lift already in the workout can only come back as a 409, so it is not
  // offered — the conflict is a backstop against a race, not a path.
  it("does not offer a movement already in the workout", async () => {
    listExercises.mockResolvedValue(
      ok([curl, { ...curl, id: 1, name: "Squat", isAccessory: true }]),
    );
    render(ActiveSession, props);

    await fireEvent.click(await screen.findByRole("button", { name: "Add assistance" }));
    const offered = await screen.findAllByRole("button", { name: /Barbell Curl/ });
    expect(offered.length).toBeGreaterThan(0);
    // Scoped to the picker: the page also draws a Squat exercise card, and the
    // point here is that the picker does not OFFER it.
    const panel = offered[0].closest("div.flex-col");
    expect(panel?.textContent).not.toContain("Squat");
  });

  // The endpoint carries a rep range now — it could not before, which left
  // every lift added at the rack with no way to progress at all. Offered here
  // rather than only on the program page, and off by default like everywhere
  // else, because a lift without one runs the linear engine.
  it("offers a rep range, off by default", async () => {
    render(ActiveSession, props);
    await pickCurl();

    await screen.findByText(/Leave the weight at 0/);
    expect(screen.getByLabelText(/Use a rep range/)).not.toBeChecked();
  });

  // Turning it on is one click, and the range then reaches the program day the
  // lift joins — the thing this endpoint could not express at all before.
  it("sends the range when the lifter turns it on", async () => {
    addSessionAssistance.mockResolvedValue(
      ok(
        [1, 2, 3].map((n) =>
          mkSet({
            id: 100 + n,
            exerciseId: 9,
            exerciseName: "Barbell Curl",
            kind: "assistance",
            setNumber: n,
            targetReps: 10,
            weightLb: 30,
            restSeconds: 90,
          }),
        ),
        201,
      ),
    );

    render(ActiveSession, props);
    await pickCurl();
    await fireEvent.click(await screen.findByLabelText(/Use a rep range/));
    await confirmAdd();

    await waitFor(() => expect(addSessionAssistance).toHaveBeenCalled());
    // reps is the BOTTOM of the range: a set is complete at the bottom and the
    // weight moves at the top.
    expect(addSessionAssistance).toHaveBeenCalledWith(1, {
      exerciseId: 9,
      sets: 3,
      reps: 8,
      weightLb: 30,
      repMin: 8,
      repMax: 12,
    });
  });

  // A refusal is real. The panel stays open with the numbers intact rather than
  // making the lifter pick the movement again.
  it("reports a refusal and keeps the picker open", async () => {
    addSessionAssistance.mockResolvedValue(ok({ code: "already_prescribed" }, 409));

    render(ActiveSession, props);
    await pickCurl();
    await confirmAdd();

    await waitFor(() =>
      expect(screen.getByText("Couldn't add that lift.")).toBeInTheDocument(),
    );
    expect(screen.queryByRole("heading", { name: "Barbell Curl" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Add to this workout/ })).toBeInTheDocument();
  });
});
