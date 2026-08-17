import { describe, it, expect } from "vitest";
import { estimateOneRepMax } from "./oneRepMax";

describe("estimateOneRepMax", () => {
  it("applies the Epley formula, rounded", () => {
    expect(estimateOneRepMax(100, 5)).toBe(117); // 100 * (1 + 5/30) = 116.67
    expect(estimateOneRepMax(135, 5)).toBe(158); // 135 * 1.1667 = 157.5
  });

  // Epley extrapolates from a set carried past one rep. At exactly one rep it
  // returns weight * 31/30, which estimates a number that needs no estimating:
  // a 200 single was reported as a 207 estimated max.
  it("returns the weight itself for a single", () => {
    expect(estimateOneRepMax(200, 1)).toBe(200);
    expect(estimateOneRepMax(225, 1)).toBe(225);
  });

  // The old formula ranked a single above a double on the same bar, which is
  // backwards — the second rep is strictly more work.
  it("ranks a single below a double at the same weight", () => {
    expect(estimateOneRepMax(225, 1)).toBeLessThan(estimateOneRepMax(225, 2));
  });

  it("is zero for non-positive inputs", () => {
    expect(estimateOneRepMax(0, 5)).toBe(0);
    expect(estimateOneRepMax(100, 0)).toBe(0);
  });
});
