import { describe, it, expect } from "vitest";
import { programSubtitle, programAttribution } from "./programs";

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
