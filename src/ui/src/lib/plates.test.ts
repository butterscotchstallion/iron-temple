import { describe, it, expect } from "vitest";
import { platesPerSide } from "./plates";

describe("platesPerSide", () => {
  it("is empty for the bar alone or lighter", () => {
    expect(platesPerSide(45)).toEqual([]);
    expect(platesPerSide(30)).toEqual([]);
  });

  it("loads a single plate per side", () => {
    expect(platesPerSide(95)).toEqual([25]); // (95-45)/2 = 25
    expect(platesPerSide(135)).toEqual([45]); // (135-45)/2 = 45
    expect(platesPerSide(50)).toEqual([2.5]); // (50-45)/2 = 2.5
  });

  it("stacks plates largest-first", () => {
    expect(platesPerSide(185)).toEqual([45, 25]); // 70 = 45 + 25
    expect(platesPerSide(100)).toEqual([25, 2.5]); // 27.5 = 25 + 2.5
    expect(platesPerSide(225)).toEqual([45, 45]); // 90 = 45 + 45
  });
});
