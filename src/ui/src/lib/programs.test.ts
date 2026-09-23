import { describe, it, expect } from "vitest";
import { programSubtitle, programAttribution, cloneSources } from "./programs";
import { testProgramSummary } from "./testFixtures";

describe("programSubtitle", () => {
  it("uses the description when present", () => {
    expect(programSubtitle({ description: "Squat, bench, row · A/B · 5×5" })).toBe(
      "Squat, bench, row · A/B · 5×5",
    );
  });

  it("trims surrounding whitespace", () => {
    expect(programSubtitle({ description: "  Reduced volume  " })).toBe("Reduced volume");
  });

  it("falls back when the description is empty or blank", () => {
    expect(programSubtitle({ description: "" })).toBe("Linear progression");
    expect(programSubtitle({ description: "   " })).toBe("Linear progression");
  });
});

describe("programAttribution", () => {
  it("says nothing about a seeded program", () => {
    // The install's own catalogue has no author to credit, and eight of the nine
    // cards a fresh install draws are these.
    expect(
      programAttribution({ ownerId: null, ownerName: "", isMine: false }),
    ).toBe("");
  });

  it("says nothing about your own program", () => {
    // The "Yours" badge on the card has already said it.
    expect(
      programAttribution({ ownerId: 1, ownerName: "Ada Lovelace", isMine: true }),
    ).toBe("");
  });

  it("names the owner of somebody else's shared program", () => {
    // The case the line exists for: per-owner name uniqueness lets two lifters
    // each have a "Push Pull Legs", so the owner is what tells the cards apart.
    expect(
      programAttribution({ ownerId: 2, ownerName: "Grace Hopper", isMine: false }),
    ).toBe("Shared by Grace Hopper");
  });

  it("falls back rather than crediting an empty name", () => {
    // Otherwise an account with a blank display name renders "Shared by " with
    // nothing after it.
    expect(
      programAttribution({ ownerId: 2, ownerName: "   ", isMine: false }),
    ).toBe("Shared by another lifter");
  });
});

describe("cloneSources", () => {
  it("offers a seeded linear program", () => {
    const strongLifts = testProgramSummary({ id: 1, name: "StrongLifts 5x5" });
    expect(cloneSources([strongLifts])).toEqual([strongLifts]);
  });

  it("leaves out a ramping program", () => {
    // Madcow prescribes percentages of a top set, which the editor cannot
    // express — the API refuses it, and offering it would turn a deliberate
    // refusal into what looks like a bug.
    const madcow = testProgramSummary({
      id: 6,
      name: "Madcow 5x5",
      progressionKind: "madcow",
    });
    expect(cloneSources([madcow])).toEqual([]);
  });

  it("leaves out an archived program", () => {
    // Retiring a program is its owner saying "stop offering this".
    const retired = testProgramSummary({
      id: 9,
      ownerId: 1,
      isMine: true,
      archivedAt: "2026-09-01T00:00:00Z",
    });
    expect(cloneSources([retired])).toEqual([]);
  });

  it("offers another lifter's shared program", () => {
    // Taking a copy and making it yours is most of the point of sharing one,
    // and it costs the owner nothing: a clone copies rows, it does not move
    // them.
    const theirs = testProgramSummary({
      id: 4,
      name: "Grace's Block",
      ownerId: 2,
      ownerName: "Grace Hopper",
      isMine: false,
      isShared: true,
    });
    expect(cloneSources([theirs])).toEqual([theirs]);
  });
});
