import { render, screen } from "@testing-library/svelte";
import { describe, expect, it } from "vitest";
import FeedList from "./FeedList.svelte";
import { testFeedEntry, testLifter } from "./testFixtures";

// The rows themselves. Small on purpose: the one thing here that can be wrong in
// a way nothing else would catch is the href, which has to be built from the
// session's OWNER rather than from whoever is reading.

describe("FeedList", () => {
  it("links each row to the recap under its owner's id", () => {
    const entry = testFeedEntry({
      id: 77,
      lifter: testLifter({ id: 5, displayName: "Grace Hopper" }),
    });
    render(FeedList, { items: [entry] });

    const link = screen.getByText("Grace Hopper").closest("a");
    // The endpoint scopes the session by the lifter in the path, so a link built
    // from the reader's id would 404 on every row.
    expect(link).toHaveAttribute("href", "#/lifters/5/sessions/77/recap");
  });

  it("names the workout, the day and what it was worth", () => {
    render(FeedList, {
      items: [
        testFeedEntry({ programDayName: "Workout B", volumeLb: 12_500, setCount: 20 }),
      ],
    });

    expect(screen.getByText(/Workout B/)).toBeInTheDocument();
    expect(screen.getByText("20 sets")).toBeInTheDocument();
  });

  it("says 1 set rather than 1 sets", () => {
    render(FeedList, { items: [testFeedEntry({ setCount: 1 })] });
    expect(screen.getByText("1 set")).toBeInTheDocument();
  });

  it("falls back to the username when a lifter has no display name", () => {
    render(FeedList, {
      items: [testFeedEntry({ lifter: testLifter({ displayName: "", username: "grace" }) })],
    });
    expect(screen.getByText("grace")).toBeInTheDocument();
  });

  // Counts, not controls. A reaction button nested inside the row's link would be
  // a click target fighting its own parent.
  it("badges the applause and conversation a session drew", () => {
    render(FeedList, {
      items: [testFeedEntry({ reactionCount: 3, commentCount: 2 })],
    });

    expect(screen.getByText("3")).toBeInTheDocument();
    expect(screen.getByText("reactions")).toBeInTheDocument();
    expect(screen.getByText("2")).toBeInTheDocument();
    expect(screen.getByText("comments")).toBeInTheDocument();
    // No way to react from here — that needs the session in front of you.
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  it("singularises a lone reaction", () => {
    render(FeedList, { items: [testFeedEntry({ reactionCount: 1, commentCount: 1 })] });
    expect(screen.getByText("reaction")).toBeInTheDocument();
    expect(screen.getByText("comment")).toBeInTheDocument();
  });

  // The common case: most sessions have neither, and an empty badge reads as a
  // broken one.
  it("draws no badges when a session drew nothing", () => {
    render(FeedList, { items: [testFeedEntry({ reactionCount: 0, commentCount: 0 })] });
    expect(screen.queryByText(/reactions?$/)).not.toBeInTheDocument();
    expect(screen.queryByText(/comments?$/)).not.toBeInTheDocument();
  });

  it("draws only the badge it has", () => {
    render(FeedList, { items: [testFeedEntry({ reactionCount: 4, commentCount: 0 })] });
    expect(screen.getByText("reactions")).toBeInTheDocument();
    expect(screen.queryByText(/comments?$/)).not.toBeInTheDocument();
  });
});
