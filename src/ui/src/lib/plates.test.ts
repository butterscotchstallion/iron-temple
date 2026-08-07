import { describe, it, expect } from "vitest";
import { platesPerSide, plateLabel } from "./plates";

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

describe("plateLabel", () => {
  it("says 'bar only' at or below the bar", () => {
    expect(plateLabel(80)).toBe("bar only");
    expect(plateLabel(60)).toBe("bar only");
  });

  it("joins the per-side plates largest-first", () => {
    expect(plateLabel(90)).toBe("5 / side");
    expect(plateLabel(140)).toBe("25 + 5 / side"); // (140-80)/2 = 30
    expect(plateLabel(180)).toBe("45 + 5 / side"); // (180-80)/2 = 50
  });
});
