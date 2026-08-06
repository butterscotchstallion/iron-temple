import { describe, it, expect } from "vitest";
import { estimateOneRepMax } from "./oneRepMax";

describe("estimateOneRepMax", () => {
  it("applies the Epley formula, rounded", () => {
    expect(estimateOneRepMax(100, 5)).toBe(117); // 100 * (1 + 5/30) = 116.67
    expect(estimateOneRepMax(135, 5)).toBe(158); // 135 * 1.1667 = 157.5
    expect(estimateOneRepMax(200, 1)).toBe(207); // 200 * (1 + 1/30) = 206.67
  });

  it("is zero for non-positive inputs", () => {
    expect(estimateOneRepMax(0, 5)).toBe(0);
    expect(estimateOneRepMax(100, 0)).toBe(0);
  });
});
