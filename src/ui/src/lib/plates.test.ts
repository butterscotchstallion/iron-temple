import { describe, it, expect } from "vitest";
import { platesPerSide } from "./plates";

describe("platesPerSide", () => {
  it("is empty for the bar alone or lighter (80 lb bar)", () => {
    expect(platesPerSide(80)).toEqual([]);
    expect(platesPerSide(60)).toEqual([]);
  });

  it("loads a single plate per side", () => {
    expect(platesPerSide(90)).toEqual([5]); // (90-80)/2 = 5
    expect(platesPerSide(130)).toEqual([25]); // (130-80)/2 = 25
    expect(platesPerSide(85)).toEqual([2.5]); // (85-80)/2 = 2.5
  });

  it("stacks plates largest-first", () => {
    expect(platesPerSide(220)).toEqual([45, 25]); // 70 = 45 + 25
    expect(platesPerSide(135)).toEqual([25, 2.5]); // 27.5 = 25 + 2.5
  });

  it("respects a custom bar weight", () => {
    expect(platesPerSide(95, 45)).toEqual([25]); // (95-45)/2 = 25
  });
});
