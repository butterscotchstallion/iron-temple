import { render, screen, waitFor, fireEvent } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import ActivityPanel from "./ActivityPanel.svelte";
import type { ActivityStatus } from "./api";

// The owner's control for generated activity.
//
// The two things worth defending: teardown is behind a confirmation, because it
// deletes accounts and everything that cascades from them; and the status is
// polled only while a loop is running, because idle there is nothing to watch and
// a panel that asked anyway would be a request every five seconds forever.

const getActivityStatus = vi.hoisted(() => vi.fn());
const backfillActivity = vi.hoisted(() => vi.fn());
const startActivity = vi.hoisted(() => vi.fn());
const stopActivity = vi.hoisted(() => vi.fn());
const deleteActivity = vi.hoisted(() => vi.fn());
vi.mock("./api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./api")>()),
  getActivityStatus,
  backfillActivity,
  startActivity,
  stopActivity,
  deleteActivity,
}));

function status(over: Partial<ActivityStatus> = {}): { status: 200; data: ActivityStatus } {
  return {
    status: 200,
    data: {
      running: false,
      lifters: 0,
      tickSeconds: 0,
      actions: 0,
      maxLifters: 8,
      maxWeeks: 26,
      // Two names is enough to assert the list is rendered from the server's
      // roster rather than hard-coded.
      roster: ["mara.quinn", "dev.oyelaran"],
      ...over,
    },
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  getActivityStatus.mockResolvedValue(status());
  backfillActivity.mockResolvedValue({
    status: 200,
    data: { accounts: 4, sessions: 137, reactions: 210, comments: 64 },
  });
  startActivity.mockResolvedValue({ status: 204, data: undefined });
  stopActivity.mockResolvedValue({ status: 204, data: undefined });
  deleteActivity.mockResolvedValue({ status: 200, data: { removed: 4 } });
});

afterEach(() => {
  vi.useRealTimers();
});

describe("ActivityPanel", () => {
  it("reads the current status on mount", async () => {
    render(ActivityPanel);
    await waitFor(() => expect(getActivityStatus).toHaveBeenCalledTimes(1));
  });

  it("generates history and reports what it made", async () => {
    render(ActivityPanel);
    await waitFor(() => expect(getActivityStatus).toHaveBeenCalled());

    await fireEvent.click(screen.getByText("Generate history"));

    await waitFor(() => {
      expect(backfillActivity).toHaveBeenCalledWith({ lifters: 4, weeks: 12 });
    });
    await waitFor(() => {
      expect(screen.getByText(/4 new lifters, 137 sessions/)).toBeInTheDocument();
    });
  });

  it("says so when generating fails instead of claiming it worked", async () => {
    backfillActivity.mockResolvedValue({ status: 500, data: undefined });
    render(ActivityPanel);
    await waitFor(() => expect(getActivityStatus).toHaveBeenCalled());

    await fireEvent.click(screen.getByText("Generate history"));

    await waitFor(() => {
      expect(screen.getByText(/That didn't work/)).toBeInTheDocument();
    });
    // No summary either. Matched on a figure rather than on the word "sessions",
    // which appears in the panel's own static copy.
    expect(screen.queryByText(/137 sessions/)).not.toBeInTheDocument();
  });

  it("starts the loop with the tick on screen", async () => {
    render(ActivityPanel);
    await waitFor(() => expect(getActivityStatus).toHaveBeenCalled());

    await fireEvent.click(screen.getByText("Start"));

    await waitFor(() => {
      expect(startActivity).toHaveBeenCalledWith({ lifters: 4, tickSeconds: 20 });
    });
  });

  it("offers Stop rather than Start while a loop is running", async () => {
    getActivityStatus.mockResolvedValue(
      status({ running: true, lifters: 3, tickSeconds: 20, actions: 7, lastAction: "Mara Quinn logged Workout A" }),
    );
    render(ActivityPanel);

    await waitFor(() => {
      expect(screen.getByText("Stop")).toBeInTheDocument();
    });
    expect(screen.queryByText("Start")).not.toBeInTheDocument();
    // The action count and the last thing done are how the screen shows the loop
    // is alive rather than merely flagged as on.
    expect(screen.getByText(/7 actions/)).toBeInTheDocument();
    expect(screen.getByText(/Mara Quinn logged Workout A/)).toBeInTheDocument();
  });

  it("stops the loop", async () => {
    getActivityStatus.mockResolvedValue(status({ running: true, lifters: 3, tickSeconds: 20 }));
    render(ActivityPanel);
    await waitFor(() => expect(screen.getByText("Stop")).toBeInTheDocument());

    await fireEvent.click(screen.getByText("Stop"));
    await waitFor(() => expect(stopActivity).toHaveBeenCalled());
  });

  // Deleting accounts, and everything that cascades from them, must not be one
  // click away.
  it("does not remove anything until the confirmation is accepted", async () => {
    render(ActivityPanel);
    await waitFor(() => expect(getActivityStatus).toHaveBeenCalled());

    await fireEvent.click(screen.getByText("Remove generated lifters"));

    await waitFor(() => {
      expect(screen.getByText("Remove the generated lifters?")).toBeInTheDocument();
    });
    expect(deleteActivity).not.toHaveBeenCalled();
  });

  it("removes them once confirmed, and says how many went", async () => {
    render(ActivityPanel);
    await waitFor(() => expect(getActivityStatus).toHaveBeenCalled());

    await fireEvent.click(screen.getByText("Remove generated lifters"));
    // By role: the dialog's action and the button that opened it both read
    // "Remove…" to a text query.
    const confirm = await waitFor(() => screen.getByRole("button", { name: "Remove" }));
    await fireEvent.click(confirm);

    await waitFor(() => expect(deleteActivity).toHaveBeenCalled());
    await waitFor(() => {
      expect(screen.getByText("Removed 4 accounts.")).toBeInTheDocument();
    });
  });

  it("removes nothing when the confirmation is declined", async () => {
    render(ActivityPanel);
    await waitFor(() => expect(getActivityStatus).toHaveBeenCalled());

    await fireEvent.click(screen.getByText("Remove generated lifters"));
    await waitFor(() => expect(screen.getByText("Keep them")).toBeInTheDocument());
    await fireEvent.click(screen.getByText("Keep them"));

    expect(deleteActivity).not.toHaveBeenCalled();
  });

  // Idle there is nothing to watch, so nothing is asked for.
  it("does not poll while the loop is idle", async () => {
    vi.useFakeTimers();
    render(ActivityPanel);
    await vi.waitFor(() => expect(getActivityStatus).toHaveBeenCalledTimes(1));

    await vi.advanceTimersByTimeAsync(30_000);
    expect(getActivityStatus).toHaveBeenCalledTimes(1);
  });

  it("polls while the loop is running, so the action count moves", async () => {
    vi.useFakeTimers();
    getActivityStatus.mockResolvedValue(status({ running: true, lifters: 3, tickSeconds: 20 }));
    render(ActivityPanel);
    await vi.waitFor(() => expect(getActivityStatus).toHaveBeenCalledTimes(1));

    await vi.advanceTimersByTimeAsync(11_000);
    expect(getActivityStatus.mock.calls.length).toBeGreaterThan(1);
  });

  // Clean-up matches on USERNAME, not on whether an account was generated, so an
  // account the owner made by hand under one of these names would be deleted with
  // all its history. Naming them is the only safeguard against that, which makes
  // these two assertions load-bearing rather than cosmetic.
  it("names the accounts clean-up would match, from the server's roster", async () => {
    render(ActivityPanel);

    await waitFor(() => {
      expect(screen.getByText(/mara\.quinn, dev\.oyelaran/)).toBeInTheDocument();
    });
    expect(screen.getByText(/including one you made yourself/)).toBeInTheDocument();
  });

  it("repeats the names inside the confirmation", async () => {
    render(ActivityPanel);
    await waitFor(() => expect(getActivityStatus).toHaveBeenCalled());

    await fireEvent.click(screen.getByText("Remove generated lifters"));

    await waitFor(() => {
      expect(screen.getByText("Remove the generated lifters?")).toBeInTheDocument();
    });
    // Twice now — under the button and again in the dialog, which is the last
    // thing read before agreeing.
    expect(screen.getAllByText(/mara\.quinn, dev\.oyelaran/)).toHaveLength(2);
    // The dialog's own wording, which the panel's shorter note does not use.
    expect(
      screen.getByText(/not on whether the account was generated/i),
    ).toBeInTheDocument();
  });

  // The server sends its own bounds so the inputs cannot offer a number the
  // endpoint would refuse.
  it("takes its input bounds from the server", async () => {
    getActivityStatus.mockResolvedValue(status({ maxLifters: 5, maxWeeks: 10 }));
    render(ActivityPanel);

    await waitFor(() => {
      expect(screen.getByLabelText("Lifters")).toHaveAttribute("max", "5");
    });
    expect(screen.getByLabelText("Weeks")).toHaveAttribute("max", "10");
  });
});
