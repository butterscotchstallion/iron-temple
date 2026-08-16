import { describe, expect, it } from "vitest";
import {
  CHART_SLOTS,
  barFraction,
  formatDelta,
  formatHour,
  formatPercent,
  formatSessionLength,
  indexedSeries,
  seriesColor,
  type LiftSeries,
} from "./racked";

function series(
  exerciseId: number,
  exerciseName: string,
  e1rms: number[],
): LiftSeries {
  return {
    exerciseId,
    exerciseName,
    points: e1rms.map((e1rmLb, i) => ({
      performedOn: `2026-03-0${i + 1}`,
      topWeightLb: e1rmLb - 10,
      e1rmLb,
    })),
  };
}

describe("indexedSeries", () => {
  it("indexes each lift to its own first session", () => {
    const { shown } = indexedSeries([series(1, "Squat", [200, 220])]);
    expect(shown).toHaveLength(1);
    expect(shown[0].points[0].pct).toBe(0);
    expect(shown[0].points[1].pct).toBeCloseTo(0.1, 10);
  });

  // The whole reason the chart is indexed: a press that gained 20 lb on a base
  // of 100 must read as a bigger move than a deadlift that gained 20 on 400.
  it("makes lifts of different scale comparable", () => {
    const { shown } = indexedSeries([
      series(1, "Deadlift", [400, 420]),
      series(2, "Overhead Press", [100, 120]),
    ]);
    const press = shown.find((s) => s.exerciseName === "Overhead Press")!;
    const dead = shown.find((s) => s.exerciseName === "Deadlift")!;
    expect(press.points[1].pct).toBeGreaterThan(dead.points[1].pct);
  });

  it("drops lifts with too little history to trend", () => {
    const { shown } = indexedSeries([
      series(1, "Squat", [200, 210]),
      series(2, "Bench Press", [100]),
    ]);
    expect(shown.map((s) => s.exerciseName)).toEqual(["Squat"]);
  });

  it("drops a lift whose first estimate is zero rather than dividing by it", () => {
    const { shown } = indexedSeries([series(1, "Squat", [0, 200])]);
    expect(shown).toEqual([]);
  });

  it("caps the drawn lifts and reports how many were left out", () => {
    const many = Array.from({ length: 8 }, (_, i) =>
      series(i + 1, `Lift ${i + 1}`, [100 + i, 120 + i]),
    );
    const { shown, hidden } = indexedSeries(many);
    expect(shown).toHaveLength(CHART_SLOTS);
    expect(hidden).toBe(3);
  });

  it("never puts one colour on two lifts", () => {
    const many = Array.from({ length: 8 }, (_, i) =>
      series(i + 1, `Lift ${i + 1}`, [100 + i, 120 + i]),
    );
    const colors = indexedSeries(many).shown.map((s) => s.color);
    expect(new Set(colors).size).toBe(colors.length);
  });

  // Colour follows the lift, not its position in a ranking, so a period with a
  // different mix does not repaint the lifts that appear in both.
  it("assigns colour by exercise id, not by rank", () => {
    const first = indexedSeries([
      series(1, "Squat", [200, 210]),
      series(9, "Deadlift", [400, 410]),
    ]).shown;
    const second = indexedSeries([
      series(9, "Deadlift", [400, 460]),
      series(1, "Squat", [200, 205]),
    ]).shown;

    const colorOf = (list: typeof first, name: string) =>
      list.find((s) => s.exerciseName === name)!.color;
    expect(colorOf(first, "Squat")).toBe(colorOf(second, "Squat"));
    expect(colorOf(first, "Deadlift")).toBe(colorOf(second, "Deadlift"));
  });

  it("handles an empty report", () => {
    expect(indexedSeries([])).toEqual({ shown: [], hidden: 0 });
  });
});

describe("seriesColor", () => {
  it("maps slots to the theme's categorical variables", () => {
    expect(seriesColor(0)).toBe("var(--chart-1)");
    expect(seriesColor(4)).toBe("var(--chart-5)");
  });

  // Clamped, never wrapped: wrapping would hand slot 1's colour to a sixth lift
  // and put the same colour on two series in one chart.
  it("clamps out-of-range slots instead of cycling", () => {
    expect(seriesColor(9)).toBe("var(--chart-5)");
    expect(seriesColor(-3)).toBe("var(--chart-1)");
  });
});

describe("formatSessionLength", () => {
  it.each([
    [0, "—"],
    [-60, "—"],
    [Number.NaN, "—"],
    [2880, "48m"],
    [3600, "1h"],
    [4320, "1h 12m"],
    [30, "1m"],
  ])("formats %s seconds as %s", (seconds, want) => {
    expect(formatSessionLength(seconds as number)).toBe(want);
  });
});

describe("formatDelta", () => {
  it.each([
    [0.083, "+8%"],
    [-0.03, "−3%"],
    [0, "0%"],
    [0.001, "0%"],
    [1.5, "+150%"],
    [Number.NaN, "—"],
  ])("formats %s as %s", (fraction, want) => {
    expect(formatDelta(fraction as number)).toBe(want);
  });

  // A real minus sign, not a hyphen, so figures align in tabular-nums columns.
  it("uses a typographic minus", () => {
    expect(formatDelta(-0.1)).toContain("−");
    expect(formatDelta(-0.1)).not.toContain("-");
  });
});

describe("formatPercent", () => {
  it.each([
    [0.86, "86%"],
    [0, "0%"],
    [1.2, "120%"],
    [-1, "0%"],
  ])("formats %s as %s", (fraction, want) => {
    expect(formatPercent(fraction as number)).toBe(want);
  });
});

describe("formatHour", () => {
  it.each([
    [0, "12am"],
    [6, "6am"],
    [12, "12pm"],
    [18, "6pm"],
    [23, "11pm"],
  ])("formats %s as %s", (hour, want) => {
    expect(formatHour(hour as number)).toBe(want);
  });
});

describe("barFraction", () => {
  it("scales against the largest value", () => {
    expect(barFraction(50, 100)).toBe(0.5);
    expect(barFraction(100, 100)).toBe(1);
  });

  it("yields nothing rather than dividing by zero", () => {
    expect(barFraction(0, 0)).toBe(0);
    expect(barFraction(10, 0)).toBe(0);
  });

  it("never overflows its track", () => {
    expect(barFraction(150, 100)).toBe(1);
  });
});
