import { render, screen, waitFor } from "@testing-library/svelte";
import { beforeEach, describe, expect, it, vi } from "vitest";
import LifterSessionRecap from "./LifterSessionRecap.svelte";
import { testLifter, testSessionRecap } from "../lib/testFixtures";

// Somebody else's session, in review.
//
// Two things here are worth defending. A 404 covers both "no such session" and
// "not that lifter's" — the endpoint does not distinguish them, so neither may
// this. And "what you earned" must not appear: it is a forecast computed from the
// owner's history, and on a page about somebody else it reads as your own.

const getLifterSessionRecap = vi.hoisted(() => vi.fn());
const getLifter = vi.hoisted(() => vi.fn());
vi.mock("../lib/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../lib/api")>()),
  getLifterSessionRecap,
  getLifter,
}));

const params = { lifterId: "5", sessionId: "42" };

beforeEach(() => {
  vi.clearAllMocks();
  getLifterSessionRecap.mockResolvedValue({ status: 200, data: testSessionRecap() });
  getLifter.mockResolvedValue({
    status: 200,
    data: { ...testLifter({ id: 5 }), sessionCount: 3, lifetimeVolumeLb: 100 },
  });
});

describe("LifterSessionRecap", () => {
  it("names the workout and whose it was", async () => {
    render(LifterSessionRecap, { params });

    await waitFor(() => {
      expect(screen.getByText("Workout A")).toBeInTheDocument();
    });
    expect(screen.getByText(/Grace Hopper/)).toBeInTheDocument();
    expect(getLifterSessionRecap).toHaveBeenCalledWith(5, 42);
  });

  // The endpoint refuses to say which of the two it was, so that a caller cannot
  // use it to learn which ids are sessions.
  it("treats a 404 as one answer, whichever reason it was", async () => {
    getLifterSessionRecap.mockResolvedValue({ status: 404, data: undefined });
    render(LifterSessionRecap, { params });

    await waitFor(() => {
      expect(screen.getByText("No such session")).toBeInTheDocument();
    });
    expect(
      screen.queryByRole("button", { name: /retry|try again/i }),
    ).not.toBeInTheDocument();
    // And it never asked who the lifter was.
    expect(getLifter).not.toHaveBeenCalled();
  });

  it("offers a retry when the request fails for another reason", async () => {
    getLifterSessionRecap.mockResolvedValue({ status: 500, data: undefined });
    render(LifterSessionRecap, { params });

    await waitFor(() => {
      expect(screen.getByText(/Couldn't load this session/)).toBeInTheDocument();
    });
  });

  // The name is a nicety fetched separately; losing it must not cost the page.
  it("still draws the session when the lifter lookup fails", async () => {
    getLifter.mockResolvedValue({ status: 500, data: undefined });
    render(LifterSessionRecap, { params });

    await waitFor(() => {
      expect(screen.getByText("Workout A")).toBeInTheDocument();
    });
    expect(screen.queryByText(/Couldn't load this session/)).not.toBeInTheDocument();
  });

  // The deliberate omission. Even when the server sends it, this page does not
  // draw somebody else's next prescription.
  it("never shows what the owner earned", async () => {
    getLifterSessionRecap.mockResolvedValue({
      status: 200,
      data: testSessionRecap({
        earned: {
          programDayId: 7,
          programDayName: "Workout A",
          exercises: [],
        },
      }),
    });
    render(LifterSessionRecap, { params });

    await waitFor(() => {
      expect(screen.getByText("Workout A")).toBeInTheDocument();
    });
    expect(screen.queryByText(/earned/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/next time/i)).not.toBeInTheDocument();
  });
});
