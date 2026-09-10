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
  recentExercises,
  RECENT_LIMIT,
} from "./library";

function exercise(over: Partial<Exercise> & { name: string }): Exercise {
  return {
    id: 1,
    muscleGroup: "other",
    equipment: "other",
    isAccessory: true,
    isCustom: false,
    // The grouping and search tests below don't care about performances, so the
    // default is the "never performed" case; the recentExercises tests set the
    // last two explicitly.
    topSet: null,
    lastPerformedOn: null,
    performedSessions: 0,
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

describe("recentExercises", () => {
  function performed(
    name: string,
    lastPerformedOn: string | null,
    performedSessions = 1,
    over: Partial<Exercise> = {},
  ): Exercise {
    return exercise({ name, lastPerformedOn, performedSessions, ...over });
  }

  const names = (exercises: Exercise[]) => exercises.map((e) => e.name);

  it("puts the most recently performed lift first", () => {
    const recent = recentExercises([
      performed("Hammer Curl", "2026-08-20"),
      performed("Dip", "2026-09-02"),
      performed("Plank", "2026-08-28"),
    ]);
    expect(names(recent)).toEqual(["Dip", "Plank", "Hammer Curl"]);
  });

  it("breaks a shared date on how often the lift has been trained", () => {
    // Every accessory in one workout carries that workout's date, so this is
    // the ordinary case rather than the edge one.
    const recent = recentExercises([
      performed("Face Pull", "2026-09-02", 3),
      performed("Ab Wheel", "2026-09-02", 12),
      performed("Curl", "2026-09-02", 7),
    ]);
    expect(names(recent)).toEqual(["Ab Wheel", "Curl", "Face Pull"]);
  });

  it("falls back to the name so the order is total", () => {
    const recent = recentExercises([
      performed("Shrug", "2026-09-02", 4),
      performed("Chin-up", "2026-09-02", 4),
    ]);
    expect(names(recent)).toEqual(["Chin-up", "Shrug"]);
  });

  it("leaves out the lifts a program prescribes", () => {
    // Trained every week, and never something you'd add as assistance — it
    // would hold the top of the section permanently for nothing.
    const recent = recentExercises([
      performed("Squat", "2026-09-02", 40, { isAccessory: false }),
      performed("Dip", "2026-08-01"),
    ]);
    expect(names(recent)).toEqual(["Dip"]);
  });

  it("leaves out lifts that have never been performed", () => {
    const recent = recentExercises([
      performed("Dip", null, 0),
      performed("Plank", "2026-09-02"),
    ]);
    expect(names(recent)).toEqual(["Plank"]);
  });

  it("treats a missing date as never performed rather than as most recent", () => {
    // A caller whose payload predates the field would otherwise have its whole
    // library promoted, since undefined !== null.
    const stale = { ...exercise({ name: "Dip" }), lastPerformedOn: undefined };
    expect(recentExercises([stale as unknown as Exercise])).toEqual([]);
  });

  it("caps the section and does not reorder its input", () => {
    const library = Array.from({ length: RECENT_LIMIT + 3 }, (_, i) =>
      performed(`Lift ${i}`, `2026-09-${String(i + 1).padStart(2, "0")}`),
    );
    const before = names(library);

    const recent = recentExercises(library);

    expect(recent).toHaveLength(RECENT_LIMIT);
    expect(names(recent)[0]).toBe(`Lift ${RECENT_LIMIT + 2}`);
    expect(names(library)).toEqual(before);
  });
});
