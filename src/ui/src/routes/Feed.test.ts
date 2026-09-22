import { render, screen, waitFor, fireEvent } from "@testing-library/svelte";
import { beforeEach, describe, expect, it, vi } from "vitest";
import Feed from "./Feed.svelte";
import { testFeedEntry } from "../lib/testFixtures";

// The full feed page. Unlike the card on Home, this one has an empty state to
// show and a page to accumulate — and because the endpoint sends no total, "is
// there more" is inferred from the page length, which is the branch most worth
// covering.

const getFeed = vi.hoisted(() => vi.fn());
vi.mock("../lib/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../lib/api")>()),
  getFeed,
}));

const PAGE = 20;

function page(count: number, offset = 0) {
  return {
    status: 200,
    data: {
      items: Array.from({ length: count }, (_, i) =>
        testFeedEntry({ id: offset * 1000 + i }),
      ),
      limit: PAGE,
      offset,
    },
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  getFeed.mockResolvedValue(page(3));
});

describe("Feed", () => {
  it("lists what the others have logged", async () => {
    render(Feed);

    await waitFor(() => {
      expect(screen.getAllByRole("link")).toHaveLength(3);
    });
    expect(getFeed).toHaveBeenCalledWith({ limit: PAGE, offset: 0 });
  });

  // The single-lifter install, which is the one this app was built for. Said
  // without implying anything went wrong.
  it("explains an empty feed rather than looking broken", async () => {
    getFeed.mockResolvedValue(page(0));
    render(Feed);

    await waitFor(() => {
      expect(screen.getByText(/Nothing here yet/)).toBeInTheDocument();
    });
    expect(screen.queryByText("Load more")).not.toBeInTheDocument();
    expect(screen.queryByText(/Couldn't load/)).not.toBeInTheDocument();
  });

  it("offers a retry when the first page fails", async () => {
    getFeed.mockResolvedValue({ status: 500, data: undefined });
    render(Feed);

    await waitFor(() => {
      expect(screen.getByText(/Couldn't load the feed/)).toBeInTheDocument();
    });
  });

  // A short page is the end of the list, which is the only signal there is.
  it("does not offer Load more on a short page", async () => {
    render(Feed);

    await waitFor(() => {
      expect(screen.getAllByRole("link")).toHaveLength(3);
    });
    expect(screen.queryByText("Load more")).not.toBeInTheDocument();
  });

  it("appends the next page and advances the offset by what has arrived", async () => {
    getFeed.mockResolvedValueOnce(page(PAGE)).mockResolvedValueOnce(page(2, 1));
    render(Feed);

    await waitFor(() => {
      expect(screen.getByText("Load more")).toBeInTheDocument();
    });

    await fireEvent.click(screen.getByText("Load more"));

    await waitFor(() => {
      expect(screen.getAllByRole("link")).toHaveLength(PAGE + 2);
    });
    expect(getFeed).toHaveBeenLastCalledWith({ limit: PAGE, offset: PAGE });
    // The second page was short, so there is nothing further to offer.
    expect(screen.queryByText("Load more")).not.toBeInTheDocument();
  });

  // A failed "load more" keeps what is already on screen. The rows a lifter is
  // reading are not worth discarding to report a page that did not arrive.
  it("keeps the rows it has when loading more fails", async () => {
    getFeed
      .mockResolvedValueOnce(page(PAGE))
      .mockResolvedValueOnce({ status: 500, data: undefined });
    render(Feed);

    await waitFor(() => {
      expect(screen.getByText("Load more")).toBeInTheDocument();
    });

    await fireEvent.click(screen.getByText("Load more"));

    await waitFor(() => {
      expect(screen.queryByText("Load more")).not.toBeInTheDocument();
    });
    expect(screen.getAllByRole("link")).toHaveLength(PAGE);
    expect(screen.queryByText(/Couldn't load the feed/)).not.toBeInTheDocument();
  });
});
