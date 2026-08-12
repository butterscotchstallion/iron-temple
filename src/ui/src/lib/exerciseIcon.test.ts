import { describe, it, expect } from "vitest";
import { exerciseEmoji, topSet } from "./exerciseIcon";
import type { ExerciseHistoryPoint } from "./api";

// Minimal ExerciseHistoryPoint fixture — the generated type carries more fields
// than topSet reads, so build points through a helper and cast.
function point(weightLb: number, performedOn: string): ExerciseHistoryPoint {
  return { weightLb, performedOn } as unknown as ExerciseHistoryPoint;
}

describe("exerciseEmoji", () => {
  it("matches known lifts case-insensitively as a substring", () => {
    expect(exerciseEmoji("Squat")).toBe("🦵");
    expect(exerciseEmoji("Barbell Bench Press")).toBe("💪");
    expect(exerciseEmoji("DEADLIFT")).toBe("🏋️");
    expect(exerciseEmoji("Barbell Row")).toBe("🚣");
    expect(exerciseEmoji("Overhead Press")).toBe("🙌");
  });

  it("lets the first matching keyword win (order matters)", () => {
    // "bench" precedes "press", so a bench press is a bench, not a press.
    expect(exerciseEmoji("Bench Press")).toBe("💪");
    // "overhead" precedes "press" and both would match "Overhead Press".
    expect(exerciseEmoji("Overhead Press")).toBe("🙌");
  });

  it("falls back to the default for unknown lifts", () => {
    expect(exerciseEmoji("Bicep Curl")).toBe("🏋️");
    expect(exerciseEmoji("")).toBe("🏋️");
  });
});

describe("topSet", () => {
  it("returns null for empty history", () => {
    expect(topSet([])).toBeNull();
  });

  it("finds the heaviest point", () => {
    const history = [point(45, "2026-01-01"), point(95, "2026-01-08"), point(65, "2026-01-15")];
    expect(topSet(history)).toEqual({ weightLb: 95, performedOn: "2026-01-08" });
  });

  it("keeps the earliest occurrence on ties (the PR-setting session)", () => {
    const history = [point(95, "2026-01-01"), point(95, "2026-01-08")];
    // history is oldest-first, so the first 95 is when the PR was set.
    expect(topSet(history)).toEqual({ weightLb: 95, performedOn: "2026-01-01" });
  });

  it("handles a single point", () => {
    expect(topSet([point(135, "2026-02-02")])).toEqual({
      weightLb: 135,
      performedOn: "2026-02-02",
    });
  });
});
