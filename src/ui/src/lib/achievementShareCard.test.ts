import { describe, expect, it } from "vitest";
import {
  achievementShareCardContent,
  achievementShareCardFilename,
  formatRemaining,
} from "./achievementShareCard";
import { SHARE_CARD, shareCardLayout } from "./shareCard";
import { testAchievement, testLifterAchievement, testUpcomingMilestone } from "./testFixtures";

// The crown's share card, tested everywhere except the pixels — the same division
// shareCard.test.ts draws, and for its reason: jsdom has no 2D context, so what the
// card SAYS and whether it fits inside itself are the two things that can go wrong
// and both are decided before anything is drawn.

describe("what the card says", () => {
  it("leads with the achievement and explains it", () => {
    const content = achievementShareCardContent(testLifterAchievement(), "Ada Lovelace");

    expect(content.headline).toBe("TOP OF WEEK STREAK");
    expect(content.lede).toBe("Ada Lovelace holds");
    expect(content.comparison).toBe(
      "Held the longest run of consecutive weeks trained.",
    );
  });

  // Second person when there is no display name, which is the state of an account
  // created through the admin area. Same fallback the other two cards make.
  it("addresses the sender when there is no name to use", () => {
    expect(achievementShareCardContent(testLifterAchievement()).lede).toBe("You hold");
  });

  it("carries the reign in its tiles", () => {
    const content = achievementShareCardContent(
      testLifterAchievement({ timesHeld: 3 }),
      "Ada",
    );

    expect(content.tiles).toHaveLength(2);
    expect(content.tiles[0]).toEqual({ value: "3", label: "times held" });
    expect(content.tiles[1].label).toBe("holding since");
  });

  // "1 times held" is the kind of thing that makes a card look automated.
  it("says time rather than times for a single reign", () => {
    const content = achievementShareCardContent(testLifterAchievement({ timesHeld: 1 }));
    expect(content.tiles[0].label).toBe("time held");
  });

  // The one way this card could be dishonest: past tense is required once the crown
  // has moved on, or it claims a standing the lifter no longer has.
  it("speaks in the past about a crown they no longer hold", () => {
    const content = achievementShareCardContent(testLifterAchievement({ heldNow: false }));
    expect(content.tiles[1].label).toBe("last held");
    expect(content.footnote).toBe("Held the top of this board.");
  });

  it("speaks in the present about one they still hold", () => {
    const content = achievementShareCardContent(testLifterAchievement());
    expect(content.footnote).toBe("Top of the board on this install.");
  });

  // No lift bars and no archetype: there is no breakdown of a crown, and an
  // archetype is a judgement about a month of training. The layout gives an absent
  // block no space, so this is what keeps the card from carrying two empty panels.
  it("asks for no lift bars and no archetype", () => {
    const content = achievementShareCardContent(testLifterAchievement());
    expect(content.lifts).toEqual([]);
    expect(content.archetype).toBeNull();
    expect(content.change).toBeNull();
  });
});

describe("the closing-in row", () => {
  it("names what is next and how far off it is", () => {
    const content = achievementShareCardContent(
      testLifterAchievement(),
      "Ada",
      testUpcomingMilestone(),
    );

    expect(content.moments).toHaveLength(1);
    expect(content.moments[0]).toEqual({
      label: "Closing in",
      value: "First 225 lb Squat · 20 lb to go",
    });
  });

  // A lifter past every rung gets a card without the row rather than one claiming a
  // goal that does not exist.
  it("is absent when there is nothing to chase", () => {
    expect(achievementShareCardContent(testLifterAchievement()).moments).toEqual([]);
  });

  it("groups a large remainder the way the rest of the app does", () => {
    expect(
      formatRemaining(testUpcomingMilestone({ targetLb: 1_000_000, currentLb: 820_000 })),
    ).toBe("180,000 lb to go");
  });

  // Rounding a sub-pound remainder to zero would print "0 lb to go" against a
  // target not yet reached, which reads as a bug rather than as nearly there.
  it("never says nothing is left on a target not yet reached", () => {
    expect(
      formatRemaining(testUpcomingMilestone({ targetLb: 225, currentLb: 224.8 })),
    ).toBe("1 lb to go");
  });
});

describe("the file", () => {
  it("is named after the achievement's slug", () => {
    expect(achievementShareCardFilename(testLifterAchievement())).toBe("crown-streak.png");
  });
});

// The property nobody can check by looking at the running app: an overflow shows up
// only on the one card that happens to fill every block it uses.
describe("shareCardLayout on an achievement card", () => {
  function fits(content: ReturnType<typeof achievementShareCardContent>) {
    const { blocks, contentBottom } = shareCardLayout(content);
    expect(blocks.length).toBeGreaterThan(0);
    expect(blocks[0].y).toBeGreaterThanOrEqual(SHARE_CARD.pad);
    for (let i = 1; i < blocks.length; i++) {
      const previousEnd = blocks[i - 1].y + blocks[i - 1].height;
      expect(blocks[i].y).toBeGreaterThan(previousEnd);
    }
    const last = blocks[blocks.length - 1];
    expect(last.y + last.height).toBeLessThanOrEqual(contentBottom);
  }

  it("fits the fullest card this selector can build", () => {
    fits(
      achievementShareCardContent(
        testLifterAchievement({
          timesHeld: 12,
          achievement: testAchievement({
            label: "Top of Sessions a week",
            description:
              "Trained more often than anyone else on the install this month, " +
              "measured over the whole period rather than over the weeks they showed up.",
          }),
        }),
        "Ada Lovelace",
        testUpcomingMilestone(),
      ),
    );
  });

  it("fits the sparsest one", () => {
    fits(achievementShareCardContent(testLifterAchievement()));
  });

  // The tiles block used to be reserved unconditionally, so an empty array booked
  // space and then had paintTiles divide the width by zero. No card here asks for
  // that, but the layer is shared and the next selector might.
  it("gives no space to a card with no tiles", () => {
    const withTiles = shareCardLayout(
      achievementShareCardContent(testLifterAchievement()),
    );
    const withoutTiles = shareCardLayout({
      ...achievementShareCardContent(testLifterAchievement()),
      tiles: [],
    });
    expect(withoutTiles.blocks.some((b) => b.kind === "tiles")).toBe(false);
    expect(withTiles.blocks.some((b) => b.kind === "tiles")).toBe(true);
  });
});
