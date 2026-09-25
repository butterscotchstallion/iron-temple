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
const updateSessionSet = vi.hoisted(() => vi.fn());
vi.mock("../lib/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../lib/api")>()),
  getSession,
  updateSessionSet,
  listExercises,
  addSessionAssistance,
}));

// Confetti needs a 2D context jsdom does not have, and it throws inside a
// requestAnimationFrame callback rather than as a test failure.
vi.mock("../lib/celebrate", () => ({ celebrate: vi.fn() }));
import { celebrate } from "../lib/celebrate";

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
    isBonus: false,
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
  updateSessionSet.mockReset();
  vi.mocked(celebrate).mockClear();
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
    // Carrying the range the picker defaults to, with reps at its BOTTOM. A
    // lift added at the rack progresses either way; the range is what keeps it
    // from advancing by the whole of the rack's step every session.
    expect(addSessionAssistance).toHaveBeenCalledWith(1, {
      exerciseId: 9,
      sets: 3,
      reps: 8,
      weightLb: 30,
      repMin: 8,
      repMax: 12,
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
  // rather than only on the program page, and on by default like everywhere
  // else: a lift without one advances by the rack's own step every session,
  // which on a pair of dumbbells is 10 lb.
  it("offers a rep range, on by default", async () => {
    render(ActiveSession, props);
    await pickCurl();

    await screen.findByText(/Leave the weight at 0/);
    expect(screen.getByLabelText(/Use a rep range/)).toBeChecked();
  });

  // The range reaches the program day the lift joins — the thing this endpoint
  // could not express at all before. No click here: it rides along by default,
  // which is the point of the default.
  it("sends the range with a lift added at the rack", async () => {
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

// Confetti is the loudest thing the app does, and until the split it fired on
// every lift of a lifter's first workout — the bests map was absent for all of
// them, read as a zero, and cleared by anything. What made a real record feel
// like nothing was being told six times before it that you had set one.
describe("ActiveSession: records and first times", () => {
  /** One set, one rep to its target, so a single tap completes it. */
  function oneTap(over: Partial<Session> = {}): Session {
    return mkSession({
      sets: [mkSet({ id: 1, setNumber: 1, targetReps: 1, weightLb: 200 })],
      ...over,
    });
  }

  async function tapTheSet() {
    const button = await screen.findByRole("button", { name: /^Set 1/ });
    await fireEvent.click(button);
  }

  it("celebrates a set above the lift's standing best", async () => {
    getSession.mockResolvedValue(
      ok(oneTap({ previousBests: [{ exerciseId: 1, weightLb: 195, e1rmLb: 228 }] })),
    );
    updateSessionSet.mockResolvedValue(
      ok(mkSet({ id: 1, setNumber: 1, targetReps: 1, actualReps: 1, completed: true })),
    );

    render(ActiveSession, props);
    await tapTheSet();

    expect(await screen.findByText(/New PR!/)).toBeInTheDocument();
    expect(celebrate).toHaveBeenCalled();
  });

  // A lift absent from previousBests has no history, so there was nothing to
  // beat. It still says so — a lifter who just did something for the first time
  // should hear about it — but without the confetti a record earns.
  it("marks a first-ever lift without claiming a record", async () => {
    getSession.mockResolvedValue(ok(oneTap({ previousBests: [] })));
    updateSessionSet.mockResolvedValue(
      ok(mkSet({ id: 1, setNumber: 1, targetReps: 1, actualReps: 1, completed: true })),
    );

    render(ActiveSession, props);
    await tapTheSet();

    expect(await screen.findByText(/First time!/)).toBeInTheDocument();
    expect(screen.queryByText(/New PR!/)).not.toBeInTheDocument();
    expect(celebrate).not.toHaveBeenCalled();
  });

  // The banner dismisses itself after six seconds. Only a NEW record may restart
  // that clock: an ordinary completed set that beat nothing must not, or on a 5x5
  // the four sets after a record each re-arm it and the banner sits there for the
  // rest of the workout announcing something that happened ten minutes ago.
  it("does not let a later ordinary set keep a stale banner alive", async () => {
    vi.useFakeTimers();
    try {
      // Two sets of one lift against a standing best of 195. The first clears it
      // and is a record; the second is a lighter back-off set that beats nothing.
      getSession.mockResolvedValue(
        ok(
          mkSession({
            previousBests: [{ exerciseId: 1, weightLb: 195, e1rmLb: 228 }],
            sets: [
              mkSet({ id: 1, setNumber: 1, targetReps: 1, weightLb: 200 }),
              mkSet({ id: 2, setNumber: 2, targetReps: 1, weightLb: 185 }),
            ],
          }),
        ),
      );
      updateSessionSet.mockImplementation((_s: number, setId: number) =>
        Promise.resolve(
          ok(mkSet({ id: setId, setNumber: setId, targetReps: 1, actualReps: 1, completed: true })),
        ),
      );

      render(ActiveSession, props);
      await vi.waitFor(() => expect(screen.getByRole("button", { name: /^Set 1/ })).toBeTruthy());

      await fireEvent.click(screen.getByRole("button", { name: /^Set 1/ }));
      await vi.waitFor(() => expect(screen.getByText(/New PR!/)).toBeTruthy());

      // Four seconds later, the second set — same weight, so not a record.
      await vi.advanceTimersByTimeAsync(4000);
      await fireEvent.click(screen.getByRole("button", { name: /^Set 2/ }));

      // Past the original six seconds. The banner must be gone: the second set
      // had no news of its own and so bought the first none either.
      await vi.advanceTimersByTimeAsync(2500);
      expect(screen.queryByText(/New PR!/)).not.toBeInTheDocument();
    } finally {
      vi.useRealTimers();
    }
  });

  // A best of zero is a lifter who HAS done the lift: bodyweight work has a
  // legitimate zero, and reading the number instead of its presence would make
  // every chin-up session a first time forever.
  it("treats a standing best of zero as a history", async () => {
    getSession.mockResolvedValue(
      ok(oneTap({ previousBests: [{ exerciseId: 1, weightLb: 0, e1rmLb: 0 }] })),
    );
    updateSessionSet.mockResolvedValue(
      ok(mkSet({ id: 1, setNumber: 1, targetReps: 1, actualReps: 1, completed: true })),
    );

    render(ActiveSession, props);
    await tapTheSet();

    expect(await screen.findByText(/New PR!/)).toBeInTheDocument();
    expect(screen.queryByText(/First time!/)).not.toBeInTheDocument();
  });
});
