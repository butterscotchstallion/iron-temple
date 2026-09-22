import { render, screen, waitFor } from "@testing-library/svelte";
import { beforeEach, describe, expect, it, vi } from "vitest";
import Lifters from "./Lifters.svelte";
import { auth } from "../lib/auth.svelte";
import { testLifter, testUser } from "../lib/testFixtures";

// A render test for the roster. The interesting branch is `lastTrainedOn`, which
// is absent for an account that has never logged a rep — the field most likely
// to be read off nothing — plus marking the row that is the reader themselves.

const listLifters = vi.hoisted(() => vi.fn());
vi.mock("../lib/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../lib/api")>()),
  listLifters,
}));

// Exactly one of the three has never trained, so the assertions below can use
// getByText for that row rather than counting matches.
const me = testLifter({
  id: 1,
  username: "ada",
  displayName: "Ada Lovelace",
  lastTrainedOn: "2026-03-20",
});
const trained = testLifter({
  id: 2,
  username: "grace",
  displayName: "Grace Hopper",
  lastTrainedOn: "2026-03-17",
});
const untrained = testLifter({ id: 3, username: "alan", displayName: "Alan Turing" });

beforeEach(() => {
  vi.clearAllMocks();
  listLifters.mockResolvedValue({ status: 200, data: [me, trained, untrained] });
  auth.me = testUser({ id: 1 });
  auth.loaded = true;
});

describe("Lifters", () => {
  it("lists everyone on the install", async () => {
    render(Lifters);

    await waitFor(() => {
      expect(screen.getByText("Ada Lovelace")).toBeInTheDocument();
    });
    expect(screen.getByText("Grace Hopper")).toBeInTheDocument();
    expect(screen.getByText("Alan Turing")).toBeInTheDocument();
  });

  it("dates the last session for a lifter who has trained", async () => {
    render(Lifters);

    await waitFor(() => {
      expect(screen.getAllByText(/Last trained/)).toHaveLength(2);
    });
  });

  // The whole point of sending the field as absent rather than as a zero date:
  // an account that has never trained has no day to name, and must not be shown
  // one.
  it("says so rather than inventing a date for a lifter who has never trained", async () => {
    render(Lifters);

    await waitFor(() => {
      expect(screen.getByText("Hasn't trained yet")).toBeInTheDocument();
    });
    expect(screen.queryByText(/1970/)).not.toBeInTheDocument();
  });

  it("marks the reader's own row", async () => {
    render(Lifters);

    await waitFor(() => {
      expect(screen.getByText("(you)")).toBeInTheDocument();
    });
    // Exactly one row is the reader's, however many accounts there are.
    expect(screen.getAllByText("(you)")).toHaveLength(1);
  });

  it("links each lifter to their profile", async () => {
    render(Lifters);

    await waitFor(() => {
      expect(screen.getByText("Grace Hopper")).toBeInTheDocument();
    });
    const link = screen.getByText("Grace Hopper").closest("a");
    // Hash-based routing: `use:link` rewrites the href it is given, so the
    // rendered attribute carries the "#" the router navigates on.
    expect(link).toHaveAttribute("href", "#/lifters/2");
  });

  it("reports a failure instead of rendering an empty roster", async () => {
    listLifters.mockResolvedValue({ status: 500, data: undefined });
    render(Lifters);

    await waitFor(() => {
      expect(screen.getByText(/Couldn't load the lifters/)).toBeInTheDocument();
    });
  });
});
