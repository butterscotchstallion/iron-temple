import { render, screen, waitFor } from "@testing-library/svelte";
import { beforeEach, describe, expect, it, vi } from "vitest";
import FeedCard from "./FeedCard.svelte";
import { testFeedEntry } from "./testFixtures";

// The card at the foot of Home.
//
// The behaviour worth defending is what it does with NOTHING: this app is built
// for one lifter, whose feed is empty by construction, and their Home must look
// exactly as it always did. No heading, no empty state, no error. Every test here
// is really about that.

const getFeed = vi.hoisted(() => vi.fn());
vi.mock("./api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./api")>()),
  getFeed,
}));

function page(count: number) {
  return {
    status: 200,
    data: {
      items: Array.from({ length: count }, (_, i) => testFeedEntry({ id: 100 + i })),
      limit: 4,
      offset: 0,
    },
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  getFeed.mockResolvedValue(page(2));
});

describe("FeedCard", () => {
  it("draws the recent sessions", async () => {
    render(FeedCard);

    await waitFor(() => {
      expect(screen.getByText("Around the gym")).toBeInTheDocument();
    });
    expect(screen.getAllByRole("link")).toHaveLength(2);
  });

  // The single-lifter install. Nothing at all, not an empty state.
  it("renders nothing when the feed is empty", async () => {
    getFeed.mockResolvedValue(page(0));
    const { container } = render(FeedCard);

    await waitFor(() => {
      expect(getFeed).toHaveBeenCalled();
    });
    expect(screen.queryByText("Around the gym")).not.toBeInTheDocument();
    expect(container.textContent?.trim()).toBe("");
  });

  // Same silence for a failure. The workout is why Home was opened; an error card
  // about other people's sessions below it would be louder than what it failed to
  // fetch.
  it("renders nothing and says nothing when the request fails", async () => {
    getFeed.mockResolvedValue({ status: 500, data: undefined });
    const { container } = render(FeedCard);

    await waitFor(() => {
      expect(getFeed).toHaveBeenCalled();
    });
    expect(container.textContent?.trim()).toBe("");
    expect(screen.queryByText(/couldn't/i)).not.toBeInTheDocument();
  });

  it("asks for only a preview", async () => {
    render(FeedCard);
    await waitFor(() => {
      expect(getFeed).toHaveBeenCalledWith({ limit: 4 });
    });
  });

  // A link to a page holding the same rows this card already shows is a link to
  // nowhere, so it only appears once the page is full.
  it("offers See all only when there may be more", async () => {
    getFeed.mockResolvedValue(page(4));
    render(FeedCard);

    await waitFor(() => {
      expect(screen.getByText("See all")).toBeInTheDocument();
    });
  });

  it("does not offer See all on a short page", async () => {
    getFeed.mockResolvedValue(page(3));
    render(FeedCard);

    await waitFor(() => {
      expect(screen.getByText("Around the gym")).toBeInTheDocument();
    });
    expect(screen.queryByText("See all")).not.toBeInTheDocument();
  });
});
