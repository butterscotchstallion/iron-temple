import { render, screen, waitFor, fireEvent } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import ActivityPanel from "./ActivityPanel.svelte";
import Toaster from "./Toaster.svelte";
import { resetToasts } from "./toast.svelte";
import type { ActivityStatus } from "./api";

// The owner's control for generated activity.
//
// The two things worth defending: teardown is behind a confirmation, because it
// deletes accounts and everything that cascades from them; and the status is
// polled only while a loop is running, because idle there is nothing to watch and
// a panel that asked anyway would be a request every five seconds forever.
//
// Successes leave this component entirely — they go to the toast store, which the
// shell renders. So the tests that read one mount the Toaster alongside the panel
// via `renderWithToasts` below. Failures stay in the panel's own error banner, and
// those assertions need no toaster.

const getActivityStatus = vi.hoisted(() => vi.fn());
const backfillActivity = vi.hoisted(() => vi.fn());
const startActivity = vi.hoisted(() => vi.fn());
const stopActivity = vi.hoisted(() => vi.fn());
const deleteActivity = vi.hoisted(() => vi.fn());
const getActivitySchedule = vi.hoisted(() => vi.fn());
const setActivitySchedule = vi.hoisted(() => vi.fn());
vi.mock("./api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./api")>()),
  getActivityStatus,
  backfillActivity,
  startActivity,
  stopActivity,
  deleteActivity,
  getActivitySchedule,
  setActivitySchedule,
}));

function daily(over: Partial<{ enabled: boolean; lifters: number; lastRunOn: string; lastSessions: number; lastReactions: number; lastComments: number }> = {}) {
  return { status: 200 as const, data: { enabled: false, lifters: 4, ...over } };
}

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

/**
 * The panel plus the notice region the shell normally provides.
 *
 * Two renders rather than a fixture component: both mount into document.body, so
 * `screen` spans them, and nothing about the pairing needs its own markup. The
 * real App.svelte keeps them apart for a reason — a notice has to outlive the
 * route that raised it — and a fixture would quietly imply otherwise.
 */
function renderWithToasts() {
  render(ActivityPanel);
  render(Toaster);
}

beforeEach(() => {
  vi.clearAllMocks();
  // Module state, shared by every test in the process.
  resetToasts();
  getActivityStatus.mockResolvedValue(status());
  backfillActivity.mockResolvedValue({
    status: 200,
    data: { accounts: 4, sessions: 137, reactions: 210, comments: 64 },
  });
  startActivity.mockResolvedValue({ status: 204, data: undefined });
  stopActivity.mockResolvedValue({ status: 204, data: undefined });
  deleteActivity.mockResolvedValue({ status: 200, data: { removed: 4 } });
  getActivitySchedule.mockResolvedValue(daily());
  // A write is reflected by subsequent reads, because every action reloads when it
  // succeeds. A mock that answered the old state afterwards would have the panel
  // report "Off." immediately after switching it on — and would quietly let a
  // regression there pass.
  setActivitySchedule.mockImplementation(async (body) => {
    const written = daily(body);
    getActivitySchedule.mockResolvedValue(written);
    return written;
  });
});

afterEach(() => {
  vi.useRealTimers();
  resetToasts();
});

describe("ActivityPanel", () => {
  it("reads the current status on mount", async () => {
    render(ActivityPanel);
    await waitFor(() => expect(getActivityStatus).toHaveBeenCalledTimes(1));
  });

  it("generates history and reports what it made", async () => {
    renderWithToasts();
    await waitFor(() => expect(getActivityStatus).toHaveBeenCalled());

    await fireEvent.click(screen.getByText("Generate history"));

    await waitFor(() => {
      expect(backfillActivity).toHaveBeenCalledWith({ lifters: 4, weeks: 12 });
    });
    await waitFor(() => {
      expect(screen.getByText(/4 new lifters, 137 sessions/)).toBeInTheDocument();
    });
  });

  // The request is synchronous and generating three months takes seconds with
  // nothing arriving, so the notice opens before the call and says what it is
  // doing — then becomes the result rather than being replaced by a second toast.
  it("says it is generating while the request is open, and resolves that notice", async () => {
    let finish: (value: unknown) => void = () => {};
    backfillActivity.mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    );
    renderWithToasts();
    await waitFor(() => expect(getActivityStatus).toHaveBeenCalled());

    await fireEvent.click(screen.getByText("Generate history"));

    await waitFor(() => {
      expect(screen.getByText("Generating history…")).toBeInTheDocument();
    });
    expect(screen.getByText("12 weeks across 4 lifters.")).toBeInTheDocument();

    finish({ status: 200, data: { accounts: 4, sessions: 137, reactions: 210, comments: 64 } });

    await waitFor(() => {
      expect(screen.getByText("History generated")).toBeInTheDocument();
    });
    // Resolved in place: the pending wording is gone rather than sitting above it.
    expect(screen.queryByText("Generating history…")).not.toBeInTheDocument();
  });

  // Otherwise a failed backfill leaves a spinner on screen forever, next to a
  // banner saying it didn't work.
  it("takes the pending notice down when generating fails", async () => {
    backfillActivity.mockResolvedValue({ status: 500, data: undefined });
    renderWithToasts();
    await waitFor(() => expect(getActivityStatus).toHaveBeenCalled());

    await fireEvent.click(screen.getByText("Generate history"));

    await waitFor(() => expect(screen.getByText(/That didn't work/)).toBeInTheDocument());
    expect(screen.queryByText("Generating history…")).not.toBeInTheDocument();
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
    renderWithToasts();
    await waitFor(() => expect(getActivityStatus).toHaveBeenCalled());

    await fireEvent.click(screen.getByText("Remove generated lifters"));
    // By role: the dialog's action and the button that opened it both read
    // "Remove…" to a text query.
    const confirm = await waitFor(() => screen.getByRole("button", { name: "Remove" }));
    await fireEvent.click(confirm);

    await waitFor(() => expect(deleteActivity).toHaveBeenCalled());
    await waitFor(() => {
      expect(screen.getByText("Removed 4 accounts")).toBeInTheDocument();
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

  // The point of leaving this screen open. A poll that finds the count has moved
  // says what was done, in the server's own words.
  it("raises a notice for work the loop does while the screen is open", async () => {
    vi.useFakeTimers();
    getActivityStatus.mockResolvedValue(
      status({ running: true, lifters: 3, tickSeconds: 20, actions: 7 }),
    );
    renderWithToasts();
    await vi.waitFor(() => expect(getActivityStatus).toHaveBeenCalledTimes(1));

    getActivityStatus.mockResolvedValue(
      status({
        running: true,
        lifters: 3,
        tickSeconds: 20,
        actions: 8,
        lastAction: "Mara Quinn logged Workout A",
      }),
    );
    await vi.advanceTimersByTimeAsync(5_500);

    await vi.waitFor(() => {
      expect(screen.getByText("Mara Quinn logged Workout A")).toBeInTheDocument();
    });
  });

  // A loop that has been running for an hour must not greet the screen with a
  // toast for each of the forty things it did before anyone was looking.
  it("says nothing about work already done when the screen opens", async () => {
    getActivityStatus.mockResolvedValue(
      status({
        running: true,
        lifters: 3,
        tickSeconds: 20,
        actions: 40,
        lastAction: "Mara Quinn logged Workout A",
      }),
    );
    renderWithToasts();

    // The panel's own status line reports it; the toaster does not.
    await waitFor(() => expect(screen.getByText(/40 actions/)).toBeInTheDocument());
    expect(screen.queryByTestId("toaster")?.textContent?.trim()).toBe("");
  });

  // Several actions between two polls: only the last one has a description, so the
  // notice says so rather than quietly reporting one of them.
  it("counts the ones it could not name", async () => {
    vi.useFakeTimers();
    getActivityStatus.mockResolvedValue(status({ running: true, lifters: 3, actions: 2 }));
    renderWithToasts();
    await vi.waitFor(() => expect(getActivityStatus).toHaveBeenCalledTimes(1));

    getActivityStatus.mockResolvedValue(
      status({ running: true, lifters: 3, actions: 5, lastAction: "Dev Oyelaran said something" }),
    );
    await vi.advanceTimersByTimeAsync(5_500);

    await vi.waitFor(() => {
      expect(screen.getByText(/and 2 more since the last check/)).toBeInTheDocument();
    });
  });

  // The roster is the SCOPE of a clean-up, and it comes from the server rather
  // than a hard-coded list. Which of those accounts actually go is decided by the
  // password hash, so the copy must not claim a real lifter would be deleted.
  it("names the accounts clean-up considers, from the server's roster", async () => {
    render(ActivityPanel);

    await waitFor(() => {
      expect(screen.getByText(/mara\.quinn, dev\.oyelaran/)).toBeInTheDocument();
    });
    expect(screen.getByText(/keeps the ones that aren't actually generated/)).toBeInTheDocument();
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
      screen.getByText(/a real lifter who happens to share one of those names/i),
    ).toBeInTheDocument();
  });

  // ---- the unattended daily run ----
  //
  // Distinct from Live above: this one is a row in the database, so it survives a
  // restart. The panel has to make that difference legible rather than offering two
  // switches that look the same.
  it("offers to switch the daily run on when it is off", async () => {
    render(ActivityPanel);

    await waitFor(() => {
      expect(screen.getByText("Switch on")).toBeInTheDocument();
    });
    expect(screen.getByText("Off.")).toBeInTheDocument();
  });

  it("switches it on, carrying the lifter count with it", async () => {
    render(ActivityPanel);
    await waitFor(() => expect(screen.getByText("Switch on")).toBeInTheDocument());

    await fireEvent.click(screen.getByText("Switch on"));

    await waitFor(() => {
      expect(setActivitySchedule).toHaveBeenCalledWith({ enabled: true, lifters: 4 });
    });
    // Said plainly, because the hourly tick means the first day is not immediate and
    // an operator who expected instant activity would think it was broken.
    //
    // Rendered without the toaster: switching on also reveals the panel's own
    // "Hasn't run yet" line, which carries the same sentence, so mounting both
    // would give this query two matches. The toast is asserted in the next test.
    await waitFor(() => {
      expect(screen.getByText(/first day lands within the hour/)).toBeInTheDocument();
    });
  });

  it("raises a notice when the daily run is switched on", async () => {
    renderWithToasts();
    await waitFor(() => expect(screen.getByText("Switch on")).toBeInTheDocument());

    await fireEvent.click(screen.getByText("Switch on"));

    await waitFor(() => {
      expect(screen.getByText("Running daily")).toBeInTheDocument();
    });
  });

  it("offers to switch it off once it is on", async () => {
    getActivitySchedule.mockResolvedValue(daily({ enabled: true, lifters: 3 }));
    render(ActivityPanel);

    await waitFor(() => {
      expect(screen.getByText("Switch off")).toBeInTheDocument();
    });
    expect(screen.getByText("On, across 3 lifters.")).toBeInTheDocument();
    expect(screen.queryByText("Switch on")).not.toBeInTheDocument();

    await fireEvent.click(screen.getByText("Switch off"));
    await waitFor(() => {
      expect(setActivitySchedule).toHaveBeenCalledWith({ enabled: false, lifters: 3 });
    });
  });

  // Enabled and never having run is the ordinary state for the first hour, so the
  // absence is explained rather than left blank.
  it("explains that an enabled schedule has not run yet", async () => {
    getActivitySchedule.mockResolvedValue(daily({ enabled: true, lifters: 4 }));
    render(ActivityPanel);

    await waitFor(() => {
      expect(screen.getByText(/Hasn't run yet/)).toBeInTheDocument();
    });
  });

  // What it last DID, not only that it is on — which is the difference between a
  // schedule you can trust and a flag.
  it("reports the last day it generated", async () => {
    getActivitySchedule.mockResolvedValue(
      daily({
        enabled: true,
        lifters: 4,
        lastRunOn: "2026-03-17",
        lastSessions: 3,
        lastReactions: 11,
        lastComments: 2,
      }),
    );
    render(ActivityPanel);

    await waitFor(() => {
      expect(screen.getByText("2026-03-17")).toBeInTheDocument();
    });
    expect(screen.getByText(/3 sessions, 11 reactions, 2 comments/)).toBeInTheDocument();
    expect(screen.queryByText(/Hasn't run yet/)).not.toBeInTheDocument();
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
