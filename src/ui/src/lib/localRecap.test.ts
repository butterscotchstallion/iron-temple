import { describe, expect, it } from "vitest";
import type { Session, SessionSet } from "./api";
import { localRecap } from "./localRecap";

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
    finishedAt: "2026-09-13T18:45:00Z",
    isOver: true,
    previousBests: [],
    sets: [1, 2, 3, 4, 5].map((n) => mkSet({ id: n, setNumber: n })),
    ...over,
  };
}

describe("localRecap", () => {
  it("totals what was logged", () => {
    const r = localRecap(mkSession());
    expect(r.volumeLb).toBe(5000); // 5 sets × 5 reps × 200 lb
    expect(r.setsLogged).toBe(5);
    expect(r.setsPrescribed).toBe(5);
    expect(r.repsLogged).toBe(25);
    expect(r.repsTargeted).toBe(25);
    expect(r.durationSeconds).toBe(45 * 60);
  });

  // Unlogged sets are the denominator of "10 of 25 reps": they count toward the
  // prescription and toward nothing else.
  it("counts unlogged sets as prescription only", () => {
    const r = localRecap(
      mkSession({
        sets: [
          mkSet({ id: 1, setNumber: 1 }),
          mkSet({ id: 2, setNumber: 2 }),
          mkSet({ id: 3, setNumber: 3, actualReps: null, completed: false }),
          mkSet({ id: 4, setNumber: 4, actualReps: null, completed: false }),
          mkSet({ id: 5, setNumber: 5, actualReps: null, completed: false }),
        ],
      }),
    );
    expect(r.setsLogged).toBe(2);
    expect(r.setsPrescribed).toBe(5);
    expect(r.repsLogged).toBe(10);
    expect(r.repsTargeted).toBe(25);
    expect(r.volumeLb).toBe(2000);
    expect(r.lifts[0].hitEveryTarget).toBe(false);
  });

  // A lift nobody performed is not a row of zeroes; it is absent.
  it("omits a lift with nothing logged against it", () => {
    const r = localRecap(
      mkSession({
        sets: [
          mkSet({ id: 1 }),
          mkSet({
            id: 2,
            exerciseId: 2,
            exerciseName: "Bench Press",
            actualReps: null,
            completed: false,
          }),
        ],
      }),
    );
    expect(r.lifts.map((l) => l.exerciseName)).toEqual(["Squat"]);
  });

  it("finds the top set and the best estimated max", () => {
    // 5×200 estimates 233; 1×245 is worth exactly 245 as a weight but only 245
    // as an estimate, so the heavier bar wins both here.
    const r = localRecap(
      mkSession({
        sets: [
          mkSet({ id: 1, actualReps: 5, weightLb: 200 }),
          mkSet({ id: 2, setNumber: 2, actualReps: 1, weightLb: 245 }),
        ],
      }),
    );
    expect(r.lifts[0].topWeightLb).toBe(245);
    expect(r.lifts[0].topReps).toBe(1);
    expect(r.lifts[0].topE1rmLb).toBe(245);
  });

  describe("records", () => {
    it("flags a set above the lift's standing best", () => {
      const r = localRecap(
        mkSession({ previousBests: [{ exerciseId: 1, weightLb: 195 }] }),
      );
      expect(r.prs).toEqual([
        { exerciseId: 1, exerciseName: "Squat", weightLb: 200, previousLb: 195 },
      ]);
    });

    it("does not flag a set that only matched the best", () => {
      const r = localRecap(
        mkSession({ previousBests: [{ exerciseId: 1, weightLb: 200 }] }),
      );
      expect(r.prs).toEqual([]);
    });

    // A lift with no history is absent from previousBests rather than zero, so
    // any completed set clears it — the same rule the session screen uses.
    it("treats an absent best as nothing to beat", () => {
      const r = localRecap(mkSession({ previousBests: [] }));
      expect(r.prs).toHaveLength(1);
      expect(r.prs[0].previousLb).toBe(0);
    });
  });

  describe("duration", () => {
    it("is null when the session was never finished", () => {
      expect(localRecap(mkSession({ finishedAt: null })).durationSeconds).toBeNull();
    });

    // Mirrors the 12-hour cap in internal/racked: a session left open
    // overnight measures the tab, not the training.
    it("is null past the twelve-hour cap", () => {
      const r = localRecap(
        mkSession({
          createdAt: "2026-09-13T06:00:00Z",
          finishedAt: "2026-09-13T19:00:00Z",
        }),
      );
      expect(r.durationSeconds).toBeNull();
    });

    it("is null when a stamp will not parse", () => {
      expect(localRecap(mkSession({ createdAt: "not a date" })).durationSeconds).toBeNull();
    });
  });

  it("carries the assistance flag through", () => {
    const r = localRecap(
      mkSession({
        sets: [mkSet({ id: 1, exerciseId: 9, exerciseName: "Curl", kind: "assistance" })],
      }),
    );
    expect(r.lifts[0].kind).toBe("assistance");
  });
});
