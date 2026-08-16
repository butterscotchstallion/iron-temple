import { describe, it, expect } from "vitest";
import { formatVolume } from "./volume";

describe("formatVolume", () => {
  it("leaves small volumes alone", () => {
    expect(formatVolume(1)).toBe("1");
    expect(formatVolume(555)).toBe("555"); // 3 reps × 185 lb
  });

  it("groups thousands with commas", () => {
    expect(formatVolume(8450)).toBe("8,450"); // a session
    expect(formatVolume(412650)).toBe("412,650"); // a career
    expect(formatVolume(1204300)).toBe("1,204,300");
  });

  it("rounds to whole pounds", () => {
    expect(formatVolume(227.5)).toBe("228"); // 5 reps × a 45.5 lb bar
    expect(formatVolume(227.4)).toBe("227");
  });

  it("reads as zero rather than nonsense for empty or bad input", () => {
    expect(formatVolume(0)).toBe("0");
    expect(formatVolume(-100)).toBe("0");
    expect(formatVolume(NaN)).toBe("0");
    expect(formatVolume(Infinity)).toBe("0");
  });
});
