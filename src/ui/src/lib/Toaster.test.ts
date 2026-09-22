import { render, screen, fireEvent, waitFor } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import Toaster from "./Toaster.svelte";
import { dismissToast, pushToast, resetToasts, toasts, updateToast } from "./toast.svelte";

// Transient notices. Two things are worth defending here and neither is cosmetic.
//
// A pending notice must not expire on its own: it stands for a request that is
// still open, and one that timed out would tell the reader the work had finished.
//
// And the cap must drop resolved notices before pending ones, because the case
// where it bites is a live loop ticking away underneath a backfill — the burst
// would otherwise evict the very notice the reader is waiting on.

beforeEach(() => {
  resetToasts();
});

afterEach(() => {
  vi.useRealTimers();
  resetToasts();
});

describe("the toast store", () => {
  it("holds a notice until it expires", async () => {
    vi.useFakeTimers();
    pushToast({ title: "History generated" });
    expect(toasts()).toHaveLength(1);

    await vi.advanceTimersByTimeAsync(6_000);
    expect(toasts()).toHaveLength(0);
  });

  // The flag exists for exactly this. A synchronous backfill takes as long as it
  // takes, and a notice that vanished mid-generation would read as "finished".
  it("never expires a pending notice", async () => {
    vi.useFakeTimers();
    pushToast({ title: "Generating history…", pending: true });

    await vi.advanceTimersByTimeAsync(60_000);
    expect(toasts()).toHaveLength(1);
  });

  it("resolves a pending notice in place rather than raising a second one", async () => {
    vi.useFakeTimers();
    const id = pushToast({ title: "Generating history…", pending: true });

    updateToast(id, { title: "History generated", body: "137 sessions." });

    expect(toasts()).toHaveLength(1);
    expect(toasts()[0].title).toBe("History generated");
    expect(toasts()[0].pending).toBe(false);
    // And now it is on the clock, which it was not a moment ago.
    await vi.advanceTimersByTimeAsync(6_000);
    expect(toasts()).toHaveLength(0);
  });

  // A caller resolving a notice the reader already dismissed is correct
  // behaviour, not an error — so it must not come back.
  it("does not resurrect a notice that is already gone", () => {
    const id = pushToast({ title: "Generating history…", pending: true });
    dismissToast(id);

    updateToast(id, { title: "History generated" });

    expect(toasts()).toHaveLength(0);
  });

  it("caps the stack, dropping resolved notices before pending ones", () => {
    const pending = pushToast({ title: "Generating history…", pending: true });
    for (const n of [1, 2, 3, 4]) pushToast({ title: `Action ${n}` });

    const titles = toasts().map((t) => t.title);
    expect(toasts()).toHaveLength(4);
    // The one still waiting on a request survived; the oldest finished one went.
    expect(toasts().some((t) => t.id === pending)).toBe(true);
    expect(titles).not.toContain("Action 1");
    expect(titles).toContain("Action 4");
  });
});

describe("Toaster", () => {
  it("renders a notice and its detail", async () => {
    render(Toaster);
    pushToast({ title: "History generated", body: "4 new lifters, 137 sessions." });

    await waitFor(() => {
      expect(screen.getByText("History generated")).toBeInTheDocument();
    });
    expect(screen.getByText("4 new lifters, 137 sessions.")).toBeInTheDocument();
  });

  it("can be dismissed by hand", async () => {
    render(Toaster);
    pushToast({ title: "Live activity stopped" });
    await waitFor(() => expect(screen.getByText("Live activity stopped")).toBeInTheDocument());

    await fireEvent.click(screen.getByRole("button", { name: "Dismiss" }));

    await waitFor(() => {
      expect(screen.queryByText("Live activity stopped")).not.toBeInTheDocument();
    });
  });

  // Nothing to dismiss yet, and a reader who took it down would be left with a
  // screen that looks idle while the request is still open.
  it("offers no dismiss on a pending notice", async () => {
    render(Toaster);
    pushToast({ title: "Generating history…", pending: true });

    await waitFor(() => expect(screen.getByText("Generating history…")).toBeInTheDocument());
    expect(screen.queryByRole("button", { name: "Dismiss" })).not.toBeInTheDocument();
  });

  // One live region holding all of them, so a screen reader hears the notices in
  // order rather than a fresh region each time.
  it("announces politely from a single live region", () => {
    render(Toaster);
    const region = screen.getByTestId("toaster");
    expect(region).toHaveAttribute("aria-live", "polite");
    expect(region).toHaveAttribute("role", "status");
  });
});
