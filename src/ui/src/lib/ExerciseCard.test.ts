import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/svelte";
import ExerciseCard from "./ExerciseCard.svelte";
import { auth } from "./auth.svelte";
import type { SessionSet } from "./api";
import { testUser } from "./testFixtures";

// The bar and the rack come off the profile, so these sign in as somebody with
// a gym: an 80 lb bar and the standard rack, which is what this file's
// arithmetic assumes. A work weight of 80 is therefore just the bar, and
// produces no warm-ups.
const lifter = testUser({
  barWeightLb: 80,
  plates: [
    { plateLb: 45, pairs: 2 },
    { plateLb: 35, pairs: 2 },
    { plateLb: 25, pairs: 2 },
    { plateLb: 10, pairs: 2 },
    { plateLb: 5, pairs: 2 },
    { plateLb: 2.5, pairs: 2 },
  ],
});

beforeEach(() => {
  auth.me = { ...lifter };
  auth.loaded = true;
});

afterEach(() => {
  auth.me = null;
  auth.loaded = false;
});

// The generated SessionSet has more fields than the card reads; build the subset
// it uses and cast.
type SetFixture = {
  id: number;
  setNumber: number;
  weightLb: number;
  targetReps: number;
  actualReps: number | null;
  completed: boolean;
  restSeconds?: number;
};
function set(p: SetFixture): SessionSet {
  return p as unknown as SessionSet;
}
function workSets(
  weightLb: number,
  count: number,
  restSeconds?: number,
): SessionSet[] {
  return Array.from({ length: count }, (_, i) =>
    set({
      id: i + 1,
      setNumber: i + 1,
      weightLb,
      targetReps: 5,
      actualReps: null,
      completed: false,
      restSeconds,
    }),
  );
}

describe("ExerciseCard", () => {
  it("shows the exercise name, target reps and work weight", () => {
    render(ExerciseCard, {
      name: "Squat",
      sets: workSets(80, 3),
      onCycle: vi.fn(),
      onChangeWeight: vi.fn(),
    });
    expect(screen.getByRole("heading", { name: "Squat" })).toBeInTheDocument();
    expect(screen.getByText("5 reps")).toBeInTheDocument();
    expect(screen.getByText("80 lb", { exact: true })).toBeInTheDocument();
  });

  // Rest is half the prescription, and the countdown that enforces it lives in
  // an unlabelled corner of the screen — so the number belongs next to the reps.
  it("shows the lift's prescribed rest alongside the rep target", () => {
    render(ExerciseCard, {
      name: "Deadlift",
      sets: workSets(80, 1, 180),
      onCycle: vi.fn(),
      onChangeWeight: vi.fn(),
    });
    expect(screen.getByText(/5 reps · 3:00 rest/)).toBeInTheDocument();
  });

  it("adjusts weight by ±5 via the stepper buttons", async () => {
    const onChangeWeight = vi.fn();
    render(ExerciseCard, {
      name: "Squat",
      sets: workSets(80, 1),
      onCycle: vi.fn(),
      onChangeWeight,
    });

    await fireEvent.click(screen.getByRole("button", { name: "Increase weight by 5 lb" }));
    expect(onChangeWeight).toHaveBeenCalledWith(5);

    await fireEvent.click(screen.getByRole("button", { name: "Decrease weight by 5 lb" }));
    expect(onChangeWeight).toHaveBeenCalledWith(-5);
  });

  it("renders one circle per work set and cycles it on tap", async () => {
    const onCycle = vi.fn();
    const sets = workSets(80, 3);
    render(ExerciseCard, { name: "Squat", sets, onCycle, onChangeWeight: vi.fn() });

    expect(screen.getByRole("button", { name: "Set 1: not logged" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Set 3: not logged" })).toBeInTheDocument();

    await fireEvent.click(screen.getByRole("button", { name: "Set 2: not logged" }));
    expect(onCycle).toHaveBeenCalledWith(expect.objectContaining({ id: 2, setNumber: 2 }));
  });

  it("omits the warm-up ramp when the work weight is just the bar", () => {
    render(ExerciseCard, {
      name: "Squat",
      sets: workSets(80, 3),
      onCycle: vi.fn(),
      onChangeWeight: vi.fn(),
    });
    expect(screen.queryAllByRole("button", { name: /^Warm-up/ })).toHaveLength(0);
  });

  it("shows warm-up circles for a heavier lift and counts reps on tap", async () => {
    const { container } = render(ExerciseCard, {
      name: "Squat",
      sets: workSets(200, 5),
      onCycle: vi.fn(),
      onChangeWeight: vi.fn(),
    });

    // warmupSets(200) → bar×2, then 100 / 140 / 180 = 5 expanded warm-up steps,
    // which a 5x5 has room for.
    const warmups = container.querySelectorAll<HTMLButtonElement>('button[aria-label^="Warm-up"]');
    expect(warmups).toHaveLength(5);

    const first = warmups[0];
    expect(first).toHaveTextContent("0");
    await fireEvent.click(first);
    expect(first).toHaveTextContent("1"); // reps count up locally
  });

  it("never shows more warm-up circles than the day has work sets", () => {
    // A 2x5 on the Lite program. The full ramp for 200 is five sets, which
    // would leave the lifter doing more warming up than lifting.
    const { container } = render(ExerciseCard, {
      name: "Squat",
      sets: workSets(200, 2),
      onCycle: vi.fn(),
      onChangeWeight: vi.fn(),
    });

    const warmups = container.querySelectorAll<HTMLButtonElement>('button[aria-label^="Warm-up"]');
    expect(warmups).toHaveLength(2);
    // The two kept are the heaviest, the ones that actually prepare the lift.
    expect(warmups[0]).toHaveAttribute("aria-label", expect.stringContaining("Warm-up 140 lb"));
    expect(warmups[1]).toHaveAttribute("aria-label", expect.stringContaining("Warm-up 180 lb"));
  });

  // The cap is the count of work sets, and sets get added at the rack. Before
  // the ramp is tapped through, more sets means more room for it; after, the
  // lifter is warm and a new rung is a rung they'd have to go back and do.
  describe("when a set is added mid-session", () => {
    const ramp = (c: HTMLElement) =>
      Array.from(
        c.querySelectorAll<HTMLButtonElement>('button[aria-label^="Warm-up"]'),
      );

    it("makes room for another rung while the ramp is unfinished", async () => {
      const { container, rerender } = render(ExerciseCard, {
        name: "Squat",
        sets: workSets(200, 2),
        onCycle: vi.fn(),
        onChangeWeight: vi.fn(),
      });
      expect(ramp(container)).toHaveLength(2);

      await rerender({ sets: workSets(200, 3) });
      expect(ramp(container)).toHaveLength(3);
      expect(ramp(container)[0]).toHaveAttribute(
        "aria-label",
        expect.stringContaining("Warm-up 100 lb"),
      );
    });

    it("leaves the ramp alone once it is complete", async () => {
      const { container, rerender } = render(ExerciseCard, {
        name: "Squat",
        sets: workSets(200, 2),
        onCycle: vi.fn(),
        onChangeWeight: vi.fn(),
      });

      // Tap both rungs out to their targets: 140x3 and 180x2.
      for (const [i, reps] of [3, 2].entries()) {
        for (let k = 0; k < reps; k++) await fireEvent.click(ramp(container)[i]);
      }
      expect(ramp(container).map((b) => b.textContent?.trim())).toEqual(["3", "2"]);

      await rerender({ sets: workSets(200, 3) });

      // Still two rungs, still the heavy ones, and the reps still belong to the
      // sets they were logged against.
      expect(ramp(container)).toHaveLength(2);
      expect(ramp(container)[0]).toHaveAttribute(
        "aria-label",
        expect.stringContaining("Warm-up 140 lb"),
      );
      expect(ramp(container).map((b) => b.textContent?.trim())).toEqual(["3", "2"]);
    });
  });

  // A session is a stack of these, and a lifter is in the middle of one lift at
  // a time: a squat finished twenty minutes ago is a result rather than a
  // control, and at full height it pushes the lift being done off the screen.
  describe("folding a finished lift away", () => {
    // Every set tapped out to its target, which is what `completed` means.
    const allDone = (sets: SessionSet[]) =>
      sets.map((s) => set({ ...s, actualReps: 5, completed: true }));

    it("folds the card once every set is complete", async () => {
      const sets = workSets(80, 3);
      const { rerender } = render(ExerciseCard, {
        name: "Squat",
        sets,
        onCycle: vi.fn(),
        onChangeWeight: vi.fn(),
      });
      expect(screen.getByRole("button", { name: "Set 1: not logged" })).toBeInTheDocument();

      await rerender({ sets: allDone(sets) });

      await waitFor(() =>
        expect(screen.queryByRole("button", { name: /^Set 1/ })).not.toBeInTheDocument(),
      );
      // The header still says what the lift was and what it carried.
      expect(screen.getByRole("heading", { name: "Squat" })).toBeInTheDocument();
      expect(screen.getByText("3/3 sets · 80 lb")).toBeInTheDocument();
    });

    it("leaves a lift with a set still to go alone", async () => {
      const sets = workSets(80, 3);
      sets[0] = set({ ...sets[0], actualReps: 5, completed: true });
      render(ExerciseCard, {
        name: "Squat",
        sets,
        onCycle: vi.fn(),
        onChangeWeight: vi.fn(),
      });
      await waitFor(() =>
        expect(screen.getByRole("button", { name: "Set 2: not logged" })).toBeInTheDocument(),
      );
    });

    it("unfolds again on a tap, because done is not finished with", async () => {
      const onCycle = vi.fn();
      const sets = allDone(workSets(80, 2));
      render(ExerciseCard, { name: "Squat", sets, onCycle, onChangeWeight: vi.fn() });

      await waitFor(() =>
        expect(screen.queryByRole("button", { name: /^Set 1/ })).not.toBeInTheDocument(),
      );

      await fireEvent.click(screen.getByRole("button", { name: "Squat" }));

      // The circles are back, and tapping one still clears a mis-tapped set.
      const first = screen.getByRole("button", { name: "Set 1: 5 reps" });
      await fireEvent.click(first);
      expect(onCycle).toHaveBeenCalledWith(expect.objectContaining({ setNumber: 1 }));
    });

    it("stays open after the lifter opens it by hand", async () => {
      const sets = allDone(workSets(80, 2));
      const { rerender } = render(ExerciseCard, {
        name: "Squat",
        sets,
        onCycle: vi.fn(),
        onChangeWeight: vi.fn(),
      });
      await waitFor(() =>
        expect(screen.queryByRole("button", { name: /^Set 1/ })).not.toBeInTheDocument(),
      );
      await fireEvent.click(screen.getByRole("button", { name: "Squat" }));

      // A weight nudge on the open card re-renders it; nothing re-folds it.
      await rerender({ sets: allDone(workSets(85, 2)) });
      expect(screen.getByRole("button", { name: /^Set 1/ })).toBeInTheDocument();
    });

    it("opens itself when a set stops being complete", async () => {
      const sets = workSets(80, 2);
      const { rerender } = render(ExerciseCard, {
        name: "Squat",
        sets: allDone(sets),
        onCycle: vi.fn(),
        onChangeWeight: vi.fn(),
      });
      await waitFor(() =>
        expect(screen.queryByRole("button", { name: /^Set 1/ })).not.toBeInTheDocument(),
      );

      // A set cleared, or one added at the rack: there is something to tap again.
      await rerender({ sets });
      await waitFor(() =>
        expect(screen.getByRole("button", { name: "Set 1: not logged" })).toBeInTheDocument(),
      );
    });

    it("can be folded by hand before the lift is done", async () => {
      const sets = workSets(80, 3);
      sets[0] = set({ ...sets[0], actualReps: 5, completed: true });
      render(ExerciseCard, {
        name: "Squat",
        sets,
        onCycle: vi.fn(),
        onChangeWeight: vi.fn(),
      });

      await fireEvent.click(screen.getByRole("button", { name: "Squat" }));
      expect(screen.queryByRole("button", { name: /^Set 1/ })).not.toBeInTheDocument();
      expect(screen.getByText("1/3 sets · 80 lb")).toBeInTheDocument();
    });

    it("leaves a finished session's cards open — it is the record", async () => {
      // Every lift in an over session is complete, so folding on load would
      // leave a screen of headings where the set-by-set detail should be.
      render(ExerciseCard, {
        name: "Squat",
        sets: allDone(workSets(80, 2)),
        onCycle: vi.fn(),
        onChangeWeight: vi.fn(),
        readonly: true,
      });
      await waitFor(() =>
        expect(screen.getByRole("button", { name: "Set 1: 5 reps" })).toBeInTheDocument(),
      );
    });
  });

  // Everything this card said about loading a weight used to assume a barbell.
  // On a pair of dumbbells all of it was fiction: a lifter curling 30 lb was
  // told to open with two sets of an 80 lb bar, shown a diagram of the plates to
  // hang off it, and given a stepper that built a 35 lb pair out of 5 lb bells.
  describe("on a lift with no bar", () => {
    it("omits the barbell diagram and the per-side plate line", () => {
      render(ExerciseCard, {
        name: "Dumbbell Curl",
        sets: workSets(30, 3),
        equipment: "dumbbell",
        onCycle: vi.fn(),
        onChangeWeight: vi.fn(),
      });
      expect(screen.queryByLabelText(/^Barbell loaded to/)).not.toBeInTheDocument();
      expect(screen.queryByText(/\/ side/)).not.toBeInTheDocument();
      expect(screen.queryByText(/bar only/)).not.toBeInTheDocument();
    });

    it("says what one bell weighs, since the prescription is the pair", () => {
      render(ExerciseCard, {
        name: "Dumbbell Curl",
        sets: workSets(30, 3),
        equipment: "dumbbell",
        onCycle: vi.fn(),
        onChangeWeight: vi.fn(),
      });
      // The ramp opens at 10, so that is the active step: 5 lb in each hand.
      expect(screen.getByText("10 lb × 5 · 5 lb per hand")).toBeInTheDocument();
    });

    // ...and says which weight the header is, because the two numbers sit a line
    // apart in different units. A lifter reading "10 lb per hand" against a bare
    // "40 lb" concludes their warm-up is 25% of the work weight, when it is the
    // 50% rung of a 20 lb bell.
    it("labels the work weight as the pair, since the rung under it is per hand", async () => {
      const sets = workSets(40, 3);
      const { rerender } = render(ExerciseCard, {
        name: "Dumbbell Curl",
        sets,
        equipment: "dumbbell",
        onCycle: vi.fn(),
        onChangeWeight: vi.fn(),
      });
      expect(screen.getByText("40 lb pair", { exact: true })).toBeInTheDocument();
      expect(screen.getByText("20 lb × 5 · 10 lb per hand")).toBeInTheDocument();

      // The folded card is the same weight with the circles gone, so it carries
      // the same label rather than reverting to the ambiguous one.
      await rerender({
        sets: sets.map((s) => set({ ...s, actualReps: 10, completed: true })),
      });
      await waitFor(() =>
        expect(screen.getByText("3/3 sets · 40 lb pair")).toBeInTheDocument(),
      );
    });

    it("leaves a barbell's weight unlabelled, where nothing is ambiguous", () => {
      // Every weight on a barbell card is the whole load, so a suffix there is
      // noise on every lift in the session to disambiguate nothing.
      render(ExerciseCard, {
        name: "Squat",
        sets: workSets(200, 3),
        onCycle: vi.fn(),
        onChangeWeight: vi.fn(),
      });
      expect(screen.getByText("200 lb", { exact: true })).toBeInTheDocument();
      expect(screen.queryByText(/lb pair/)).not.toBeInTheDocument();
    });

    it("warms up without the empty bar", () => {
      const { container } = render(ExerciseCard, {
        name: "Dumbbell Curl",
        sets: workSets(30, 3),
        equipment: "dumbbell",
        onCycle: vi.fn(),
        onChangeWeight: vi.fn(),
      });

      const warmups = container.querySelectorAll<HTMLButtonElement>(
        'button[aria-label^="Warm-up"]',
      );
      expect(warmups).toHaveLength(2);
      expect(warmups[0]).toHaveAttribute("aria-label", expect.stringContaining("Warm-up 10 lb"));
      expect(warmups[1]).toHaveAttribute("aria-label", expect.stringContaining("Warm-up 20 lb"));
    });

    it("steps the weight by 10, which is what a pair of bells can do", async () => {
      const onChangeWeight = vi.fn();
      render(ExerciseCard, {
        name: "Dumbbell Curl",
        sets: workSets(30, 3),
        equipment: "dumbbell",
        onCycle: vi.fn(),
        onChangeWeight,
      });

      await fireEvent.click(
        screen.getByRole("button", { name: "Increase weight by 10 lb" }),
      );
      expect(onChangeWeight).toHaveBeenCalledWith(10);

      await fireEvent.click(
        screen.getByRole("button", { name: "Decrease weight by 10 lb" }),
      );
      expect(onChangeWeight).toHaveBeenCalledWith(-10);
    });

    it("leaves a machine's weight to speak for itself", () => {
      // No plates to list and no bell to halve — the stack is its own label, and
      // a second line restating the weight above it is noise.
      render(ExerciseCard, {
        name: "Leg Press",
        sets: workSets(90, 3),
        equipment: "machine",
        onCycle: vi.fn(),
        onChangeWeight: vi.fn(),
      });
      expect(screen.queryByLabelText(/^Barbell loaded to/)).not.toBeInTheDocument();
      expect(screen.getByText("45 lb × 5")).toBeInTheDocument();
    });
  });

  describe("when readonly (the session is over)", () => {
    it("ignores taps on work sets and the weight steppers", async () => {
      const onCycle = vi.fn();
      const onChangeWeight = vi.fn();
      render(ExerciseCard, {
        name: "Squat",
        sets: workSets(80, 3),
        onCycle,
        onChangeWeight,
        readonly: true,
      });

      await fireEvent.click(screen.getByRole("button", { name: "Set 1: not logged" }));
      await fireEvent.click(
        screen.getByRole("button", { name: "Increase weight by 5 lb" }),
      );
      await fireEvent.click(
        screen.getByRole("button", { name: "Decrease weight by 5 lb" }),
      );

      expect(onCycle).not.toHaveBeenCalled();
      expect(onChangeWeight).not.toHaveBeenCalled();
    });

    it("disables the warm-up circles so their local reps can't move", async () => {
      const { container } = render(ExerciseCard, {
        name: "Squat",
        sets: workSets(200, 1),
        onCycle: vi.fn(),
        onChangeWeight: vi.fn(),
        readonly: true,
      });

      const first = container.querySelector<HTMLButtonElement>(
        'button[aria-label^="Warm-up"]',
      )!;
      expect(first).toBeDisabled();
      await fireEvent.click(first);
      expect(first).toHaveTextContent("0");
    });

    it("explains that the sets are locked instead of how to tap them", () => {
      render(ExerciseCard, {
        name: "Squat",
        sets: workSets(80, 3),
        onCycle: vi.fn(),
        onChangeWeight: vi.fn(),
        readonly: true,
      });
      expect(
        screen.getByText(/This workout is finished — sets are locked\./),
      ).toBeInTheDocument();
      expect(screen.queryByText(/Tap a set to add a rep/)).not.toBeInTheDocument();
    });

    it("hides the add and remove controls", () => {
      render(ExerciseCard, {
        name: "Squat",
        sets: workSets(80, 3),
        onCycle: vi.fn(),
        onChangeWeight: vi.fn(),
        onAddSet: vi.fn(),
        onRemoveSet: vi.fn(),
        readonly: true,
      });
      expect(
        screen.queryByRole("button", { name: "Add a set of Squat" }),
      ).not.toBeInTheDocument();
      expect(
        screen.queryByRole("button", { name: /^Remove set/ }),
      ).not.toBeInTheDocument();
    });
  });

  // The prescription is a plan, not a cage: an extra set, an AMRAP, or a set
  // skipped all happen, and until these existed the closest a lifter could get
  // was a ghost row logged at zero reps.
  describe("adding and removing sets", () => {
    it("adds a set", async () => {
      const onAddSet = vi.fn();
      render(ExerciseCard, {
        name: "Squat",
        sets: workSets(80, 3),
        onCycle: vi.fn(),
        onChangeWeight: vi.fn(),
        onAddSet,
      });
      await fireEvent.click(
        screen.getByRole("button", { name: "Add a set of Squat" }),
      );
      expect(onAddSet).toHaveBeenCalledOnce();
    });

    it("removes an unlogged set without asking", async () => {
      // A dialog here is a dialog in the way of somebody between sets: dropping
      // a set nobody touched is the same gesture as never having had it.
      const onRemoveSet = vi.fn();
      render(ExerciseCard, {
        name: "Squat",
        sets: workSets(80, 3),
        onCycle: vi.fn(),
        onChangeWeight: vi.fn(),
        onRemoveSet,
      });
      await fireEvent.click(
        screen.getByRole("button", { name: "Remove set 3 of Squat" }),
      );
      expect(onRemoveSet).toHaveBeenCalledWith(
        expect.objectContaining({ setNumber: 3 }),
      );
    });

    it("confirms before throwing away logged reps", async () => {
      const onRemoveSet = vi.fn();
      const sets = workSets(80, 2);
      sets[1] = set({ ...sets[1], actualReps: 5, completed: true });
      render(ExerciseCard, {
        name: "Squat",
        sets,
        onCycle: vi.fn(),
        onChangeWeight: vi.fn(),
        onRemoveSet,
      });

      await fireEvent.click(
        screen.getByRole("button", { name: "Remove set 2 of Squat" }),
      );
      expect(onRemoveSet).not.toHaveBeenCalled();
      expect(screen.getByText(/Remove this set\?/)).toBeInTheDocument();

      await fireEvent.click(screen.getByRole("button", { name: "Remove" }));
      expect(onRemoveSet).toHaveBeenCalledWith(
        expect.objectContaining({ setNumber: 2 }),
      );
    });

    it("leaves the set alone when the confirmation is dismissed", async () => {
      const onRemoveSet = vi.fn();
      const sets = workSets(80, 2);
      sets[1] = set({ ...sets[1], actualReps: 5, completed: true });
      render(ExerciseCard, {
        name: "Squat",
        sets,
        onCycle: vi.fn(),
        onChangeWeight: vi.fn(),
        onRemoveSet,
      });

      await fireEvent.click(
        screen.getByRole("button", { name: "Remove set 2 of Squat" }),
      );
      await fireEvent.click(screen.getByRole("button", { name: "Keep it" }));
      expect(onRemoveSet).not.toHaveBeenCalled();
    });

    it("shows no controls at all when the handlers aren't wired", () => {
      render(ExerciseCard, {
        name: "Squat",
        sets: workSets(80, 3),
        onCycle: vi.fn(),
        onChangeWeight: vi.fn(),
      });
      expect(
        screen.queryByRole("button", { name: /^Add a set/ }),
      ).not.toBeInTheDocument();
      expect(
        screen.queryByRole("button", { name: /^Remove set/ }),
      ).not.toBeInTheDocument();
    });
  });

  // Madcow climbs 50/62.5/75/87.5/100% of a top set, and its intensity day
  // finishes with a triple above that and a backoff below it. A card that
  // assumed one weight and one rep target across a lift could not show any of
  // that — it read sets[0] and called it the prescription.
  describe("a ramping lift", () => {
    const ramp = [
      set({ id: 1, setNumber: 1, weightLb: 100, targetReps: 5, actualReps: null, completed: false }),
      set({ id: 2, setNumber: 2, weightLb: 125, targetReps: 5, actualReps: null, completed: false }),
      set({ id: 3, setNumber: 3, weightLb: 150, targetReps: 5, actualReps: null, completed: false }),
      set({ id: 4, setNumber: 4, weightLb: 175, targetReps: 5, actualReps: null, completed: false }),
      set({ id: 5, setNumber: 5, weightLb: 205, targetReps: 3, actualReps: null, completed: false }),
      set({ id: 6, setNumber: 6, weightLb: 150, targetReps: 8, actualReps: null, completed: false }),
    ];

    it("shows the top set rather than the first one", () => {
      render(ExerciseCard, {
        name: "Squat",
        sets: ramp,
        onCycle: vi.fn(),
        onChangeWeight: vi.fn(),
      });
      // 205 is the heaviest set; 100 is merely the one it opens on.
      expect(screen.getByText("205 lb", { exact: true })).toBeInTheDocument();
    });

    it("says it ramps instead of naming one rep target", () => {
      render(ExerciseCard, {
        name: "Squat",
        sets: ramp,
        onCycle: vi.fn(),
        onChangeWeight: vi.fn(),
      });
      expect(screen.getByText(/6 sets, ramping/)).toBeInTheDocument();
    });

    it("writes the whole climb out, weight by weight", () => {
      render(ExerciseCard, {
        name: "Squat",
        sets: ramp,
        onCycle: vi.fn(),
        onChangeWeight: vi.fn(),
      });
      expect(screen.getByText("100×5")).toBeInTheDocument();
      expect(screen.getByText("205×3")).toBeInTheDocument();
      expect(screen.getByText("150×8")).toBeInTheDocument();
    });

    it("carries each set's own weight into its label", () => {
      render(ExerciseCard, {
        name: "Squat",
        sets: ramp,
        onCycle: vi.fn(),
        onChangeWeight: vi.fn(),
      });
      expect(
        screen.getByRole("button", { name: /^Set 5, 205 lb for 3/ }),
      ).toBeInTheDocument();
    });

    it("adds no warm-up ramp in front of a ramp", () => {
      // The first three rungs ARE the warm-up; a second one would have the
      // lifter warming up to warm up.
      render(ExerciseCard, {
        name: "Squat",
        sets: ramp,
        onCycle: vi.fn(),
        onChangeWeight: vi.fn(),
      });
      expect(screen.queryAllByRole("button", { name: /^Warm-up/ })).toHaveLength(0);
    });

    it("still warms up a uniform block", () => {
      render(ExerciseCard, {
        name: "Squat",
        sets: workSets(200, 5),
        onCycle: vi.fn(),
        onChangeWeight: vi.fn(),
      });
      expect(
        screen.queryAllByRole("button", { name: /^Warm-up/ }).length,
      ).toBeGreaterThan(0);
    });
  });
});
