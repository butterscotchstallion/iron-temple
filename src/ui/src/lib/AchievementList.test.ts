import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/svelte";
import AchievementList from "./AchievementList.svelte";
import { testAchievement, testLifterAchievement } from "./testFixtures";

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
