import { describe, expect, it } from "vitest";
import type { SessionRecap } from "./api";
import { sessionShareCardContent } from "./sessionShareCard";
import { LIFT_ROWS, shareCardLayout } from "./shareCard";

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
      setsBonus: 0,
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

function mkPR(exerciseName: string, valueLb: number) {
  return {
    kind: "weight" as const,
    performedOn: "2026-09-13",
    exerciseId: exerciseName.length,
    exerciseName,
    weightLb: valueLb,
    reps: 5,
    valueLb,
    previousLb: valueLb - 5,
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
    setsBonus: 0,
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
    // The date is formatted, not raw. This eyebrow was the one place in the app
    // that put a bare "2026-09-13" in front of a reader, and a share card is a
    // picture — there is no hover here to recover a readable date from.
    expect(c.eyebrow).toBe("WORKOUT A · SEPTEMBER 13 2026");
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

    // A row each, with what was lifted. The block used to be one row reading
    // "Personal records (3)" beside a single lift's name, which looked like a
    // card that had failed to render the other two — and naming them without
    // their weights still says a good day happened without saying what it was.
    it("gives every record a row of its own, with the weight on it", () => {
      const c = sessionShareCardContent(
        mkRecap({
          prs: [mkPR("Bench Press", 160), mkPR("Squat", 245), mkPR("Barbell Row", 135)],
        }),
      );
      // Heaviest first: the server returns them alphabetically, which is right
      // for a list and arbitrary for a headline. mkPR beats the old mark by 5,
      // so each row also says how far it moved it.
      expect(c.moments).toEqual([
        { label: "PR · Squat", value: "245 lb × 5 · +2%" },
        { label: "PR · Bench Press", value: "160 lb × 5 · +3%" },
        { label: "PR · Barbell Row", value: "135 lb × 5 · +4%" },
      ]);
    });

    // previousLb is 0 for a lift with no history — "nothing to beat" rather
    // than a mark of zero pounds — so there is no percentage to report.
    it("omits the gain on a lift with no history", () => {
      const c = sessionShareCardContent(
        mkRecap({ prs: [{ ...mkPR("Squat", 245), previousLb: 0 }] }),
      );
      expect(c.moments[0]).toEqual({ label: "PR · Squat", value: "245 lb × 5" });
    });

    // The bar did not move, so quoting it would make the row look like a record
    // that isn't one. The estimate is what was beaten, and it says so.
    it("marks an estimated-max record apart from a heavier bar", () => {
      const c = sessionShareCardContent(
        mkRecap({ prs: [{ ...mkPR("Squat", 286), kind: "e1rm", weightLb: 245, reps: 8 }] }),
      );
      expect(c.moments[0]).toEqual({ label: "Est. max · Squat", value: "286 lb · +2%" });
    });

    // Rows are finite, so the overflow is counted rather than silently dropped —
    // and what overflows is the lightest.
    it("counts the records past the rows it has", () => {
      const c = sessionShareCardContent(
        mkRecap({
          prs: [
            mkPR("Curl", 40),
            mkPR("Squat", 245),
            mkPR("Bench Press", 160),
            mkPR("Barbell Row", 135),
            mkPR("Overhead Press", 95),
          ],
        }),
      );
      expect(c.moments).toEqual([
        { label: "PR · Squat", value: "245 lb × 5 · +2%" },
        { label: "PR · Bench Press", value: "160 lb × 5 · +3%" },
        { label: "PR · Barbell Row", value: "135 lb × 5 · +4%" },
        { label: "More records", value: "+2" },
      ]);
    });

    // Exactly four fit, so nothing is given up to a count of nothing.
    it("spends no row counting when they all fit", () => {
      const prs = Array.from({ length: 4 }, (_, i) => mkPR(`Lift ${i}`, 300 - i));
      const c = sessionShareCardContent(mkRecap({ prs }));
      expect(c.moments).toHaveLength(4);
      expect(c.moments.every((m) => m.label.startsWith("PR · "))).toBe(true);
    });

    it("has nothing to say about a quiet session", () => {
      expect(sessionShareCardContent(mkRecap()).moments).toEqual([]);
    });
  });

  // The records got their own rows by spending vertical space the card has a
  // fixed amount of. shareCardLayout packs blocks into the region above the
  // footer, and a card that outgrows it runs long rather than cropping — so the
  // bound is asserted rather than reasoned about.
  //
  // Pure arithmetic over the metrics, which is why this can run in jsdom at all:
  // no canvas is touched.
  describe("the layout it produces", () => {
    /** The fullest a session card gets: every optional line, every row taken. */
    function fullestRecap() {
      return mkRecap({
        // Both optional header lines present.
        volume: { ...mkRecap().volume, comparison: { count: 3, label: "pickup trucks", unitLb: 5000 } },
        progress: {
          previousSessionId: 40,
          previousPerformedOn: "2026-09-06",
          weightDeltaPct: 0.021,
          liftsCompared: 5,
          liftsNew: 0,
        },
        pace: { medianSeconds: 3600, deltaPct: 0.12, rank: 4, of: 12, sampleSize: 11 },
        lifts: Array.from({ length: 8 }, (_, i) => mkLift(`Exercise Number ${i}`, 900 - i, 200)),
        prs: Array.from({ length: 9 }, (_, i) => mkPR(`Exercise Number ${i}`, 300 - i)),
        milestones: [
          {
            kind: "plate" as const,
            performedOn: "2026-09-13",
            label: "First 245 lb Squat",
            valueLb: 245,
            exerciseId: 1,
            exerciseName: "Squat",
          },
        ],
      });
    }

    it("fits above the footer at its fullest", () => {
      const content = sessionShareCardContent(fullestRecap(), "Ada");
      const { blocks, contentBottom } = shareCardLayout(content);

      const last = blocks[blocks.length - 1];
      expect(last.y + last.height).toBeLessThanOrEqual(contentBottom);
      // And it starts on the card rather than above it.
      expect(blocks[0].y).toBeGreaterThanOrEqual(0);
    });

    it("still fits when every moment row is a record", () => {
      // No pace and no milestone, so the records take the whole budget.
      const content = sessionShareCardContent(
        mkRecap({
          ...fullestRecap(),
          pace: null,
          milestones: [],
        }),
        "Ada",
      );
      const { blocks, contentBottom } = shareCardLayout(content);
      const last = blocks[blocks.length - 1];
      expect(last.y + last.height).toBeLessThanOrEqual(contentBottom);
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
