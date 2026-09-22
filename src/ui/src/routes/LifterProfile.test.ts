import { render, screen, waitFor } from "@testing-library/svelte";
import { beforeEach, describe, expect, it, vi } from "vitest";
import LifterProfile from "./LifterProfile.svelte";
import type { LifterProfile as Profile } from "../lib/api";
import { testLifter, testRackedReport } from "../lib/testFixtures";

// A render test for the profile, which earns its place the way Racked.test.ts
// does: the page is assembled from two requests that can fail independently, and
// the branches worth covering are the ones where something is missing — an id
// that names nobody, a statistics call that failed while the identity arrived, a
// month with nothing in it.

const getLifter = vi.hoisted(() => vi.fn());
const getLifterRacked = vi.hoisted(() => vi.fn());
vi.mock("../lib/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../lib/api")>()),
  getLifter,
  getLifterRacked,
}));

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
