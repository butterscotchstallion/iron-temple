import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/svelte";
import AchievementList from "./AchievementList.svelte";
import {
  testAchievement,
  testLifterAchievement,
  testUpcomingMilestone,
} from "./testFixtures";

describe("the empty state", () => {
  // Second person to yourself and third person about anybody else. Getting this
  // backwards tells a reader that THEY have not won anything while they are
  // looking at somebody else's profile.
  it("invites you to win something on your own profile", () => {
    render(AchievementList, { props: { items: [], you: true } });
    expect(screen.getByText(/Lead any board/)).toBeInTheDocument();
  });

  it("names the lifter on somebody else's", () => {
    render(AchievementList, { props: { items: [], name: "Grace" } });
    expect(screen.getByText("Grace hasn't earned anything yet.")).toBeInTheDocument();
  });

  it("falls back to a neutral phrase when the name has not arrived", () => {
    render(AchievementList, { props: { items: [] } });
    expect(screen.getByText("This lifter hasn't earned anything yet.")).toBeInTheDocument();
  });
});

describe("an entry", () => {
  it("names the achievement and explains it", () => {
    render(AchievementList, { props: { items: [testLifterAchievement()] } });
    expect(screen.getByText("Top of Week streak")).toBeInTheDocument();
    expect(
      screen.getByText("Held the longest run of consecutive weeks trained."),
    ).toBeInTheDocument();
  });

  it("marks one they are holding right now", () => {
    render(AchievementList, { props: { items: [testLifterAchievement()] } });
    expect(screen.getByText("Holding")).toBeInTheDocument();
    expect(screen.getByText(/Holding it since/)).toBeInTheDocument();
  });

  // A lapsed reign is DIMMED, not dropped. Showing only what somebody holds right
  // now would empty the section for everybody the day after they were overtaken,
  // which defeats the point of keeping history at all.
  it("keeps one they used to hold, without the badge", () => {
    render(AchievementList, {
      props: { items: [testLifterAchievement({ heldNow: false })] },
    });
    expect(screen.getByText("Top of Week streak")).toBeInTheDocument();
    expect(screen.queryByText("Holding")).not.toBeInTheDocument();
    expect(screen.getByText(/Last held/)).toBeInTheDocument();
  });

  // "Held once" is what every first win reads as and adds nothing to a line that
  // already says when, so the count only appears past one.
  it("says nothing about the count of a single reign", () => {
    render(AchievementList, { props: { items: [testLifterAchievement()] } });
    expect(screen.queryByText(/held 1 time/)).not.toBeInTheDocument();
  });

  it("counts the reigns once there has been more than one", () => {
    render(AchievementList, {
      props: { items: [testLifterAchievement({ timesHeld: 3 })] },
    });
    expect(screen.getByText(/held 3 times/)).toBeInTheDocument();
  });

  it("lists every achievement it is given", () => {
    render(AchievementList, {
      props: {
        items: [
          testLifterAchievement(),
          testLifterAchievement({
            achievement: testAchievement({
              slug: "crown-volume",
              label: "Top of Volume",
              description: "Moved more total weight than anyone else.",
            }),
            heldNow: false,
          }),
        ],
      },
    });
    expect(screen.getByText("Top of Week streak")).toBeInTheDocument();
    expect(screen.getByText("Top of Volume")).toBeInTheDocument();
  });
});

// What is still ahead. Only ever drawn about yourself — what somebody else is
// closing in on is their business, so another lifter's profile passes none.
describe("closing in", () => {
  it("names what is next and how far off it is", () => {
    render(AchievementList, {
      props: { items: [], you: true, upcoming: [testUpcomingMilestone()] },
    });

    expect(screen.getByText("Closing in")).toBeInTheDocument();
    expect(screen.getByText("First 225 lb Squat")).toBeInTheDocument();
    expect(screen.getByText("20 lb to go")).toBeInTheDocument();
  });

  // RecapHighlights' rule: draw nothing rather than an empty congratulation. A
  // lifter past every rung, or one who has trained nothing, gets no heading.
  it("draws nothing at all when there is nothing to chase", () => {
    render(AchievementList, { props: { items: [testLifterAchievement()], you: true } });
    expect(screen.queryByText("Closing in")).not.toBeInTheDocument();
  });

  it("lists every rung it is given", () => {
    render(AchievementList, {
      props: {
        items: [],
        you: true,
        upcoming: [
          testUpcomingMilestone(),
          testUpcomingMilestone({
            kind: "volume",
            label: "1,000,000 lb lifted, all time",
            targetLb: 1_000_000,
            currentLb: 820_000,
            exerciseId: 0,
            exerciseName: "",
          }),
        ],
      },
    });

    expect(screen.getByText("First 225 lb Squat")).toBeInTheDocument();
    expect(screen.getByText("1,000,000 lb lifted, all time")).toBeInTheDocument();
    expect(screen.getByText("180,000 lb to go")).toBeInTheDocument();
  });
});

describe("sharing", () => {
  // Offered only on a crown they still hold. "I used to be top of this" is not a
  // brag, and the list keeps showing lapsed reigns for a different reason.
  it("offers a share button for a crown they hold", () => {
    render(AchievementList, {
      props: { items: [testLifterAchievement()], you: true, onShare: () => {} },
    });
    expect(
      screen.getByRole("button", { name: "Share Top of Week streak" }),
    ).toBeInTheDocument();
  });

  it("offers none for a reign that has ended", () => {
    render(AchievementList, {
      props: {
        items: [testLifterAchievement({ heldNow: false })],
        you: true,
        onShare: () => {},
      },
    });
    expect(screen.queryByRole("button", { name: /^Share/ })).not.toBeInTheDocument();
  });

  // Another lifter's profile passes no handler, so there is nothing to press —
  // sharing somebody else's crown is not the reader's to do.
  it("offers none when no handler was given", () => {
    render(AchievementList, { props: { items: [testLifterAchievement()] } });
    expect(screen.queryByRole("button", { name: /^Share/ })).not.toBeInTheDocument();
  });

  it("hands the achievement back when pressed", async () => {
    const onShare = vi.fn();
    const held = testLifterAchievement();
    render(AchievementList, { props: { items: [held], you: true, onShare } });

    await fireEvent.click(screen.getByRole("button", { name: "Share Top of Week streak" }));
    expect(onShare).toHaveBeenCalledWith(held);
  });
});
