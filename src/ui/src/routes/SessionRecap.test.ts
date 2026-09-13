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
    previousBests: [{ exerciseId: 1, weightLb: 195 }],
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
