import { describe, expect, it } from "vitest";
import type { SessionRecap } from "./api";
import { sessionShareCardContent } from "./sessionShareCard";
import { LIFT_ROWS } from "./shareCard";

function mkRecap(over: Partial<SessionRecap> = {}): SessionRecap {
  return {
    session: {
      sessionId: 42,
      programId: 1,
      programName: "StrongLifts 5x5",
      programDayId: 7,
      programDayName: "Workout A",
      performedOn: "2026-09-13",
      startedAt: "2026-09-13T18:00:00Z",
      finishedAt: "2026-09-13T18:52:00Z",
      isOver: true,
    },
    durationSeconds: 3120,
    pace: null,
    volume: {
      totalLb: 18240,
      previousLb: null,
      deltaPct: null,
      comparison: { count: 3, label: "pickup trucks", unitLb: 5000 },
      setsLogged: 25,
      setsPrescribed: 25,
      repsLogged: 125,
      repsTargeted: 125,
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
    streak: { sessions: 4, weeks: 2 },
    ...over,
  };
}

function mkLift(name: string, volumeLb: number, topWeightLb: number) {
  return {
    exerciseId: name.length,
    exerciseName: name,
    kind: "main" as const,
    topWeightLb,
    topReps: 5,
    topE1rmLb: topWeightLb + 40,
    setsLogged: 5,
    setsPrescribed: 5,
    repsLogged: 25,
    repsTargeted: 25,
    volumeLb,
    hitEveryTarget: true,
    previous: null,
    weightDeltaLb: null,
    weightDeltaPct: null,
    e1rmDeltaPct: null,
  };
}

describe("sessionShareCardContent", () => {
  it("leads with the day, the date and the tonnage", () => {
    const c = sessionShareCardContent(mkRecap(), "Ada");
    expect(c.eyebrow).toBe("WORKOUT A · 2026-09-13");
    expect(c.lede).toBe("Ada lifted");
    expect(c.headline).toBe("18,240 LB");
    expect(c.comparison).toBe("That's 3 pickup trucks.");
    expect(c.footnote).toBe("StrongLifts 5x5 · Workout A");
  });

  it("addresses a lifter with no display name directly", () => {
    expect(sessionShareCardContent(mkRecap()).lede).toBe("You lifted");
  });

  // A session is not a month, and there is no archetype to draw from one. The
  // layout gives an absent panel no space at all.
  it("carries no archetype", () => {
    expect(sessionShareCardContent(mkRecap()).archetype).toBeNull();
  });

  describe("the change line", () => {
    it("names the day it is measured against", () => {
      const c = sessionShareCardContent(
        mkRecap({
          progress: {
            previousSessionId: 40,
            previousPerformedOn: "2026-09-06",
            weightDeltaPct: 0.021,
            liftsCompared: 3,
            liftsNew: 0,
          },
        }),
      );
      expect(c.change).toBe("+2% on your last Workout A");
    });

    // "+0%" on a first workout would be a claim rather than an absence.
    it("is absent when nothing was compared", () => {
      expect(sessionShareCardContent(mkRecap()).change).toBeNull();
    });

    it("is absent when a previous session existed but no lift lined up", () => {
      const c = sessionShareCardContent(
        mkRecap({
          progress: {
            previousSessionId: 40,
            previousPerformedOn: "2026-09-06",
            weightDeltaPct: null,
            liftsCompared: 0,
            liftsNew: 3,
          },
        }),
      );
      expect(c.change).toBeNull();
    });
  });

  describe("lifts", () => {
    it("takes the heaviest first, not the prescribed order", () => {
      const c = sessionShareCardContent(
        mkRecap({
          lifts: [
            mkLift("Curl", 900, 30),
            mkLift("Squat", 6125, 245),
            mkLift("Bench Press", 4000, 160),
          ],
        }),
      );
      expect(c.lifts.map((l) => l.name)).toEqual(["Squat", "Bench Press", "Curl"]);
      expect(c.lifts[0].value).toBe("245 lb × 5");
      expect(c.lifts[0].fraction).toBe(1);
    });

    it("fits the card", () => {
      const many = Array.from({ length: 9 }, (_, i) => mkLift(`Lift ${i}`, 1000 - i, 100));
      expect(sessionShareCardContent(mkRecap({ lifts: many })).lifts).toHaveLength(LIFT_ROWS);
    });
  });

  describe("moments", () => {
    it("calls out a session that was the fastest yet", () => {
      const c = sessionShareCardContent(
        mkRecap({
          pace: { medianSeconds: 3600, deltaPct: -0.13, rank: 1, of: 12, sampleSize: 11 },
        }),
      );
      expect(c.moments[0]).toEqual({ label: "Fastest yet", value: "52m" });
    });

    it("places a session that was not", () => {
      const c = sessionShareCardContent(
        mkRecap({
          pace: { medianSeconds: 3000, deltaPct: 0.12, rank: 4, of: 12, sampleSize: 11 },
        }),
      );
      expect(c.moments[0]).toEqual({ label: "Pace", value: "12% slower · 4th of 12" });
    });

    it("names the record and counts the rest", () => {
      const pr = {
        kind: "weight" as const,
        performedOn: "2026-09-13",
        exerciseId: 1,
        exerciseName: "Squat",
        weightLb: 245,
        reps: 5,
        valueLb: 245,
        previousLb: 240,
      };
      const one = sessionShareCardContent(mkRecap({ prs: [pr] }));
      expect(one.moments[0]).toEqual({ label: "Personal record", value: "Squat 245 lb" });

      const two = sessionShareCardContent(
        mkRecap({ prs: [pr, { ...pr, exerciseId: 2, exerciseName: "Row" }] }),
      );
      expect(two.moments[0].label).toBe("Personal records (2)");
    });

    it("has nothing to say about a quiet session", () => {
      expect(sessionShareCardContent(mkRecap()).moments).toEqual([]);
    });
  });

  describe("tiles", () => {
    it("carries duration, sets, reps and the streak", () => {
      expect(sessionShareCardContent(mkRecap()).tiles).toEqual([
        { value: "52m", label: "duration" },
        { value: "25", label: "sets" },
        { value: "125", label: "reps" },
        { value: "4", label: "session streak" },
      ]);
    });

    it("shows a dash for a session that was never finished", () => {
      const c = sessionShareCardContent(mkRecap({ durationSeconds: null }));
      expect(c.tiles[0]).toEqual({ value: "—", label: "duration" });
    });
  });
});
