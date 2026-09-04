import { describe, it, expect } from "vitest";
import type { Exercise } from "./api";
import {
  MUSCLE_GROUPS,
  countByGroup,
  equipmentLabel,
  exerciseSubtitle,
  groupExercises,
  matchesSearch,
  muscleGroupLabel,
} from "./library";

function exercise(over: Partial<Exercise> & { name: string }): Exercise {
  return {
    id: 1,
    muscleGroup: "other",
    equipment: "other",
    isAccessory: true,
    isCustom: false,
    // Nothing in this file exercises the top set — it drives the library's
    // grouping and search — so the default is the "never performed" case.
    topSet: null,
    ...over,
  };
}

const library: Exercise[] = [
  exercise({ id: 1, name: "Squat", muscleGroup: "legs", equipment: "barbell", isAccessory: false }),
  exercise({ id: 2, name: "Leg Press", muscleGroup: "legs", equipment: "machine" }),
  exercise({ id: 3, name: "Barbell Curl", muscleGroup: "arms", equipment: "barbell" }),
  exercise({ id: 4, name: "Hammer Curl", muscleGroup: "arms", equipment: "dumbbell" }),
  exercise({ id: 5, name: "Dip", muscleGroup: "chest", equipment: "bodyweight" }),
];

describe("matchesSearch", () => {
  it("matches case-insensitively on a substring", () => {
    expect(matchesSearch({ name: "Barbell Curl" }, "curl")).toBe(true);
    expect(matchesSearch({ name: "Barbell Curl" }, "BARBELL")).toBe(true);
    expect(matchesSearch({ name: "Barbell Curl" }, "squat")).toBe(false);
  });

  it("treats an empty or whitespace-only query as matching everything", () => {
    expect(matchesSearch({ name: "Dip" }, "")).toBe(true);
    expect(matchesSearch({ name: "Dip" }, "   ")).toBe(true);
  });

  it("ignores whitespace around the query", () => {
    expect(matchesSearch({ name: "Leg Press" }, "  press  ")).toBe(true);
  });
});

describe("groupExercises", () => {
  it("groups in the library's reading order, not alphabetically", () => {
    const groups = groupExercises(library);
    expect(groups.map((g) => g.group)).toEqual(["chest", "legs", "arms"]);
  });

  it("keeps the order exercises arrived in within a group", () => {
    const arms = groupExercises(library).find((g) => g.group === "arms");
    expect(arms?.exercises.map((e) => e.name)).toEqual(["Barbell Curl", "Hammer Curl"]);
  });

  it("drops groups the search emptied rather than showing a bare heading", () => {
    const groups = groupExercises(library, { query: "curl" });
    expect(groups).toHaveLength(1);
    expect(groups[0].group).toBe("arms");
    expect(groups[0].exercises.map((e) => e.name)).toEqual(["Barbell Curl", "Hammer Curl"]);
  });

  it("narrows to one muscle group when asked", () => {
    const groups = groupExercises(library, { group: "legs" });
    expect(groups.map((g) => g.group)).toEqual(["legs"]);
    expect(groups[0].exercises).toHaveLength(2);
  });

  it("applies the search and the group filter together", () => {
    expect(groupExercises(library, { group: "legs", query: "curl" })).toEqual([]);
    const groups = groupExercises(library, { group: "arms", query: "hammer" });
    expect(groups[0].exercises.map((e) => e.name)).toEqual(["Hammer Curl"]);
  });

  it("labels each group for display", () => {
    const groups = groupExercises(library, { group: "chest" });
    expect(groups[0].label).toBe("Chest");
  });

  it("returns nothing for an empty library", () => {
    expect(groupExercises([])).toEqual([]);
  });
});

describe("countByGroup", () => {
  it("counts the whole library, so the chips hold still while you type", () => {
    const counts = countByGroup(library);
    expect(counts.legs).toBe(2);
    expect(counts.arms).toBe(2);
    expect(counts.chest).toBe(1);
    expect(counts.back).toBe(0);
  });

  it("has an entry for every group, including the empty ones", () => {
    expect(Object.keys(countByGroup([])).sort()).toEqual([...MUSCLE_GROUPS].sort());
  });
});

describe("labels", () => {
  it("renders known values in title case", () => {
    expect(muscleGroupLabel("shoulders")).toBe("Shoulders");
    expect(equipmentLabel("bodyweight")).toBe("Bodyweight");
  });

  it("passes an unknown value through rather than rendering undefined", () => {
    expect(muscleGroupLabel("spleen")).toBe("spleen");
    expect(equipmentLabel("kettlebell")).toBe("kettlebell");
  });
});

describe("exerciseSubtitle", () => {
  it("is the equipment alone for an ordinary accessory", () => {
    expect(
      exerciseSubtitle({ equipment: "cable", isAccessory: true, isCustom: false }),
    ).toBe("Cable");
  });

  it("marks the lifts a program prescribes", () => {
    expect(
      exerciseSubtitle({ equipment: "barbell", isAccessory: false, isCustom: false }),
    ).toBe("Barbell · Program lift");
  });

  it("marks a lifter's own movements", () => {
    expect(
      exerciseSubtitle({ equipment: "other", isAccessory: true, isCustom: true }),
    ).toBe("Other · Yours");
  });
});
