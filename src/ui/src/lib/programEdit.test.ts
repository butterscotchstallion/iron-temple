import { describe, it, expect } from "vitest";
import {
  canEdit,
  idsInOrder,
  move,
  prescribedExerciseIds,
  prescriptionSummary,
  validName,
} from "./programEdit";
import type { ProgramDay, ProgramDayExercise } from "./api";

function lift(over: Partial<ProgramDayExercise> = {}): ProgramDayExercise {
  return {
    id: 1,
    exerciseId: 10,
    exerciseName: "Squat",
    position: 1,
    sets: 5,
    reps: 5,
    startingWeightLb: 95,
    restSeconds: 180,
    ...over,
  };
}

describe("move", () => {
  const items = [{ id: 1 }, { id: 2 }, { id: 3 }];

  it("swaps an item with the one above it", () => {
    expect(move(items, 2, "up")).toEqual([{ id: 2 }, { id: 1 }, { id: 3 }]);
  });

  it("swaps an item with the one below it", () => {
    expect(move(items, 2, "down")).toEqual([{ id: 1 }, { id: 3 }, { id: 2 }]);
  });

  it("produces the rotation a naive database UPDATE cannot do", () => {
    // 1,2,3 -> 2,3,1 is the shuffle that collides against a non-deferrable
    // UNIQUE part-way through a single statement. It is reachable from the UI by
    // moving the first item down twice, so it is worth pinning that this is the
    // list the client would send.
    const once = move(items, 1, "down");
    expect(idsInOrder(move(once, 1, "down"))).toEqual([2, 3, 1]);
  });

  it("returns the same array when the first item moves up", () => {
    // Identity, not equality: callers compare by reference to decide whether to
    // send anything at all, so a no-op must cost no request.
    expect(move(items, 1, "up")).toBe(items);
  });

  it("returns the same array when the last item moves down", () => {
    expect(move(items, 3, "down")).toBe(items);
  });

  it("returns the same array for an id that is not in the list", () => {
    expect(move(items, 99, "up")).toBe(items);
  });

  it("does not mutate the list it was given", () => {
    const original = [{ id: 1 }, { id: 2 }];
    move(original, 2, "up");
    expect(original).toEqual([{ id: 1 }, { id: 2 }]);
  });
});

describe("validName", () => {
  it("rejects empty and whitespace-only names", () => {
    expect(validName("")).toBe(false);
    expect(validName("   ")).toBe(false);
  });

  it("accepts a name at the API's 80-character limit and rejects one past it", () => {
    // Matching the server's bound exactly, so the two refuse the same things —
    // a client that waves through what the API rejects turns a validation
    // message into a failed request.
    expect(validName("x".repeat(80))).toBe(true);
    expect(validName("x".repeat(81))).toBe(false);
  });
});

describe("prescriptionSummary", () => {
  it("names the starting weight when there is one", () => {
    expect(prescriptionSummary(lift())).toBe("5 × 5 · starts at 95 lb");
  });

  it("says nothing about weight at zero", () => {
    // Zero means bodyweight, and "0 lb" is a worse way of saying that than
    // saying nothing — the rule the rest of the app applies to banded and
    // bodyweight work.
    expect(prescriptionSummary(lift({ startingWeightLb: 0 }))).toBe("5 × 5");
  });
});

describe("prescribedExerciseIds", () => {
  const day: ProgramDay = {
    id: 7,
    name: "Workout A",
    position: 1,
    weekday: null,
    exercises: [lift({ id: 1, exerciseId: 10 }), lift({ id: 2, exerciseId: 11 })],
    assistance: [],
  };

  it("lists the movements already prescribed", () => {
    // One entry per lift is the rule, so offering these again would only earn a
    // 409 from the API.
    expect(prescribedExerciseIds(day)).toEqual([10, 11]);
  });
});

describe("canEdit", () => {
  it("is false for a seeded program, which belongs to nobody", () => {
    expect(canEdit({ isMine: false })).toBe(false);
  });

  it("is true for your own", () => {
    expect(canEdit({ isMine: true })).toBe(true);
  });
});
