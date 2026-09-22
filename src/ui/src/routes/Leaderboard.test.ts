import { render, screen, waitFor, fireEvent, within } from "@testing-library/svelte";
import { beforeEach, describe, expect, it, vi } from "vitest";
import Leaderboard from "./Leaderboard.svelte";
import { auth } from "../lib/auth.svelte";
import { testBoard, testBoardEntry, testLifter, testUser } from "../lib/testFixtures";

// The leaderboard.
//
// Two behaviours carry the design and get the most cover: the page opens on the
// board the SERVER put first (which is the fairness opinion, and must not be
// re-sorted or defaulted away here), and switching boards costs no request
// because they all arrive together.

const getLeaderboard = vi.hoisted(() => vi.fn());
vi.mock("../lib/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../lib/api")>()),
  getLeaderboard,
}));

const ME = 1;

function twoLifters(value = 3) {
  return [
    testBoardEntry({ rank: 1, lifter: testLifter({ id: ME, displayName: "Ada" }), value }),
    testBoardEntry({
      rank: 2,
      lifter: testLifter({ id: 2, displayName: "Grace" }),
      value: value - 1,
    }),
  ];
}

function payload(boards = [testBoard({ entries: twoLifters() })]) {
  return {
    status: 200,
    data: {
      period: {
        kind: "month",
        start: "2026-03-01",
        end: "2026-03-31",
        label: "March 2026",
        inProgress: false,
      },
      boards,
    },
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  getLeaderboard.mockResolvedValue(payload());
  auth.me = testUser({ id: ME });
  auth.loaded = true;
});

describe("Leaderboard", () => {
  it("opens on the month, and on the board the server put first", async () => {
    getLeaderboard.mockResolvedValue(
      payload([
        testBoard({ metric: "sessionsPerWeek", label: "Sessions a week", entries: twoLifters() }),
        testBoard({
          metric: "volume",
          label: "Volume",
          unit: "pounds",
          entries: twoLifters(50_000),
        }),
      ]),
    );
    render(Leaderboard);

    await waitFor(() => {
      expect(getLeaderboard).toHaveBeenCalledWith({ period: "month" });
    });
    // The fairness ordering is the server's, and the page must not default past it
    // to the board people ask for first.
    const shown = screen.getByTestId("leaderboard-board");
    expect(within(shown).getByText("3.0×")).toBeInTheDocument();
    expect(within(shown).queryByText(/lb$/)).not.toBeInTheDocument();
  });

  it("ranks the lifters and marks the reader", async () => {
    render(Leaderboard);

    await waitFor(() => {
      expect(screen.getByText("Ada")).toBeInTheDocument();
    });
    expect(screen.getByText("Grace")).toBeInTheDocument();
    // First place gets a medal rather than a bare "1".
    expect(screen.getByText("🥇")).toBeInTheDocument();
    expect(screen.getByText("🥈")).toBeInTheDocument();
    expect(screen.getAllByText("(you)")).toHaveLength(1);
  });

  // They all arrive in one response, so switching is free. A refetch here would
  // mean re-running the heaviest query in the API to read a different field off
  // the same result.
  it("switches board without another request", async () => {
    getLeaderboard.mockResolvedValue(
      payload([
        testBoard({ label: "Sessions a week", entries: twoLifters() }),
        testBoard({
          metric: "volume",
          label: "Volume",
          unit: "pounds",
          entries: twoLifters(50_000),
        }),
      ]),
    );
    render(Leaderboard);

    await waitFor(() => expect(getLeaderboard).toHaveBeenCalledTimes(1));
    await fireEvent.click(screen.getByText("Volume"));

    await waitFor(() => {
      expect(screen.getByText("50,000 lb")).toBeInTheDocument();
    });
    expect(getLeaderboard).toHaveBeenCalledTimes(1);
  });

  it("refetches when the period changes, keeping the board you were reading", async () => {
    getLeaderboard.mockResolvedValue(
      payload([
        testBoard({ label: "Sessions a week", entries: twoLifters() }),
        testBoard({
          metric: "volume",
          label: "Volume",
          unit: "pounds",
          entries: twoLifters(50_000),
        }),
      ]),
    );
    render(Leaderboard);

    await waitFor(() => expect(getLeaderboard).toHaveBeenCalledTimes(1));
    await fireEvent.click(screen.getByText("Volume"));
    await fireEvent.click(screen.getByText("year"));

    await waitFor(() => {
      expect(getLeaderboard).toHaveBeenCalledWith({ period: "year" });
    });
    // Still the volume board — a period switcher that reset the board would make
    // comparing a year to a month two clicks instead of one.
    await waitFor(() => {
      expect(screen.getByText("50,000 lb")).toBeInTheDocument();
    });
  });

  it("formats each unit the way the rest of the app does", async () => {
    getLeaderboard.mockResolvedValue(
      payload([
        testBoard({
          metric: "attendance",
          label: "Attendance",
          unit: "percent",
          // Fractions on the wire — 0.75 is 75%.
          entries: [
            testBoardEntry({ lifter: testLifter({ id: ME }), value: 0.75 }),
            testBoardEntry({ rank: 2, lifter: testLifter({ id: 2 }), value: 0.5 }),
          ],
        }),
        testBoard({
          metric: "streak",
          label: "Week streak",
          unit: "count",
          entries: [
            testBoardEntry({ lifter: testLifter({ id: ME }), value: 4 }),
            testBoardEntry({ rank: 2, lifter: testLifter({ id: 2 }), value: 2 }),
          ],
        }),
      ]),
    );
    render(Leaderboard);

    await waitFor(() => {
      expect(screen.getByText("75%")).toBeInTheDocument();
    });
    await fireEvent.click(screen.getByText("Week streak"));
    await waitFor(() => {
      expect(screen.getByText("4")).toBeInTheDocument();
    });
  });

  it("names the lift behind a most-improved figure", async () => {
    getLeaderboard.mockResolvedValue(
      payload([
        testBoard({
          metric: "improvement",
          label: "Most improved",
          unit: "percent",
          entries: [
            testBoardEntry({ lifter: testLifter({ id: ME }), value: 0.08, detail: "Squat" }),
            testBoardEntry({
              rank: 2,
              lifter: testLifter({ id: 2 }),
              value: 0.03,
              detail: "Bench Press",
            }),
          ],
        }),
      ]),
    );
    render(Leaderboard);

    await waitFor(() => {
      expect(screen.getByText("Squat")).toBeInTheDocument();
    });
  });

  // On the wire because a board that omits lifters — attendance does — has to say
  // so where it is drawn.
  it("shows the board's own note", async () => {
    getLeaderboard.mockResolvedValue(
      payload([
        testBoard({
          note: "Lifters whose program carries no weekdays are not listed.",
          entries: twoLifters(),
        }),
      ]),
    );
    render(Leaderboard);

    await waitFor(() => {
      expect(
        screen.getByText("Lifters whose program carries no weekdays are not listed."),
      ).toBeInTheDocument();
    });
  });

  // A leaderboard of one is not a leaderboard — same call the feed card makes.
  it("says there is nothing to compare on a one-lifter install", async () => {
    getLeaderboard.mockResolvedValue(
      payload([testBoard({ entries: [testBoardEntry({ lifter: testLifter({ id: ME }) })] })]),
    );
    render(Leaderboard);

    await waitFor(() => {
      expect(screen.getByText(/only lifter on this install/)).toBeInTheDocument();
    });
    expect(screen.queryByTestId("leaderboard-board")).not.toBeInTheDocument();
  });

  // The one-lifter check counts lifters across every board, not off the first one.
  // If it leaned on board 0 — which lists everybody today only because
  // sessions-a-week is defined for everybody — then an empty leading board would
  // make a populated install claim it was alone.
  it("does not read an empty leading board as a one-lifter install", async () => {
    getLeaderboard.mockResolvedValue(
      payload([
        testBoard({ metric: "attendance", label: "Attendance", unit: "percent", entries: [] }),
        testBoard({ metric: "volume", label: "Volume", unit: "pounds", entries: twoLifters() }),
      ]),
    );
    render(Leaderboard);

    await waitFor(() => {
      expect(screen.getByTestId("leaderboard-board")).toBeInTheDocument();
    });
    expect(screen.queryByText(/only lifter on this install/)).not.toBeInTheDocument();
  });

  // A board can list nobody while others list everyone: attendance does exactly
  // that when no lifter has a scheduled program.
  it("handles a board with no qualifying lifters", async () => {
    getLeaderboard.mockResolvedValue(
      payload([
        testBoard({ entries: twoLifters() }),
        testBoard({ metric: "attendance", label: "Attendance", unit: "percent", entries: [] }),
      ]),
    );
    render(Leaderboard);

    await waitFor(() => expect(screen.getByText("Ada")).toBeInTheDocument());
    await fireEvent.click(screen.getByText("Attendance"));

    await waitFor(() => {
      expect(screen.getByText(/Nothing to show on this board/)).toBeInTheDocument();
    });
  });

  it("offers a retry when the request fails", async () => {
    getLeaderboard.mockResolvedValue({ status: 500, data: undefined });
    render(Leaderboard);

    await waitFor(() => {
      expect(screen.getByText(/Couldn't load the leaderboard/)).toBeInTheDocument();
    });
  });
});
