import { describe, it, expect, vi, beforeEach, afterEach, afterAll } from "vitest";
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
  version.notes = { version: "", entries: [] };
  resetPendingWrites();
});

afterEach(() => {
  vi.useRealTimers();
});

// bits-ui's body-scroll-lock resets the document's styles on a TIMER scheduled
// when the dialog unmounts. The last test's cleanup leaves one pending with
// nothing after it, and if vitest tears the jsdom environment down first it
// fires into a world with no `document` — which fails the whole run on an
// unhandled error while every test still reports as passing, because none of
// them did anything wrong.
//
// It is a race, so it turns on machine speed: the sandbox wins it and CI, being
// slower, does not. Waiting here removes it rather than making it rarer.
// Deliberately afterAll and not afterEach: unmounting by hand between tests
// would close a dialog that bits-ui then complains about being read during
// teardown, trading this for a pile of derived_inert warnings.
afterAll(async () => {
  await new Promise((resolve) => setTimeout(resolve, 100));
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

  // What's actually in the build being offered, so "Load it?" is answerable. The
  // notes are fetched from the deployed bundle (version.svelte.ts owns that);
  // here they're set directly, the same way the versions are.
  describe("release notes", () => {
    const notes = (v: string, entries: string[]) => {
      version.notes = { version: v, entries };
    };

    it("lists what shipped in the offered build", async () => {
      deployed("v1.2.3", "v1.3.0");
      notes("v1.3.0", ["feat(ui): list programs (abc1234)", "fix(api): empty day (def5678)"]);
      render(UpdatePrompt);

      await waitFor(() => expect(dialog()).toBeInTheDocument());
      expect(screen.getByRole("heading", { name: /What's new in v1\.3\.0/ })).toBeInTheDocument();
      expect(dialog()).toHaveTextContent("feat(ui): list programs (abc1234)");
      expect(dialog()).toHaveTextContent("fix(api): empty day (def5678)");
    });

    // The whole point of the version field in changelog.json: during a rollout
    // the pods can still be serving the PREVIOUS build's notes, and captioning
    // those with the new version would misdescribe what's being offered.
    it("says nothing about the release when the notes are for another one", async () => {
      deployed("v1.2.3", "v1.3.0");
      notes("v1.2.3", ["feat(ui): this shipped last time (abc1234)"]);
      render(UpdatePrompt);

      await waitFor(() => expect(dialog()).toBeInTheDocument());
      expect(dialog()).not.toHaveTextContent("What's new");
      expect(dialog()).not.toHaveTextContent("this shipped last time");
    });

    // Not awaited by poll(), so the dialog can be up before they land — and a
    // rejected fetch retries, which can be minutes later with it still open.
    it("picks up notes that arrive after it is already open", async () => {
      deployed("v1.2.3", "v1.3.0");
      render(UpdatePrompt);

      await waitFor(() => expect(dialog()).toBeInTheDocument());
      expect(dialog()).not.toHaveTextContent("What's new");

      notes("v1.3.0", ["feat(ui): arrived late (abc1234)"]);
      await waitFor(() => expect(dialog()).toHaveTextContent("feat(ui): arrived late (abc1234)"));
    });

    // The notes are decoration on a dialog whose job is the reload. With none —
    // an unreachable changelog.json, or a pod mid-roll — it has to be exactly the
    // dialog it was before they existed.
    it("is unchanged when there are none", async () => {
      deployed("v1.2.3", "v1.3.0");
      render(UpdatePrompt);

      await waitFor(() => expect(dialog()).toBeInTheDocument());
      expect(dialog()).not.toHaveTextContent("What's new");
      expect(dialog()).toHaveTextContent("v1.3.0");
      expect(dialog()).toHaveTextContent(/every set you've logged is already saved/i);
      expect(loadButton()).toBeInTheDocument();
    });
  });
});
