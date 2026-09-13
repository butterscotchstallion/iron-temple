import { render, screen, waitFor } from "@testing-library/svelte";
import { afterAll, beforeEach, describe, expect, it, vi } from "vitest";
import SessionRecap from "./SessionRecap.svelte";
import type { Session, SessionRecap as Recap, SessionSet } from "../lib/api";
import { clearCache } from "../lib/cache.svelte";
import { handOffSession } from "../lib/recapHandoff";

// A render test for a route, which the suite otherwise leaves to Playwright —
// earning its place for the same reason Racked.test.ts does, and more so.
//
// The recap branches on nullable fields at every turn (a first workout has no
// pace, no previous session and no lift with a delta), and it has a second
// source entirely: the local recap it falls back to when the server cannot be
// reached. That fallback is the one path that MUST work in a basement, and it
// is the one Playwright is least able to exercise here.

const getSessionRecap = vi.hoisted(() => vi.fn());
vi.mock("../lib/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../lib/api")>()),
  getSessionRecap,
}));

// canvas-confetti draws to a 2D context, and jsdom has none — it throws inside
// a requestAnimationFrame callback, which lands as an unhandled error rather
// than a test failure and can fail an unrelated file later in the run. Stubbed
// rather than guarded in the source: the physics works in a browser, and this
// file is about what the recap says, not about the confetti.
const celebrate = vi.hoisted(() => vi.fn());
vi.mock("../lib/celebrate", () => ({ celebrate }));

// The recap re-fetches when the write queue drains. Captured rather than
// stubbed away, so the test can fire the same signal the queue does.
const { onDrained, drain } = vi.hoisted(() => {
  let handler: (() => void) | null = null;
  return {
    onDrained: vi.fn((h: (() => void) | null) => {
      handler = h;
    }),
    drain: () => handler?.(),
  };
});
vi.mock("../lib/writeQueue.svelte", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../lib/writeQueue.svelte")>()),
  onDrained,
}));

const props = { params: { id: "42" } };

/** A recap with every optional field absent — a lifter's first workout. */
function emptyRecap(): Recap {
  return {
    session: {
      sessionId: 42,
      programId: 1,
      programName: "StrongLifts 5x5",
      programDayId: 7,
      programDayName: "Workout A",
      performedOn: "2026-09-13",
      startedAt: "2026-09-13T18:00:00Z",
      finishedAt: null,
      isOver: false,
    },
    durationSeconds: null,
    pace: null,
    volume: {
      totalLb: 0,
      previousLb: null,
      deltaPct: null,
      comparison: { count: 0, label: "", unitLb: 0 },
      setsLogged: 0,
      setsPrescribed: 5,
      repsLogged: 0,
      repsTargeted: 25,
    },
    progress: {
      previousSessionId: null,
      previousPerformedOn: null,
      weightDeltaPct: null,
      liftsCompared: 0,
      liftsNew: 0,
    },
    muscles: [],
    split: {
      main: { volumeLb: 0, sets: 0, reps: 0, lifts: 0, share: 0 },
      assistance: { volumeLb: 0, sets: 0, reps: 0, lifts: 0, share: 0 },
    },
    bodyweightLb: null,
    earned: null,
    lifts: [],
    prs: [],
    milestones: [],
    streak: { sessions: 0, weeks: 0 },
  };
}

function fullRecap(): Recap {
  return {
    ...emptyRecap(),
    session: { ...emptyRecap().session, finishedAt: "2026-09-13T18:52:00Z", isOver: true },
    durationSeconds: 3120,
    pace: { medianSeconds: 3600, deltaPct: -0.133, rank: 1, of: 12, sampleSize: 11 },
    volume: {
      totalLb: 18240,
      previousLb: 17500,
      deltaPct: 0.042,
      comparison: { count: 3, label: "pickup trucks", unitLb: 5000 },
      setsLogged: 25,
      setsPrescribed: 25,
      repsLogged: 125,
      repsTargeted: 125,
    },
    progress: {
      previousSessionId: 40,
      previousPerformedOn: "2026-09-06",
      weightDeltaPct: 0.021,
      liftsCompared: 3,
      liftsNew: 1,
    },
    lifts: [
      {
        exerciseId: 1,
        exerciseName: "Squat",
        kind: "main",
        topWeightLb: 245,
        topReps: 5,
        topE1rmLb: 286,
        setsLogged: 5,
        setsPrescribed: 5,
        repsLogged: 25,
        repsTargeted: 25,
        volumeLb: 6125,
        hitEveryTarget: true,
        previous: {
          performedOn: "2026-09-06",
          topWeightLb: 240,
          topReps: 5,
          topE1rmLb: 280,
        },
        weightDeltaLb: 5,
        weightDeltaPct: 0.021,
        e1rmDeltaPct: 0.021,
      },
    ],
    prs: [
      {
        kind: "weight",
        performedOn: "2026-09-13",
        exerciseId: 1,
        exerciseName: "Squat",
        weightLb: 245,
        reps: 5,
        valueLb: 245,
        previousLb: 240,
      },
    ],
    milestones: [
      {
        kind: "plate",
        performedOn: "2026-09-13",
        label: "First 245 lb Squat",
        valueLb: 245,
        exerciseId: 1,
        exerciseName: "Squat",
      },
    ],
    streak: { sessions: 6, weeks: 3 },
  };
}

function mkSet(over: Partial<SessionSet> & Pick<SessionSet, "id">): SessionSet {
  return {
    exerciseId: 1,
    exerciseName: "Squat",
    kind: "main",
    setNumber: 1,
    targetReps: 5,
    actualReps: 5,
    weightLb: 200,
    completed: true,
    restSeconds: 180,
    ...over,
  };
}

function mkSession(): Session {
  return {
    id: 42,
    programId: 1,
    programName: "StrongLifts 5x5",
    programDayId: 7,
    programDayName: "Workout A",
    performedOn: "2026-09-13",
    notes: "",
    createdAt: "2026-09-13T18:00:00Z",
    finishedAt: "2026-09-13T18:45:00Z",
    isOver: true,
    previousBests: [{ exerciseId: 1, weightLb: 195, e1rmLb: 228 }],
    sets: [1, 2, 3, 4, 5].map((n) => mkSet({ id: n, setNumber: n })),
  };
}

const unreachable = { status: 0, data: undefined, headers: new Headers() };

beforeEach(() => {
  getSessionRecap.mockReset();
  celebrate.mockReset();
  // The recap is cached per session across mounts, which is the point of it —
  // but that cache is module state shared by every test in this file.
  clearCache();
});

// bits-ui's scroll lock resets document styles on a timer at unmount, which can
// fire after jsdom has torn down and fail the whole run. Deliberately afterAll.
afterAll(async () => {
  await new Promise((resolve) => setTimeout(resolve, 100));
});

describe("SessionRecap", () => {
  it("renders the headline figures", async () => {
    getSessionRecap.mockResolvedValue({ status: 200, data: fullRecap(), headers: new Headers() });
    render(SessionRecap, props);

    await waitFor(() => expect(screen.getByText("52m")).toBeInTheDocument());
    expect(screen.getByText("18,240")).toBeInTheDocument();
    expect(screen.getByText("25 / 25")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: /Workout complete/ })).toBeInTheDocument();
  });

  it("restates the tonnage as something to picture", async () => {
    getSessionRecap.mockResolvedValue({ status: 200, data: fullRecap(), headers: new Headers() });
    render(SessionRecap, props);

    await waitFor(() =>
      expect(screen.getByTestId("recap-comparison")).toHaveTextContent(
        "about 3 pickup trucks",
      ),
    );
  });

  // A quicker session is a NEGATIVE deltaPct, and must never read as a loss.
  it("reads a faster session as faster", async () => {
    getSessionRecap.mockResolvedValue({ status: 200, data: fullRecap(), headers: new Headers() });
    render(SessionRecap, props);

    await waitFor(() => expect(screen.getByTestId("stat-pace")).toBeInTheDocument());
    expect(screen.getByTestId("stat-pace")).toHaveTextContent("13% faster");
    expect(screen.getByTestId("stat-pace")).toHaveTextContent("1st fastest Workout A of 12");
  });

  it("shows the weight progression and what it was drawn from", async () => {
    getSessionRecap.mockResolvedValue({ status: 200, data: fullRecap(), headers: new Headers() });
    render(SessionRecap, props);

    await waitFor(() => expect(screen.getByTestId("stat-progress")).toBeInTheDocument());
    const card = screen.getByTestId("stat-progress");
    expect(card).toHaveTextContent("+2%");
    expect(card).toHaveTextContent("across 3 lifts");
    expect(card).toHaveTextContent("1 new");
  });

  it("lists records, milestones and the streak", async () => {
    getSessionRecap.mockResolvedValue({ status: 200, data: fullRecap(), headers: new Headers() });
    render(SessionRecap, props);

    await waitFor(() => expect(screen.getByTestId("recap-highlights")).toBeInTheDocument());
    const box = screen.getByTestId("recap-highlights");
    expect(box).toHaveTextContent("Squat");
    expect(box).toHaveTextContent("First 245 lb Squat");
    expect(box).toHaveTextContent("6 sessions in a row");
  });

  // Every nullable field null is a lifter's first workout, and every one of
  // them is a chance to read a property off null.
  it("renders a first workout without any comparison", async () => {
    getSessionRecap.mockResolvedValue({ status: 200, data: emptyRecap(), headers: new Headers() });
    render(SessionRecap, props);

    await waitFor(() =>
      expect(screen.getByRole("heading", { name: /Workout finished/ })).toBeInTheDocument(),
    );
    expect(screen.queryByTestId("stat-pace")).not.toBeInTheDocument();
    expect(screen.queryByTestId("stat-progress")).not.toBeInTheDocument();
    expect(screen.queryByTestId("recap-highlights")).not.toBeInTheDocument();
    expect(screen.queryByTestId("recap-lifts")).not.toBeInTheDocument();
    expect(screen.queryByTestId("recap-comparison")).not.toBeInTheDocument();
    // The tiles are still there, saying what there is to say.
    expect(screen.getByTestId("stat-duration")).toHaveTextContent("—");
    expect(screen.getByTestId("stat-sets")).toHaveTextContent("0 / 5");
  });

  describe("with no signal", () => {
    // The basement case: the Finish was queued, the GET cannot land, and the
    // lifter still gets a recap built from the session handed across.
    it("falls back to the handed-off session", async () => {
      getSessionRecap.mockResolvedValue(unreachable);
      handOffSession(mkSession());
      render(SessionRecap, props);

      await waitFor(() => expect(screen.getByTestId("stat-volume")).toBeInTheDocument());
      expect(screen.getByTestId("stat-volume")).toHaveTextContent("5,000");
      expect(screen.getByTestId("stat-duration")).toHaveTextContent("45m");
      expect(screen.getByTestId("stat-sets")).toHaveTextContent("5 / 5");

      // Records are derivable from previousBests, so they survive offline.
      expect(screen.getByTestId("recap-highlights")).toHaveTextContent("Squat");
      // Nothing that needs history is invented.
      expect(screen.queryByTestId("stat-pace")).not.toBeInTheDocument();
      expect(screen.queryByTestId("stat-progress")).not.toBeInTheDocument();
    });

    // The celebration belongs here rather than in finish(), so it lands over
    // the recap instead of behind the screen being navigated away from.
    it("fires the confetti for a workout finished to the letter", async () => {
      getSessionRecap.mockResolvedValue(unreachable);
      handOffSession(mkSession());
      render(SessionRecap, props);

      await waitFor(() => expect(celebrate).toHaveBeenCalled());
    });

    it("does not celebrate a workout that fell short", async () => {
      getSessionRecap.mockResolvedValue(unreachable);
      const short = mkSession();
      short.sets[4] = mkSet({ id: 5, setNumber: 5, actualReps: 3, completed: false });
      handOffSession(short);
      render(SessionRecap, props);

      await waitFor(() => expect(screen.getByTestId("stat-volume")).toBeInTheDocument());
      expect(celebrate).not.toHaveBeenCalled();
    });

    it("says what is missing rather than showing an error", async () => {
      getSessionRecap.mockResolvedValue(unreachable);
      handOffSession(mkSession());
      render(SessionRecap, props);

      await waitFor(() => expect(screen.getByTestId("recap-degraded")).toBeInTheDocument());
      expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    });

    // A cold open from history with nothing cached and nothing handed across is
    // the one case with genuinely nothing to show.
    it("shows an error card when there is nothing at all", async () => {
      getSessionRecap.mockResolvedValue(unreachable);
      render(SessionRecap, props);

      await waitFor(() =>
        expect(screen.getByText("Couldn't load the recap.")).toBeInTheDocument(),
      );
      expect(screen.queryByTestId("stat-volume")).not.toBeInTheDocument();
    });
  });

  // The degraded state promises the rest will fill in on signal. Without this
  // it never looks again — and the recap is reached by finishing, which is the
  // write most likely to have been queued in the first place.
  it("asks again once the write queue drains", async () => {
    getSessionRecap.mockResolvedValue(unreachable);
    handOffSession(mkSession());
    render(SessionRecap, props);

    await waitFor(() => expect(screen.getByTestId("recap-degraded")).toBeInTheDocument());

    // Back on signal: the queue drains, and the server's answer replaces the
    // reconstruction without the lifter touching anything.
    getSessionRecap.mockResolvedValue({ status: 200, data: fullRecap(), headers: new Headers() });
    drain();

    await waitFor(() => expect(screen.getByTestId("stat-volume")).toHaveTextContent("18,240"));
    expect(screen.queryByTestId("recap-degraded")).not.toBeInTheDocument();
    expect(screen.getByTestId("stat-pace")).toBeInTheDocument();
  });

  // A handed-off session is taken once, so it cannot decorate a recap it does
  // not belong to.
  it("ignores a handed-off session for a different workout", async () => {
    getSessionRecap.mockResolvedValue(unreachable);
    handOffSession({ ...mkSession(), id: 99 });
    render(SessionRecap, props);

    await waitFor(() =>
      expect(screen.getByText("Couldn't load the recap.")).toBeInTheDocument(),
    );
  });

  // The server's answer is the better one, and replaces the local reconstruction
  // as soon as it lands.
  it("replaces the local recap when the server answers", async () => {
    getSessionRecap.mockResolvedValue({ status: 200, data: fullRecap(), headers: new Headers() });
    handOffSession(mkSession());
    render(SessionRecap, props);

    await waitFor(() => expect(screen.getByTestId("stat-volume")).toHaveTextContent("18,240"));
    expect(screen.queryByTestId("recap-degraded")).not.toBeInTheDocument();
    expect(screen.getByTestId("stat-pace")).toBeInTheDocument();
  });

  it("divides the session by muscle group, and names the split when there is one", async () => {
    getSessionRecap.mockResolvedValue({
      status: 200,
      data: {
        ...fullRecap(),
        muscles: [
          { group: "legs", volumeLb: 6125, sets: 5, reps: 25, lifts: 1, share: 0.7, trained: true },
          { group: "back", volumeLb: 2625, sets: 5, reps: 25, lifts: 1, share: 0.3, trained: true },
        ],
        split: {
          main: { volumeLb: 7000, sets: 8, reps: 40, lifts: 2, share: 0.8 },
          assistance: { volumeLb: 1750, sets: 2, reps: 10, lifts: 1, share: 0.2 },
        },
      },
      headers: new Headers(),
    });
    render(SessionRecap, props);

    await waitFor(() => expect(screen.getByTestId("recap-muscles")).toBeInTheDocument());
    expect(screen.getByTestId("recap-muscles")).toHaveTextContent("80% prescribed");
    expect(screen.getByTestId("recap-muscles")).toHaveTextContent("20% assistance");
  });

  // On a session that was purely the program, "100% prescribed" tells the
  // lifter what they already know.
  it("stays quiet about the split when nothing was bolted on", async () => {
    getSessionRecap.mockResolvedValue({
      status: 200,
      data: {
        ...fullRecap(),
        muscles: [
          { group: "legs", volumeLb: 6125, sets: 5, reps: 25, lifts: 1, share: 1, trained: true },
        ],
      },
      headers: new Headers(),
    });
    render(SessionRecap, props);

    await waitFor(() => expect(screen.getByTestId("recap-muscles")).toBeInTheDocument());
    expect(screen.getByTestId("recap-muscles")).not.toHaveTextContent("prescribed");
  });

  describe("what it earned", () => {
    const earned = {
      programDayId: 7,
      programDayName: "Workout A",
      exercises: [
        {
          exerciseId: 1,
          exerciseName: "Squat",
          kind: "main" as const,
          sets: 5,
          reps: 5,
          weightLb: 250,
          restSeconds: 180,
          setPlan: [],
          progression: {
            status: "advance",
            failureCount: 0,
            failuresBeforeDeload: 3,
            incrementLb: 5,
          },
        },
      ],
    };

    it("shows the next prescription and the engine's verdict", async () => {
      getSessionRecap.mockResolvedValue({
        status: 200,
        data: { ...fullRecap(), earned },
        headers: new Headers(),
      });
      render(SessionRecap, props);

      await waitFor(() => expect(screen.getByTestId("recap-earned")).toBeInTheDocument());
      const card = screen.getByTestId("recap-earned");
      expect(card).toHaveTextContent("Next Workout A");
      expect(card).toHaveTextContent("250 lb");
      expect(card).toHaveTextContent("up");
    });

    // A stall is worth seeing coming, so the count comes with it.
    it("counts a hold towards the deload it is heading for", async () => {
      const holding = {
        ...earned,
        exercises: [
          {
            ...earned.exercises[0],
            weightLb: 245,
            progression: {
              status: "hold",
              failureCount: 2,
              failuresBeforeDeload: 3,
              incrementLb: 5,
            },
          },
        ],
      };
      getSessionRecap.mockResolvedValue({
        status: 200,
        data: { ...fullRecap(), earned: holding },
        headers: new Headers(),
      });
      render(SessionRecap, props);

      await waitFor(() =>
        expect(screen.getByTestId("recap-earned")).toHaveTextContent("held · 2/3 to deload"),
      );
    });

    // Withheld on an older session, since the prescription describes today.
    it("is absent when the server withholds it", async () => {
      getSessionRecap.mockResolvedValue({
        status: 200,
        data: fullRecap(),
        headers: new Headers(),
      });
      render(SessionRecap, props);

      await waitFor(() => expect(screen.getByTestId("recap-lifts")).toBeInTheDocument());
      expect(screen.queryByTestId("recap-earned")).not.toBeInTheDocument();
    });
  });

  // The two streaks measure different things, but stacked they read as two ways
  // of flattering one fact — so the weeks only speak when the sessions cannot.
  it("falls back to the week streak when the session run is broken", async () => {
    getSessionRecap.mockResolvedValue({
      status: 200,
      data: { ...fullRecap(), streak: { sessions: 1, weeks: 8 } },
      headers: new Headers(),
    });
    render(SessionRecap, props);

    await waitFor(() => expect(screen.getByTestId("recap-highlights")).toBeInTheDocument());
    const box = screen.getByTestId("recap-highlights");
    expect(box).toHaveTextContent("8 weeks trained in a row");
    expect(box).not.toHaveTextContent("sessions in a row");
  });

  it("prefers the session streak when there is one", async () => {
    getSessionRecap.mockResolvedValue({ status: 200, data: fullRecap(), headers: new Headers() });
    render(SessionRecap, props);

    await waitFor(() => expect(screen.getByTestId("recap-highlights")).toBeInTheDocument());
    const box = screen.getByTestId("recap-highlights");
    expect(box).toHaveTextContent("6 sessions in a row");
    expect(box).not.toHaveTextContent("weeks trained in a row");
  });

  it("reports what the lifter weighed, when they said", async () => {
    getSessionRecap.mockResolvedValue({
      status: 200,
      data: { ...fullRecap(), bodyweightLb: 184.5 },
      headers: new Headers(),
    });
    render(SessionRecap, props);

    await waitFor(() => expect(screen.getByText(/You weighed 184.5 lb/)).toBeInTheDocument());
  });

  it("lists every lift with its own movement", async () => {
    getSessionRecap.mockResolvedValue({ status: 200, data: fullRecap(), headers: new Headers() });
    render(SessionRecap, props);

    await waitFor(() => expect(screen.getByTestId("recap-lifts")).toBeInTheDocument());
    const table = screen.getByTestId("recap-lifts");
    expect(table).toHaveTextContent("Squat");
    expect(table).toHaveTextContent("245 lb × 5");
    expect(table).toHaveTextContent("+2% from 240");
  });
});
