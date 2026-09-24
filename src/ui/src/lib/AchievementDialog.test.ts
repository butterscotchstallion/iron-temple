import { render, screen, waitFor, fireEvent } from "@testing-library/svelte";
import { afterAll, afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import AchievementDialog from "./AchievementDialog.svelte";
import { achievements, resetAchievements } from "./achievements.svelte";
import { auth } from "./auth.svelte";
import {
  testAchievement,
  testAchievementHolders,
  testLifter,
  testNotification,
  testUser,
} from "./testFixtures";

// The dialog a folded crown row opens.
//
// What is worth most here is the cycling and the standing line. Next has to
// actually advance rather than dismiss — it is a plain Button precisely because
// AlertDialog.Action would close the dialog — and the "still theirs" line is the
// one thing stopping the dialog from reporting an hours-old event as current.

const GRACE = testLifter({ id: 2, displayName: "Grace Hopper" });
const ADA = testLifter({ id: 3, displayName: "Ada Lovelace" });

/** A crown notification about `actor` on `slug`. */
function crown(actor = GRACE, slug = "crown-streak", id = 1) {
  return testNotification({
    id,
    kind: "crown",
    emoji: undefined,
    sessionId: undefined,
    sessionOwnerId: undefined,
    programDayName: undefined,
    achievementSlug: slug,
    actor,
  });
}

/** Seed the catalogue with one crown and whoever currently holds it. */
function catalogue(holders = [GRACE], achievement = testAchievement()) {
  achievements.items = [testAchievementHolders({ achievement, holders })];
  achievements.loaded = true;
}

// The dialog asks who the actor is so it can draw a Follow button in the right
// state — a notification's actor carries no `following`, deliberately.
const getLifter = vi.hoisted(() => vi.fn());
const followLifter = vi.hoisted(() => vi.fn());
vi.mock("./api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./api")>()),
  getLifter,
  followLifter,
}));

const onLeaderboard = vi.fn();

function show(items: ReturnType<typeof crown>[], loading = false) {
  return render(AchievementDialog, {
    props: { open: true, items, loading, onLeaderboard },
  });
}

beforeEach(() => {
  onLeaderboard.mockReset();
  getLifter.mockReset().mockResolvedValue({
    status: 200,
    data: { ...GRACE, sessionCount: 1, lifetimeVolumeLb: 1000, following: false },
  });
  followLifter.mockReset().mockResolvedValue({ status: 204, data: undefined });
  auth.me = testUser({ id: 1 });
  auth.loaded = true;
  catalogue();
});
afterEach(() => resetAchievements());

// Same bits-ui scroll-lock teardown race UpdatePrompt.test.ts documents: the
// timer it schedules on unmount fires into a torn-down jsdom unless something
// waits for it. Loses the whole run to an unhandled error while every test
// reports as passing.
afterAll(async () => {
  await new Promise((resolve) => setTimeout(resolve, 100));
});

describe("one card", () => {
  it("names the board, who took it and what it means", async () => {
    show([crown()]);

    expect(await screen.findByText("Top of Week streak")).toBeInTheDocument();
    expect(screen.getByText("Grace Hopper")).toBeInTheDocument();
    expect(
      screen.getByText("Held the longest run of consecutive weeks trained."),
    ).toBeInTheDocument();
  });

  // The catalogue is a separate request. A cold client must still get a dialog
  // that reads as a sentence rather than one with holes in it.
  it("falls back when the catalogue has not loaded", async () => {
    resetAchievements();
    show([crown()]);

    expect(await screen.findByText("A crown was taken")).toBeInTheDocument();
    expect(
      screen.getByText("Somebody took the top of a leaderboard board."),
    ).toBeInTheDocument();
  });

  // No position label on a group of one — "1 of 1" is a count nobody needs, and
  // it is the common case.
  it("does not count a group of one", async () => {
    show([crown()]);

    await screen.findByText("Top of Week streak");
    expect(screen.queryByText(/1 of 1/)).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Next" })).not.toBeInTheDocument();
  });

  it("says nothing is there when the members came back empty", async () => {
    show([]);
    expect(await screen.findByText(/isn't here any more/)).toBeInTheDocument();
  });

  it("shows a placeholder while the members are on their way", async () => {
    show([], true);
    // Loading renders its label as "<label>…", which is the sentence a screen
    // reader gets — matched loosely so the ellipsis is not the assertion.
    expect(await screen.findByText(/Loading the achievement/)).toBeInTheDocument();
  });
});

describe("the standing line", () => {
  it("says a crown is still theirs when they still hold it", async () => {
    catalogue([GRACE]);
    show([crown(GRACE)]);

    expect(await screen.findByText("Still theirs.")).toBeInTheDocument();
  });

  // The whole reason this line exists: a notification is an event, and it can be
  // hours old by the time anybody opens the panel.
  it("names the new holder when it has changed hands", async () => {
    catalogue([ADA]);
    show([crown(GRACE)]);

    expect(await screen.findByText("Since taken by Ada Lovelace.")).toBeInTheDocument();
  });

  // Ties share rank 1, so a board can have several holders at once.
  it("names every holder of a tied board", async () => {
    catalogue([ADA, testLifter({ id: 4, displayName: "Joan Clarke" })]);
    show([crown(GRACE)]);

    expect(
      await screen.findByText("Since taken by Ada Lovelace and Joan Clarke."),
    ).toBeInTheDocument();
  });

  // A board led at zero crowns nobody, so an empty holder list is a real state
  // rather than a missing fetch.
  it("says nobody holds it when the crown has lapsed entirely", async () => {
    catalogue([]);
    show([crown(GRACE)]);

    expect(await screen.findByText("Nobody holds this right now.")).toBeInTheDocument();
  });

  // Saying "nobody holds this" on the strength of a catalogue we have not fetched
  // would be inventing an answer.
  it("says nothing about standing before the catalogue has loaded", async () => {
    resetAchievements();
    show([crown(GRACE)]);

    await screen.findByText("A crown was taken");
    expect(screen.queryByText(/Still theirs/)).not.toBeInTheDocument();
    expect(screen.queryByText(/Nobody holds this/)).not.toBeInTheDocument();
  });
});

describe("cycling", () => {
  const VOLUME = testAchievement({
    slug: "crown-volume",
    metric: "volume",
    label: "Top of Volume",
    description: "Moved more total weight than anyone else this month.",
  });

  /** A group of two crowns on different boards, taken by different lifters. */
  function pair() {
    achievements.items = [
      testAchievementHolders({ holders: [GRACE] }),
      testAchievementHolders({ achievement: VOLUME, holders: [ADA] }),
    ];
    achievements.loaded = true;
    return [crown(GRACE, "crown-streak", 1), crown(ADA, "crown-volume", 2)];
  }

  it("counts the group and opens on the first", async () => {
    show(pair());

    expect(await screen.findByText("Top of Week streak")).toBeInTheDocument();
    expect(screen.getByText("1 of 2")).toBeInTheDocument();
  });

  // Next ADVANCES. It is a plain Button rather than AlertDialog.Action for
  // exactly this reason — Action closes the dialog, which would make Next dismiss
  // the thing it was meant to move through.
  it("advances to the next crown without closing", async () => {
    show(pair());
    await screen.findByText("Top of Week streak");

    await fireEvent.click(screen.getByRole("button", { name: "Next" }));

    await waitFor(() => expect(screen.getByText("Top of Volume")).toBeInTheDocument());
    expect(screen.getByText("2 of 2")).toBeInTheDocument();
    expect(screen.getByTestId("achievement-dialog")).toBeInTheDocument();
    // The card is the other lifter's now, which is what says the cursor moved
    // rather than just the title.
    expect(screen.getByText("Ada Lovelace")).toBeInTheDocument();
  });

  it("offers no Next on the last card", async () => {
    show(pair());
    await screen.findByText("Top of Week streak");

    await fireEvent.click(screen.getByRole("button", { name: "Next" }));

    await waitFor(() => expect(screen.getByText("2 of 2")).toBeInTheDocument());
    expect(screen.queryByRole("button", { name: "Next" })).not.toBeInTheDocument();
  });

  it("walks a group of three all the way", async () => {
    const third = crown(GRACE, "crown-streak", 3);
    show([...pair(), third]);
    await screen.findByText("1 of 3");

    await fireEvent.click(screen.getByRole("button", { name: "Next" }));
    await waitFor(() => expect(screen.getByText("2 of 3")).toBeInTheDocument());
    await fireEvent.click(screen.getByRole("button", { name: "Next" }));
    await waitFor(() => expect(screen.getByText("3 of 3")).toBeInTheDocument());
    expect(screen.queryByRole("button", { name: "Next" })).not.toBeInTheDocument();
  });

  // The cursor is clamped by resetting on the LIST rather than on `open`, so a
  // shorter list arriving after somebody has paged forward cannot leave it
  // pointing past the end.
  it("returns to the first card when the members change", async () => {
    const { rerender } = show(pair());
    await screen.findByText("1 of 2");
    await fireEvent.click(screen.getByRole("button", { name: "Next" }));
    await waitFor(() => expect(screen.getByText("2 of 2")).toBeInTheDocument());

    await rerender({ open: true, items: [crown()], loading: false, onLeaderboard });

    await waitFor(() =>
      expect(screen.getByText("Top of Week streak")).toBeInTheDocument(),
    );
    expect(screen.queryByText(/of 2/)).not.toBeInTheDocument();
  });
});

describe("leaving", () => {
  it("hands the leaderboard back to its caller", async () => {
    show([crown()]);

    await fireEvent.click(await screen.findByRole("button", { name: "Leaderboard" }));
    expect(onLeaderboard).toHaveBeenCalled();
  });

  it("closes on Close", async () => {
    show([crown()]);

    await fireEvent.click(await screen.findByRole("button", { name: "Close" }));
    await waitFor(() =>
      expect(screen.queryByTestId("achievement-dialog")).not.toBeInTheDocument(),
    );
  });
});

// Getting from "who is this?" to following them. The notification that told you
// about the crown is where you act on it, rather than being sent off to a profile
// to come back from.
describe("reaching the lifter", () => {
  it("links the name to their profile", async () => {
    show([crown()]);
    const anchor = await screen.findByRole("link", { name: "Grace Hopper" });
    expect(anchor).toHaveAttribute("href", "#/lifters/2");
  });

  it("offers to follow them", async () => {
    show([crown()]);

    await waitFor(() => expect(getLifter).toHaveBeenCalledWith(2));
    expect(
      await screen.findByRole("button", { name: "Follow Grace Hopper" }),
    ).toBeInTheDocument();
  });

  it("shows one already followed as followed", async () => {
    getLifter.mockResolvedValue({
      status: 200,
      data: { ...GRACE, sessionCount: 1, lifetimeVolumeLb: 1000, following: true },
    });
    show([crown()]);

    expect(
      await screen.findByRole("button", { name: "Unfollow Grace Hopper" }),
    ).toBeInTheDocument();
  });

  it("follows from inside the dialog", async () => {
    show([crown()]);

    await fireEvent.click(await screen.findByRole("button", { name: /^Follow/ }));

    await waitFor(() => expect(followLifter).toHaveBeenCalledWith(2));
    expect(
      await screen.findByRole("button", { name: "Unfollow Grace Hopper" }),
    ).toBeInTheDocument();
  });

  // A Follow button that might already be Following is worse than none, so a
  // failed or in-flight lookup draws nothing.
  it("offers nothing when it could not find out", async () => {
    getLifter.mockResolvedValue({ status: 500, data: {} });
    show([crown()]);

    await screen.findByText("Top of Week streak");
    expect(screen.queryByRole("button", { name: /^Follow/ })).not.toBeInTheDocument();
  });

  // Your own crown: nothing to follow, nobody to link to, and no request made.
  it("asks nothing and offers nothing about your own crown", async () => {
    const me = testLifter({ id: 1, displayName: "Ada Lovelace" });
    show([crown(me)]);

    await screen.findByText("Top of Week streak");
    expect(screen.getByText("You")).toBeInTheDocument();
    expect(getLifter).not.toHaveBeenCalled();
    expect(screen.queryByRole("button", { name: /^Follow/ })).not.toBeInTheDocument();
  });

  // The answer is keyed by lifter id, so a slow reply about the previous card
  // cannot be drawn against this one.
  it("follows the cursor to the next card's lifter", async () => {
    achievements.items = [
      testAchievementHolders({ holders: [GRACE] }),
      testAchievementHolders({
        achievement: testAchievement({ slug: "crown-volume", label: "Top of Volume" }),
        holders: [ADA],
      }),
    ];
    achievements.loaded = true;
    show([crown(GRACE, "crown-streak", 1), crown(ADA, "crown-volume", 2)]);

    await screen.findByRole("button", { name: "Follow Grace Hopper" });

    getLifter.mockResolvedValue({
      status: 200,
      data: { ...ADA, sessionCount: 1, lifetimeVolumeLb: 1000, following: true },
    });
    await fireEvent.click(screen.getByRole("button", { name: "Next" }));

    expect(
      await screen.findByRole("button", { name: "Unfollow Ada Lovelace" }),
    ).toBeInTheDocument();
    expect(getLifter).toHaveBeenLastCalledWith(3);
  });
});
