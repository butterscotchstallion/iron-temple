import { describe, it, expect } from "vitest";
import { programSubtitle } from "./programs";

describe("programSubtitle", () => {
  it("uses the description when present", () => {
    expect(programSubtitle({ description: "Squat, bench, row · A/B · 5×5" })).toBe(
      "Squat, bench, row · A/B · 5×5",
    );
  });

  it("trims surrounding whitespace", () => {
    expect(programSubtitle({ description: "  Reduced volume  " })).toBe("Reduced volume");
  });

  it("falls back when the description is empty or blank", () => {
    expect(programSubtitle({ description: "" })).toBe("Linear progression");
    expect(programSubtitle({ description: "   " })).toBe("Linear progression");
  });
});
