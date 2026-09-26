import { render, screen, waitFor } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import LifterProfile from "./LifterProfile.svelte";
import type { LifterProfile as Profile } from "../lib/api";
import { auth } from "../lib/auth.svelte";
import { levels, resetLevels } from "../lib/levels.svelte";
import {
  testLifter,
  testLifterLevel,
  testRackedReport,
  testUser,
} from "../lib/testFixtures";

// A render test for the profile, which earns its place the way Racked.test.ts
// does: the page is assembled from two requests that can fail independently, and
// the branches worth covering are the ones where something is missing — an id
// that names nobody, a statistics call that failed while the identity arrived, a
// month with nothing in it.

const getLifter = vi.hoisted(() => vi.fn());
const getLifterRacked = vi.hoisted(() => vi.fn());
const listLifterSessions = vi.hoisted(() => vi.fn());
vi.mock("../lib/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../lib/api")>()),
  getLifter,
  getLifterRacked,
  listLifterSessions,
}));

/** One row of somebody's history, as the session list draws it. */
function session(over: Record<string, unknown> = {}) {
  return {
    id: 7,
    programId: 1,
    programName: "Starting Strength",
    programDayId: 10,
    programDayName: "Workout A",
    performedOn: "2026-03-17",
    setCount: 15,
    completedSetCount: 15,
    volumeLb: 8450,
    isOver: true,
    exercises: [],
    ...over,
  };
}

function history(items: unknown[], total = items.length) {
  return { status: 200, data: { items, total, totalVolumeLb: 250_000, limit: 10, offset: 0 } };
}

function profile(overrides: Partial<Profile> = {}): Profile {
  return {
    ...testLifter(),
    sessionCount: 40,
    lifetimeVolumeLb: 250_000,
    ...overrides,
  };
}

const busyMonth = testRackedReport({
  totals: { volumeLb: 84_000, sessions: 12, sets: 180, reps: 900 },
  streak: { longestWeeks: 9, currentWeeks: 4 },
});

beforeEach(() => {
  vi.clearAllMocks();
  getLifter.mockResolvedValue({ status: 200, data: profile() });
  getLifterRacked.mockResolvedValue({ status: 200, data: busyMonth });
  listLifterSessions.mockResolvedValue(history([]));
});

describe("LifterProfile", () => {
  it("names the lifter and their lifetime totals", async () => {
    render(LifterProfile, { params: { id: "2" } });

    await waitFor(() => {
      expect(screen.getByText("Grace Hopper")).toBeInTheDocument();
    });
    expect(screen.getByText("Lifetime volume")).toBeInTheDocument();
    expect(screen.getByText("40")).toBeInTheDocument();
  });

  it("reads the month's figures off the Racked report", async () => {
    render(LifterProfile, { params: { id: "2" } });

    await waitFor(() => {
      expect(screen.getByText("March 2026")).toBeInTheDocument();
    });
    // Sessions and sets for the month, straight from totals — nothing on this
    // page recomputes them.
    expect(screen.getByText("12")).toBeInTheDocument();
    expect(screen.getByText("180")).toBeInTheDocument();
    expect(screen.getByText("Week streak")).toBeInTheDocument();
    expect(screen.getByText("4")).toBeInTheDocument();
  });

  it("asks for the profile before the statistics, and only once for each", async () => {
    render(LifterProfile, { params: { id: "2" } });

    await waitFor(() => {
      expect(getLifterRacked).toHaveBeenCalledTimes(1);
    });
    expect(getLifter).toHaveBeenCalledTimes(1);
    expect(getLifter).toHaveBeenCalledWith(2);
    expect(getLifterRacked).toHaveBeenCalledWith(2, { period: "month" });
  });

  // An id that names nobody is an answer, not a failure. A retry button would be
  // offering to ask a question that already has a final reply.
  it("says there is no such lifter on a 404, with nothing to retry", async () => {
    getLifter.mockResolvedValue({ status: 404, data: undefined });
    render(LifterProfile, { params: { id: "999" } });

    await waitFor(() => {
      expect(screen.getByText("No such lifter")).toBeInTheDocument();
    });
    expect(screen.queryByRole("button", { name: /retry|try again/i })).not.toBeInTheDocument();
    // And it never went looking for statistics about somebody who isn't there.
    expect(getLifterRacked).not.toHaveBeenCalled();
  });

  it("offers a retry when the profile request itself fails", async () => {
    getLifter.mockResolvedValue({ status: 500, data: undefined });
    render(LifterProfile, { params: { id: "2" } });

    await waitFor(() => {
      expect(screen.getByText(/Couldn't load this lifter/)).toBeInTheDocument();
    });
  });

  // The two requests fail independently, and a failed statistics call must not
  // take down a page whose identity half already arrived.
  it("still shows who they are when the statistics call fails", async () => {
    getLifterRacked.mockResolvedValue({ status: 500, data: undefined });
    render(LifterProfile, { params: { id: "2" } });

    await waitFor(() => {
      expect(screen.getByText("Grace Hopper")).toBeInTheDocument();
    });
    expect(screen.getByText("Lifetime volume")).toBeInTheDocument();
    // No month section, and no error card standing in for the whole page.
    expect(screen.queryByText("March 2026")).not.toBeInTheDocument();
    expect(screen.queryByText(/Couldn't load this lifter/)).not.toBeInTheDocument();
  });

  // A quiet month is said in words. Drawn as a row of zeroes it reads as a page
  // that failed to load.
  it("says so plainly when the month holds nothing", async () => {
    getLifterRacked.mockResolvedValue({ status: 200, data: testRackedReport() });
    render(LifterProfile, { params: { id: "2" } });

    await waitFor(() => {
      expect(
        screen.getByText(/Grace Hopper hasn't logged anything this month/),
      ).toBeInTheDocument();
    });
  });

  // lastTrainedOn is absent for an account that has never logged a rep.
  it("does not invent a last-trained date for a lifter who has never trained", async () => {
    getLifter.mockResolvedValue({
      status: 200,
      data: profile({ sessionCount: 0, lifetimeVolumeLb: 0 }),
    });
    getLifterRacked.mockResolvedValue({ status: 200, data: testRackedReport() });
    render(LifterProfile, { params: { id: "2" } });

    await waitFor(() => {
      expect(screen.getByText(/hasn't trained yet/)).toBeInTheDocument();
    });
    expect(screen.queryByText(/1970/)).not.toBeInTheDocument();
  });
});

// The Experience section: the badge worn beside the name, written out with the
// progress the badge cannot show.
//
// The module is seeded rather than mocked, for the reason LifterName.test.ts seeds
// it: the lookup is as much of this as the markup, and a card drawn from the wrong
// lifter's row is the failure that matters. It is also the only test anywhere that
// renders those figures — LevelCard's other caller is a hover card, whose contents
// no test opens.
describe("the level on a profile", () => {
  /** What the server says about everyone, as App.svelte's poll would have left it. */
  function standing(items: ReturnType<typeof testLifterLevel>[]) {
    levels.items = items;
    levels.loaded = true;
  }

  afterEach(resetLevels);

  it("draws the level and how far into it they are", async () => {
    standing([
      testLifterLevel({
        lifterId: 2,
        level: 12,
        xp: 6900,
        xpIntoLevel: 300,
        xpForNextLevel: 1200,
      }),
    ]);
    render(LifterProfile, { params: { id: "2" } });

    const section = await screen.findByTestId("lifter-level");
    expect(section).toHaveTextContent("Level 12");
    // Lifetime, grouped — "6900 XP" is a number nobody reads at a glance.
    expect(section).toHaveTextContent("6,900 XP");
    // The progress itself: both halves of the bar's fraction, and the remainder
    // spelled out for a reader who cannot see the bar.
    expect(section).toHaveTextContent("300 / 1,200 XP");
    expect(section).toHaveTextContent("900 to Level 13");
  });

  // Scoped to the lifter the page is about. A section reading the first row of the
  // list would pass every assertion above on an install with one account.
  it("reads the row belonging to this lifter", async () => {
    standing([
      testLifterLevel({ lifterId: 3, level: 30 }),
      testLifterLevel({ lifterId: 2, level: 12 }),
    ]);
    render(LifterProfile, { params: { id: "2" } });

    const section = await screen.findByTestId("lifter-level");
    expect(section).toHaveTextContent("Level 12");
    expect(section).not.toHaveTextContent("Level 30");
  });

  // Level 1 on no experience is an ordinary state, not an empty one — it is what
  // every account that has never trained is, and the bar sits at nothing rather
  // than the section standing down.
  it("draws a lifter who has never trained at the bottom of level 1", async () => {
    standing([
      testLifterLevel({
        lifterId: 2,
        level: 1,
        xp: 0,
        xpIntoLevel: 0,
        xpForNextLevel: 100,
      }),
    ]);
    render(LifterProfile, { params: { id: "2" } });

    const section = await screen.findByTestId("lifter-level");
    expect(section).toHaveTextContent("Level 1");
    expect(section).toHaveTextContent("0 / 100 XP");
  });

  // Nothing is fetched for this section, so before the site-wide poll lands there
  // is no answer to draw — and a "Level 1" placeholder would be a claim about
  // somebody the client has not been told about yet.
  it("stands the section down until the levels have landed", async () => {
    render(LifterProfile, { params: { id: "2" } });

    await waitFor(() => {
      expect(screen.getByText("Lifetime volume")).toBeInTheDocument();
    });
    expect(screen.queryByTestId("lifter-level")).toBeNull();
  });

  // An id the list does not carry — an account created since the last poll. Same
  // answer as above, which is the whole reason levelFor conflates them.
  it("draws nothing for a lifter the list does not carry", async () => {
    standing([testLifterLevel({ lifterId: 3, level: 30 })]);
    render(LifterProfile, { params: { id: "2" } });

    await waitFor(() => {
      expect(screen.getByText("Lifetime volume")).toBeInTheDocument();
    });
    expect(screen.queryByTestId("lifter-level")).toBeNull();
  });

  // Reading it about yourself is covered with the rest of the second-person prose,
  // in "your own profile" below — where the point is that this section is the part
  // that does NOT change.
});

// The section that turns this page from a summary into somewhere to go. Until
// it existed the only route to another lifter's recap — the screen where one
// lifter applauds another — was the feed.
describe("LifterProfile sessions", () => {
  it("lists their training", async () => {
    listLifterSessions.mockResolvedValue(
      history([session({ id: 7, programDayName: "Workout A" })]),
    );
    render(LifterProfile, { params: { id: "2" } });

    await waitFor(() => {
      expect(screen.getByTestId("lifter-sessions")).toBeInTheDocument();
    });
    expect(screen.getByText(/Workout A/)).toBeInTheDocument();
    expect(listLifterSessions).toHaveBeenCalledWith(2, { limit: 10 });
  });

  // Every row goes to the OWNER-scoped recap. A reader-scoped link would name a
  // session the server will not hand over, because /sessions/{id} is scoped to
  // the caller.
  it("links each row to that lifter's recap", async () => {
    listLifterSessions.mockResolvedValue(history([session({ id: 7 })]));
    render(LifterProfile, { params: { id: "2" } });

    await waitFor(() => {
      expect(screen.getByTestId("lifter-sessions")).toBeInTheDocument();
    });
    const link = screen
      .getAllByRole("link")
      .find((a) => a.getAttribute("href")?.includes("/sessions/7"));
    expect(link).toHaveAttribute("href", "#/lifters/2/sessions/7/recap");
  });

  // An account that has never logged a rep gets no section at all — the two
  // lifetime cards above already say so, and an empty heading would be noise.
  it("draws nothing for a lifter who has never trained", async () => {
    listLifterSessions.mockResolvedValue(history([]));
    render(LifterProfile, { params: { id: "2" } });

    await waitFor(() => {
      expect(screen.getByText("Lifetime volume")).toBeInTheDocument();
    });
    expect(screen.queryByTestId("lifter-sessions")).toBeNull();
  });

  it("offers more only while there are more", async () => {
    listLifterSessions.mockResolvedValue(history([session({ id: 7 })], 3));
    render(LifterProfile, { params: { id: "2" } });

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Load more" })).toBeInTheDocument();
    });
  });

  it("pages from where the list has got to", async () => {
    listLifterSessions.mockResolvedValue(history([session({ id: 7 })], 2));
    render(LifterProfile, { params: { id: "2" } });
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Load more" })).toBeInTheDocument();
    });

    listLifterSessions.mockResolvedValue(
      history([session({ id: 6, programDayName: "Workout B" })], 2),
    );
    (await screen.findByRole("button", { name: "Load more" })).click();

    await waitFor(() => {
      expect(listLifterSessions).toHaveBeenLastCalledWith(2, { limit: 10, offset: 1 });
    });
  });

  // The history is allowed to fail on its own, like the statistics above it:
  // the identity and the lifetime totals have arrived and are worth showing.
  it("stands the section down when the history cannot be read", async () => {
    listLifterSessions.mockResolvedValue({ status: 500, data: undefined });
    render(LifterProfile, { params: { id: "2" } });

    await waitFor(() => {
      expect(screen.getByText("Lifetime volume")).toBeInTheDocument();
    });
    expect(screen.queryByTestId("lifter-sessions")).toBeNull();
  });
});

// The other surface that draws the Follow control, and the one where the follow
// state arrives free — it rides on the profile response the page already fetches.
describe("following from the profile", () => {
  beforeEach(() => {
    auth.me = testUser({ id: 1 });
    auth.loaded = true;
  });

  it("offers to follow a lifter it does not follow", async () => {
    render(LifterProfile, { params: { id: "2" } });

    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Follow Grace Hopper" }),
      ).toBeInTheDocument(),
    );
  });

  it("shows one already followed as followed", async () => {
    getLifter.mockResolvedValue({ status: 200, data: profile({ following: true }) });
    render(LifterProfile, { params: { id: "2" } });

    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Unfollow Grace Hopper" }),
      ).toBeInTheDocument(),
    );
  });

  // Not on your own profile: you cannot follow yourself, and a control that always
  // refuses is worse than no control.
  it("offers nothing on your own profile", async () => {
    getLifter.mockResolvedValue({
      status: 200,
      data: profile({ id: 1, displayName: "Ada Lovelace" }),
    });
    render(LifterProfile, { params: { id: "1" } });

    await waitFor(() =>
      expect(screen.getByText("Ada Lovelace")).toBeInTheDocument(),
    );
    expect(screen.queryByRole("button", { name: /^Follow/ })).not.toBeInTheDocument();
  });
});

// The account menu links straight here now, which makes your own profile a page
// lifters actually arrive at rather than one they stumbled into from the roster.
// It is the same component drawing the same history either way — what these
// cover is that the prose switches person when the subject is the reader.
describe("your own profile", () => {
  beforeEach(() => {
    auth.me = testUser({ id: 1 });
    auth.loaded = true;
    getLifter.mockResolvedValue({
      status: 200,
      data: profile({ id: 1, displayName: "Ada Lovelace" }),
    });
  });

  afterEach(resetLevels);

  it("names the sections in the second person", async () => {
    listLifterSessions.mockResolvedValue(history([session()]));
    render(LifterProfile, { params: { id: "1" } });

    await waitFor(() => {
      expect(screen.getByText("Your training")).toBeInTheDocument();
    });
    expect(screen.getByText("What you trained")).toBeInTheDocument();
    expect(screen.queryByText("Their training")).not.toBeInTheDocument();
    expect(screen.queryByText("What they trained")).not.toBeInTheDocument();
  });

  // The section is drawn even when empty, so the one line in it that has to name
  // somebody is the line that would otherwise talk about the reader in the third
  // person. `getLifterAchievements` is unmocked on purpose — apiFetch turns the
  // jsdom network failure into status 0, which is the empty case.
  it("addresses you in the empty achievements list", async () => {
    render(LifterProfile, { params: { id: "1" } });

    await waitFor(() => {
      expect(
        screen.getByText(/Lead any board on the leaderboard and its crown is yours/),
      ).toBeInTheDocument();
    });
  });

  it("says so in the second person when the month is empty", async () => {
    getLifterRacked.mockResolvedValue({ status: 200, data: testRackedReport() });
    render(LifterProfile, { params: { id: "1" } });

    await waitFor(() => {
      expect(
        screen.getByText("You haven't logged anything this month."),
      ).toBeInTheDocument();
    });
  });

  // Where the Follow control sits on anybody else's profile. Arriving from the
  // account menu, this page is where you notice the display name is stale — and
  // until now the edit for it was somewhere else entirely.
  it("offers the way back to the settings screen", async () => {
    render(LifterProfile, { params: { id: "1" } });

    const settings = await screen.findByRole("link", { name: "Configure profile" });
    expect(settings).toHaveAttribute("href", "#/profile");
  });

  // The Experience section is the part that does NOT switch with the reader, and
  // that is deliberate: "how far into the level am I" has to be the same page as
  // "how far into the level are they", or the section is a second feature wearing
  // the first's name. Nothing is hidden by drawing it either — XP is a session
  // count times a hundred, and the session count is the tile above it.
  it("draws the level in the same terms it draws anybody else's", async () => {
    levels.items = [testLifterLevel({ lifterId: 1, level: 12, xpIntoLevel: 300 })];
    levels.loaded = true;
    render(LifterProfile, { params: { id: "1" } });

    const section = await screen.findByTestId("lifter-level");
    expect(section).toHaveTextContent("Level 12");
    expect(section).toHaveTextContent("300 / 1,200 XP");
  });

  // Somebody else's profile keeps the third person and keeps the Follow control.
  // Asserted here rather than left to the cases above, because one flag drives
  // both and a bug that pinned it true would pass every test in this block.
  it("leaves another lifter's profile in the third person", async () => {
    // The flag reads the profile the server returned, not the id in the URL, so
    // this override is the whole of what makes it somebody else.
    getLifter.mockResolvedValue({ status: 200, data: profile() });
    listLifterSessions.mockResolvedValue(history([session()]));
    render(LifterProfile, { params: { id: "2" } });

    await waitFor(() => {
      expect(screen.getByText("Their training")).toBeInTheDocument();
    });
    expect(screen.getByText("What they trained")).toBeInTheDocument();
    expect(
      screen.queryByRole("link", { name: "Configure profile" }),
    ).not.toBeInTheDocument();
  });
});
