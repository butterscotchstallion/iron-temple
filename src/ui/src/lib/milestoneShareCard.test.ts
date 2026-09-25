import { describe, expect, it } from "vitest";
import {
  milestoneShareCardContent,
  milestoneShareCardFilename,
} from "./milestoneShareCard";
import { testUpcomingMilestone } from "./testFixtures";
import type { RackedMilestone } from "./api";

function plate(over: Partial<RackedMilestone> = {}): RackedMilestone {
  return {
    kind: "plate",
    performedOn: "2026-03-14",
    label: "First 225 lb Squat",
    valueLb: 225,
    exerciseId: 1,
    exerciseName: "Squat",
    ...over,
  };
}

describe("milestoneShareCardContent", () => {
  it("leads with the rung and says why it is worth having", () => {
    const c = milestoneShareCardContent(plate(), "Ada");
    expect(c.eyebrow).toBe("IRON TEMPLE · MILESTONE");
    // Third person with a name — the sender is showing this to other people.
    expect(c.lede).toBe("Ada reached");
    expect(c.headline).toBe("FIRST 225 LB SQUAT");
    // Not the label again: a milestone has no catalogue description to lean on, so
    // this slot carries the argument instead.
    expect(c.comparison).toMatch(/only be a first once/);
    expect(c.tiles[0]).toEqual({ value: "225 lb", label: "on the bar" });
  });

  it("speaks in the second person without a name", () => {
    expect(milestoneShareCardContent(plate()).lede).toBe("You reached");
  });

  // A lifetime total is a different claim from a weight on a bar, and the card has
  // to stop saying "on the bar" about one.
  it("words a volume threshold as a lifetime", () => {
    const c = milestoneShareCardContent(
      plate({ kind: "volume", label: "100,000 lb lifted, all time", valueLb: 100_000 }),
    );
    expect(c.tiles[0].label).toBe("lifted, all time");
    expect(c.comparison).toMatch(/lifetime total/);
    expect(c.footnote).toMatch(/every logged set/);
  });

  // The thing that makes a rung land is that there is another one. Absent rather
  // than a row reading "nothing" when there is not.
  it("names the next rung when there is one", () => {
    const c = milestoneShareCardContent(plate(), "Ada", testUpcomingMilestone());
    expect(c.moments[0].label).toBe("Next up");
    expect(c.moments[0].value).toMatch(/First 225 lb Squat · 20 lb to go/);
  });

  it("carries no forward row when there is nothing left to chase", () => {
    expect(milestoneShareCardContent(plate(), "Ada", null).moments).toEqual([]);
  });

  // No lift bars and no archetype: there is no breakdown of a single rung, and a
  // judgement about a month of training is not what this card is about.
  it("carries none of the period card's blocks", () => {
    const c = milestoneShareCardContent(plate());
    expect(c.lifts).toEqual([]);
    expect(c.archetype).toBeNull();
    expect(c.change).toBeNull();
  });

  // A dumbbell rung's label is per hand while its figures are the pair, so the
  // "next up" row has to agree with the label beside it rather than with the raw
  // numbers — see formatRemaining.
  it("counts down a dumbbell rung in bells", () => {
    const c = milestoneShareCardContent(
      plate({ label: "First 45 lb per hand Dumbbell Bench Press", valueLb: 90 }),
      "Ada",
      testUpcomingMilestone({
        label: "First 50 lb per hand Dumbbell Bench Press",
        targetLb: 100,
        currentLb: 90,
        equipment: "dumbbell",
      }),
    );
    // 10 lb of pair is 5 lb a hand, which is one step on a rack.
    expect(c.moments[0].value).toMatch(/5 lb to go/);
  });
});

describe("milestoneShareCardFilename", () => {
  it("slugs the label", () => {
    expect(milestoneShareCardFilename("First 225 lb Squat")).toBe("first-225-lb-squat.png");
  });

  it("falls back rather than producing a bare extension", () => {
    expect(milestoneShareCardFilename("!!!")).toBe("milestone.png");
  });
});
