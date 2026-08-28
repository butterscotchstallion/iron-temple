import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/svelte";
import ExerciseCard from "./ExerciseCard.svelte";
import { auth } from "./auth.svelte";
import type { SessionSet, User } from "./api";

// The bar and the rack come off the profile, so these sign in as somebody with
// a gym: an 80 lb bar and the standard rack, which is what this file's
// arithmetic assumes. A work weight of 80 is therefore just the bar, and
// produces no warm-ups.
const lifter = {
  id: 1,
  username: "ada",
  displayName: "Ada",
  avatarColor: "",
  isAdmin: true,
  hasAvatar: false,
  barWeightLb: 80,
  plates: [
    { plateLb: 45, pairs: 2 },
    { plateLb: 35, pairs: 2 },
    { plateLb: 25, pairs: 2 },
    { plateLb: 10, pairs: 2 },
    { plateLb: 5, pairs: 2 },
    { plateLb: 2.5, pairs: 2 },
  ],
} satisfies User;

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
      sets: workSets(80, 1, 300),
      onCycle: vi.fn(),
      onChangeWeight: vi.fn(),
    });
    expect(screen.getByText(/5 reps · 5:00 rest/)).toBeInTheDocument();
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
      sets: workSets(200, 1),
      onCycle: vi.fn(),
      onChangeWeight: vi.fn(),
    });

    // warmupSets(200) → bar×2, then 100 / 140 / 180 = 5 expanded warm-up steps.
    const warmups = container.querySelectorAll<HTMLButtonElement>('button[aria-label^="Warm-up"]');
    expect(warmups).toHaveLength(5);

    const first = warmups[0];
    expect(first).toHaveTextContent("0");
    await fireEvent.click(first);
    expect(first).toHaveTextContent("1"); // reps count up locally
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
});
