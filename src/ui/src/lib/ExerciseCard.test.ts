import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/svelte";
import ExerciseCard from "./ExerciseCard.svelte";
import type { SessionSet } from "./api";

// The generated SessionSet has more fields than the card reads; build the subset
// it uses and cast. (BAR_LB is 80, so a work weight of 80 produces no warm-ups.)
type SetFixture = {
  id: number;
  setNumber: number;
  weightLb: number;
  targetReps: number;
  actualReps: number | null;
  completed: boolean;
};
function set(p: SetFixture): SessionSet {
  return p as unknown as SessionSet;
}
function workSets(weightLb: number, count: number): SessionSet[] {
  return Array.from({ length: count }, (_, i) =>
    set({
      id: i + 1,
      setNumber: i + 1,
      weightLb,
      targetReps: 5,
      actualReps: null,
      completed: false,
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
});
