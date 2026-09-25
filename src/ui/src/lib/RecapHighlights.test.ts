import { describe, it, expect } from "vitest";
import { render, screen, fireEvent } from "@testing-library/svelte";
import RecapHighlights from "./RecapHighlights.svelte";
import type { RecapPRRow } from "./recap";

// The card that answers "was any of that worth anything". It had no test of its
// own until first times were split out of records — it was reached only through
// SessionRecap's route test, which cannot say much about the card itself.

function pr(exerciseName: string, valueLb: number, previousLb: number): RecapPRRow {
  return { exerciseName, kind: "weight", valueLb, previousLb };
}

describe("RecapHighlights", () => {
  it("renders nothing when there is nothing to note", () => {
    const { container } = render(RecapHighlights, { props: { prs: [] } });
    expect(container.querySelector("[data-testid='recap-highlights']")).toBeNull();
  });

  // A first workout has no records at all. Before the split it had one per lift,
  // and the card was the loudest place that happened.
  it("folds every first time into a single line", () => {
    render(RecapHighlights, {
      props: {
        prs: [],
        firstTimes: [
          { exerciseName: "Overhead Press" },
          { exerciseName: "Barbell Row" },
          { exerciseName: "Chin-up" },
        ],
      },
    });

    // One list item, not three: the count and the names on one row.
    const items = screen.getAllByRole("listitem");
    expect(items).toHaveLength(1);
    expect(items[0].textContent).toMatch(/3\s*lifts for the first time/);
    expect(items[0].textContent).toContain("Overhead Press");
    expect(items[0].textContent).toContain("Chin-up");
  });

  // Singular, because "1 lifts for the first time" is the kind of thing a lifter
  // notices on the screen they were meant to enjoy.
  it("says lift rather than lifts for a single one", () => {
    render(RecapHighlights, {
      props: { prs: [], firstTimes: [{ exerciseName: "Chin-up" }] },
    });
    expect(screen.getByRole("listitem").textContent).toMatch(/1\s*lift for the first time/);
  });

  // The card opens on first times alone: a lifter whose whole session was new
  // lifts still has something worth reading.
  it("shows the card for first times with no records", () => {
    const { container } = render(RecapHighlights, {
      props: { prs: [], firstTimes: [{ exerciseName: "Squat" }] },
    });
    expect(container.querySelector("[data-testid='recap-highlights']")).not.toBeNull();
  });

  // A record and a first time in one session are two different rows, and only
  // the record claims to have beaten anything.
  it("keeps records and first times apart", () => {
    render(RecapHighlights, {
      props: {
        prs: [pr("Squat", 225, 215)],
        firstTimes: [{ exerciseName: "Barbell Row" }],
      },
    });

    const rows = screen.getAllByRole("listitem").map((li) => li.textContent ?? "");
    const record = rows.find((t) => t.includes("Squat"))!;
    const first = rows.find((t) => t.includes("Barbell Row"))!;

    expect(record).toContain("was 215");
    expect(first).not.toMatch(/was /);
    expect(first).not.toContain("PR");
  });

  it("explains how records are decided when the help affordance is opened", async () => {
    render(RecapHighlights, { props: { prs: [pr("Squat", 225, 215)] } });

    // Nothing is said until asked — the card is the reward, not a manual.
    expect(screen.queryByText(/beats your own previous best/)).toBeNull();

    // By accessible name: an icon-only button with no label is the failure mode
    // this affordance is most likely to ship with.
    await fireEvent.click(
      screen.getByRole("button", { name: /how records and milestones work/i }),
    );

    // The three things a lifter would otherwise guess at: what a record is
    // measured against, that a first time is not one, and that milestones are
    // once-only.
    expect(await screen.findByText(/beats your own previous best/)).toBeTruthy();
    expect(screen.getByText(/first time on a lift isn't a record/)).toBeTruthy();
    expect(screen.getByText(/once per lift, ever/)).toBeTruthy();
  });
});
