import { describe, it, expect, afterEach } from "vitest";
import { render, screen } from "@testing-library/svelte";
import LifterName from "./LifterName.svelte";
import { achievements, resetAchievements } from "./achievements.svelte";
import {
  testAchievement,
  testAchievementHolders,
  testLifter,
  testUser,
} from "./testFixtures";

// Two things here are worth more than the rest.
//
// The name has to keep rendering exactly as it did when it was written out at ten
// call sites — this component was extracted, not designed, so a regression here
// is a regression on the feed, the roster, the leaderboard and the header at
// once. And a crown has to be readable without a mouse: it is the entire
// user-visible payload of the feature, and an icon with no accessible name is a
// decoration that says nothing to a screen reader.

/**
 * Put crowns on lifters.
 *
 * Seeds the module the component reads rather than mocking it, because what is
 * under test is the lookup as much as the markup — a crown drawn for the wrong
 * lifter is the failure that matters, and a mocked `crownsFor` could not catch
 * it.
 */
function crowned(entries: { achievement: ReturnType<typeof testAchievement>; ids: number[] }[]) {
  achievements.items = entries.map((e) =>
    testAchievementHolders({
      achievement: e.achievement,
      holders: e.ids.map((id) => testLifter({ id })),
    }),
  );
  achievements.loaded = true;
}

afterEach(() => resetAchievements());

describe("the name", () => {
  it("renders the display name", () => {
    render(LifterName, { props: { lifter: testUser() } });
    expect(screen.getByText("Ada Lovelace")).toBeInTheDocument();
  });

  // An account created through the admin area has no display name until its
  // owner sets one, so this is an ordinary state and not an edge case.
  it("falls back to the username when there is no display name", () => {
    render(LifterName, { props: { lifter: testUser({ displayName: "" }) } });
    expect(screen.getByText("ada")).toBeInTheDocument();
  });
});

describe("crowns", () => {
  it("draws none for a lifter holding nothing", () => {
    crowned([{ achievement: testAchievement(), ids: [2] }]);
    render(LifterName, { props: { lifter: testUser({ id: 1 }) } });
    // The name, and nothing claiming a board.
    expect(screen.getByText("Ada Lovelace")).toBeInTheDocument();
    expect(screen.queryByText("Top of Week streak")).not.toBeInTheDocument();
  });

  it("names the board a crown is for, in text a screen reader reaches", () => {
    crowned([{ achievement: testAchievement(), ids: [1] }]);
    render(LifterName, { props: { lifter: testUser({ id: 1 }) } });
    expect(screen.getByText("Top of Week streak")).toBeInTheDocument();
  });

  // ONE CROWN PER BOARD is the whole decision the component encodes: a lifter
  // leading three boards has to be distinguishable from one leading a single
  // board, and each icon has to say which of the three it is.
  it("draws one per board led, each naming its own board", () => {
    crowned([
      { achievement: testAchievement(), ids: [1] },
      {
        achievement: testAchievement({
          slug: "crown-volume",
          metric: "volume",
          label: "Top of Volume",
        }),
        ids: [1],
      },
      {
        achievement: testAchievement({
          slug: "crown-attendance",
          metric: "attendance",
          label: "Top of Attendance",
        }),
        ids: [1],
      },
    ]);
    render(LifterName, { props: { lifter: testUser({ id: 1 }) } });

    expect(screen.getByText("Top of Week streak")).toBeInTheDocument();
    expect(screen.getByText("Top of Volume")).toBeInTheDocument();
    expect(screen.getByText("Top of Attendance")).toBeInTheDocument();
  });

  // Ties share rank 1 on a board, so two lifters genuinely level are both
  // crowned. Asserting the SECOND of them gets it is what catches a lookup that
  // only ever reads a board's first holder.
  it("crowns every holder when a board is tied", () => {
    crowned([{ achievement: testAchievement(), ids: [1, 2] }]);
    render(LifterName, { props: { lifter: testLifter({ id: 2 }) } });
    expect(screen.getByText("Top of Week streak")).toBeInTheDocument();
  });

  // The crown belongs to the lifter the component was handed, not to whoever
  // happens to be first in the holders list.
  it("does not lend one lifter's crown to another", () => {
    crowned([{ achievement: testAchievement(), ids: [2] }]);
    render(LifterName, { props: { lifter: testUser({ id: 1 }) } });
    expect(screen.queryByText("Top of Week streak")).not.toBeInTheDocument();
  });

  // The icon is decorative — the board name is rendered beside it — so it must
  // not be announced as well, which is the rule every other icon in this app
  // follows.
  it("keeps the icon itself out of the accessibility tree", () => {
    crowned([{ achievement: testAchievement(), ids: [1] }]);
    const { container } = render(LifterName, { props: { lifter: testUser({ id: 1 }) } });
    const icon = container.querySelector("svg");
    expect(icon).not.toBeNull();
    expect(icon).toHaveAttribute("aria-hidden", "true");
  });

  // Before the first load nobody is wearing anything, and that must render as a
  // plain name rather than as a crash or a crown for lifter `undefined`.
  it("draws nothing before the standings have loaded", () => {
    render(LifterName, { props: { lifter: testUser({ id: 1 }) } });
    expect(screen.getByText("Ada Lovelace")).toBeInTheDocument();
    expect(document.querySelector("svg")).toBeNull();
  });
});
