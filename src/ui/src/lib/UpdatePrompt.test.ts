import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/svelte";
import UpdatePrompt from "./UpdatePrompt.svelte";
import { version } from "./version.svelte";
import { track, resetPendingWrites } from "./pendingWrites.svelte";

// The prompt is driven by the version store, so drive that directly rather than
// stubbing /health — the polling itself has its own spec.
function deployed(running: string, latest: string) {
  version.running = running;
  version.latest = latest;
}

const dialog = () => screen.queryByTestId("update-prompt");
const loadButton = () => screen.getByRole("button", { name: "Load it" });
const notNowButton = () => screen.getByRole("button", { name: "Not now" });

beforeEach(() => {
  version.running = "";
  version.latest = "";
  version.environment = "";
  version.dismissed = "";
  resetPendingWrites();
});

afterEach(() => {
  vi.useRealTimers();
});

describe("UpdatePrompt", () => {
  it("stays out of the way while the running build is current", () => {
    deployed("v1.2.3", "v1.2.3");
    render(UpdatePrompt);

    expect(dialog()).not.toBeInTheDocument();
  });

  it("offers the update once a newer build is deployed", async () => {
    deployed("v1.2.3", "v1.3.0");
    render(UpdatePrompt);

    await waitFor(() => expect(dialog()).toBeInTheDocument());
    // Both versions are named, so it's clear what's being swapped for what.
    expect(dialog()).toHaveTextContent("v1.3.0");
    expect(dialog()).toHaveTextContent("v1.2.3");
  });

  // The point of the whole feature: an interruption mid-workout is only
  // acceptable if it says, truthfully, that nothing is lost.
  it("says the workout is safe", async () => {
    deployed("v1.2.3", "v1.3.0");
    render(UpdatePrompt);

    await waitFor(() => expect(dialog()).toBeInTheDocument());
    expect(dialog()).toHaveTextContent(/every set you've logged is already saved/i);
  });

  it("reloads when the update is accepted", async () => {
    const reload = vi.fn();
    deployed("v1.2.3", "v1.3.0");
    render(UpdatePrompt, { reload });

    await waitFor(() => expect(dialog()).toBeInTheDocument());
    await fireEvent.click(loadButton());

    await waitFor(() => expect(reload).toHaveBeenCalledOnce());
  });

  // Reloading between a set tap and its response would drop that rep — the one
  // thing in the session that isn't on the server yet.
  it("holds the reload until an in-flight write lands", async () => {
    let landed!: () => void;
    void track(new Promise<void>((resolve) => (landed = resolve)));

    const reload = vi.fn();
    deployed("v1.2.3", "v1.3.0");
    render(UpdatePrompt, { reload });

    await waitFor(() => expect(dialog()).toBeInTheDocument());
    await fireEvent.click(loadButton());

    // Still waiting on the write, and saying so.
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Saving…" })).toBeDisabled(),
    );
    expect(reload).not.toHaveBeenCalled();

    landed();
    await waitFor(() => expect(reload).toHaveBeenCalledOnce());
  });

  // A request that never comes back must delay the reload, not cancel it.
  it("reloads anyway if a write never settles", async () => {
    vi.useFakeTimers();
    void track(new Promise<void>(() => {})); // never settles

    const reload = vi.fn();
    deployed("v1.2.3", "v1.3.0");
    render(UpdatePrompt, { reload });

    await vi.advanceTimersByTimeAsync(0);
    await fireEvent.click(loadButton());
    expect(reload).not.toHaveBeenCalled();

    await vi.advanceTimersByTimeAsync(5000);
    expect(reload).toHaveBeenCalledOnce();
  });

  it("closes and stops asking when declined", async () => {
    deployed("v1.2.3", "v1.3.0");
    render(UpdatePrompt);

    await waitFor(() => expect(dialog()).toBeInTheDocument());
    await fireEvent.click(notNowButton());

    await waitFor(() => expect(dialog()).not.toBeInTheDocument());
    expect(version.dismissed).toBe("v1.3.0");
  });

  // Declining is per-version. Silencing every future release would be a worse
  // bug than the nagging it avoids.
  it("comes back when a newer release lands after a decline", async () => {
    deployed("v1.2.3", "v1.3.0");
    render(UpdatePrompt);

    await waitFor(() => expect(dialog()).toBeInTheDocument());
    await fireEvent.click(notNowButton());
    await waitFor(() => expect(dialog()).not.toBeInTheDocument());

    version.latest = "v1.4.0";
    await waitFor(() => expect(dialog()).toBeInTheDocument());
  });
});
