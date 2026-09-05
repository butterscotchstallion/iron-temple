import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/svelte";
import OfflineBanner from "./OfflineBanner.svelte";
import { markReachable, markUnreachable, resetConnectivity } from "./connectivity.svelte";
import { clearQueue, clearRejected, enqueue, flush } from "./writeQueue.svelte";

const updateSessionSet = vi.hoisted(() => vi.fn());

vi.mock("./api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./api")>()),
  updateSessionSet,
}));

const aSet = (setId: number) =>
  ({ kind: "updateSet", sessionId: 1, setId, body: { actualReps: 5 } }) as const;

beforeEach(() => {
  localStorage.clear();
  clearQueue();
  clearRejected();
  resetConnectivity();
});

afterEach(() => {
  vi.clearAllMocks();
});

// Online with nothing waiting is the normal state of the app. A permanent
// "connected" badge would be noise on the one screen where every pixel is a
// thumb target.
it("says nothing when everything is fine", () => {
  render(OfflineBanner);
  expect(screen.queryByRole("status")).not.toBeInTheDocument();
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
});

describe("offline", () => {
  // Worth saying precisely BECAUSE nothing on screen looks wrong — every tap
  // still registers. Told plainly it reads as "carry on"; unsaid, the sync
  // later looks like a glitch.
  it("tells the lifter to keep going", () => {
    markUnreachable();
    render(OfflineBanner);

    expect(screen.getByRole("status")).toHaveTextContent(
      /Offline — keep logging, it'll sync when you're back/,
    );
  });

  it("says how much is waiting", () => {
    markUnreachable();
    enqueue(aSet(1));
    enqueue(aSet(2));
    render(OfflineBanner);

    expect(screen.getByRole("status")).toHaveTextContent(/2 changes saved here/);
  });

  it("counts one change in the singular", () => {
    markUnreachable();
    enqueue(aSet(1));
    render(OfflineBanner);

    expect(screen.getByRole("status")).toHaveTextContent(/1 change saved here/);
  });
});

// The moment a lifter might close the tab believing everything is saved.
it("reports a queue still draining after the network is back", () => {
  enqueue(aSet(1));
  markReachable();
  render(OfflineBanner);

  expect(screen.getByRole("status")).toHaveTextContent(/Syncing 1 change/);
});

describe("refused writes", () => {
  async function refuseOne() {
    enqueue(aSet(1));
    updateSessionSet.mockResolvedValue({
      data: undefined,
      error: { message: "no" },
      response: new Response("", { status: 409 }),
    });
    await flush();
  }

  // Not transient and not retried, so it needs saying and then dismissing —
  // unlike the queue, which clears itself.
  it("says so, as an alert", async () => {
    await refuseOne();
    render(OfflineBanner);

    expect(screen.getByRole("alert")).toHaveTextContent(/1 change couldn't be saved/);
  });

  it("can be dismissed", async () => {
    await refuseOne();
    render(OfflineBanner);

    await fireEvent.click(screen.getByRole("button", { name: "Dismiss" }));

    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
});
