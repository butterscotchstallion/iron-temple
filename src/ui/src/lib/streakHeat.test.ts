import { describe, it, expect } from "vitest";
import {
  combinedHeat,
  heatColor,
  streakHeat,
  STREAK_FIRE_THRESHOLD,
  STREAK_PEAK,
} from "./streakHeat";
import { STREAK_DISPLAY_THRESHOLD } from "./streak";

/**
 * The hue out of an `oklch(L C H)` string, unwrapped past 360.
 *
 * heatColor emits hues mod 360, so the ramp's arc reads 308 -> 346 -> 49 -> 75
 * and appears to jump backwards halfway through. The arc only crosses 360 once
 * (it spans 127 degrees in total), so anything that comes out below the purple
 * it starts from is on the far side of that single crossing.
 */
function hue(color: string): number {
  const raw = Number(color.slice("oklch(".length, -1).split(" ")[2]);
  return raw < 300 ? raw + 360 : raw;
}

function lightness(color: string): number {
  return Number(color.slice("oklch(".length, -1).split(" ")[0]);
}

describe("streakHeat", () => {
  it("is cold at the threshold where the card first appears", () => {
    const heat = streakHeat(STREAK_DISPLAY_THRESHOLD);
    expect(heat.glow).toBe(0);
    expect(heat.blaze).toBe(0);
    expect(heat.ablaze).toBe(false);
  });

  it("brightens over the run-up to ignition without lighting any flames", () => {
    expect(streakHeat(4).glow).toBeCloseTo(1 / 3);
    expect(streakHeat(5).glow).toBeCloseTo(2 / 3);
    expect(streakHeat(4).ablaze).toBe(false);
    expect(streakHeat(5).ablaze).toBe(false);
    expect(streakHeat(5).blaze).toBe(0);
  });

  it("catches fire at exactly six sessions, with the glow ramp complete", () => {
    const heat = streakHeat(STREAK_FIRE_THRESHOLD);
    expect(heat.ablaze).toBe(true);
    expect(heat.glow).toBe(1);
    expect(heat.blaze).toBe(0);
  });

  it("keeps intensifying between ignition and the peak", () => {
    expect(streakHeat(9).blaze).toBeCloseTo(0.5);
    expect(streakHeat(STREAK_PEAK).blaze).toBe(1);
  });

  it("holds at the peak rather than escalating forever", () => {
    expect(streakHeat(40).blaze).toBe(1);
    expect(streakHeat(40).glow).toBe(1);
  });

  it("clamps below the threshold instead of going negative", () => {
    // The card renders nothing down here, but a negative ramp leaking into a
    // colour or an opacity would be a silent mess if that ever changed.
    for (const streak of [0, 1, 2]) {
      expect(streakHeat(streak).glow).toBe(0);
      expect(streakHeat(streak).blaze).toBe(0);
      expect(streakHeat(streak).ablaze).toBe(false);
    }
  });
});

describe("combinedHeat", () => {
  it("spans the whole ladder, with ignition at the midpoint", () => {
    expect(combinedHeat(streakHeat(STREAK_DISPLAY_THRESHOLD))).toBe(0);
    expect(combinedHeat(streakHeat(STREAK_FIRE_THRESHOLD))).toBe(0.5);
    expect(combinedHeat(streakHeat(STREAK_PEAK))).toBe(1);
  });
});

describe("heatColor", () => {
  it("starts on the app's neon purple", () => {
    expect(heatColor(0)).toBe("oklch(0.601 0.286 307.98)");
  });

  it("is hot orange at ignition", () => {
    expect(heatColor(0.5)).toBe("oklch(0.724 0.187 49.09)");
  });

  it("is amber at the peak", () => {
    expect(heatColor(1)).toBe("oklch(0.813 0.165 75.04)");
  });

  it("emits hues inside 0-360 rather than relying on the browser to wrap them", () => {
    for (let i = 0; i <= 20; i += 1) {
      const raw = Number(heatColor(i / 20).slice("oklch(".length, -1).split(" ")[2]);
      expect(raw).toBeGreaterThanOrEqual(0);
      expect(raw).toBeLessThan(360);
    }
  });

  it("travels forward through the spectrum and never doubles back", () => {
    // The STOPS table is written as an increasing sequence past 360 precisely so
    // the ramp takes the short way round through red. A non-monotonic step here
    // means someone wrapped a stop back under 360 in the table itself, and the
    // colour will swing through green on the way.
    const hues = Array.from({ length: 21 }, (_, i) => hue(heatColor(i / 20)));
    for (let i = 1; i < hues.length; i += 1) {
      expect(hues[i]).toBeGreaterThan(hues[i - 1]);
    }
  });

  it("gets lighter the whole way up", () => {
    const steps = Array.from({ length: 21 }, (_, i) => lightness(heatColor(i / 20)));
    for (let i = 1; i < steps.length; i += 1) {
      expect(steps[i]).toBeGreaterThan(steps[i - 1]);
    }
  });

  it("clamps outside 0-1 rather than extrapolating off the ramp", () => {
    expect(heatColor(-2)).toBe(heatColor(0));
    expect(heatColor(5)).toBe(heatColor(1));
  });
});
