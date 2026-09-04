import { describe, it, expect } from "vitest";
import { exerciseEmoji } from "./exerciseIcon";

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

  it("resolves the accessory movements in the exercise library", () => {
    expect(exerciseEmoji("Barbell Curl")).toBe("💪");
    expect(exerciseEmoji("Dip")).toBe("🤸");
    expect(exerciseEmoji("Chin-Up")).toBe("🧗");
    expect(exerciseEmoji("Plank")).toBe("🧘");
    expect(exerciseEmoji("Cable Fly")).toBe("🦅");
  });

  it("prefers the specific movement over the generic keyword", () => {
    // "leg curl" precedes "curl": a leg curl is a hamstring movement, not an
    // arm one. Same for the leg press against the bare "press".
    expect(exerciseEmoji("Leg Curl")).toBe("🦵");
    expect(exerciseEmoji("Leg Press")).toBe("🦵");
    // "calf" precedes "raise", so a calf raise is not a lateral raise.
    expect(exerciseEmoji("Standing Calf Raise")).toBe("🦶");
    expect(exerciseEmoji("Lateral Raise")).toBe("🙆");
    // "face pull" precedes "row"; both are upper-back work, but only one is a row.
    expect(exerciseEmoji("Face Pull")).toBe("🎯");
    expect(exerciseEmoji("Seated Cable Row")).toBe("🚣");
  });

  it("falls back to the default for unknown lifts", () => {
    expect(exerciseEmoji("Sandbag Carry")).toBe("🏋️");
    expect(exerciseEmoji("")).toBe("🏋️");
  });
});

