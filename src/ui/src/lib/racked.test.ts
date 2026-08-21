import { describe, expect, it } from "vitest";
import {
  CHART_SLOTS,
  barFraction,
  formatDelta,
  formatHour,
  formatPercent,
  formatPerWeek,
  formatSessionLength,
  formatSignedLb,
  formatWeighIn,
  indexedSeries,
  joinNames,
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

  // The reason selection is by volume. Ranking by the weight on the bar cut the
  // overhead press from every chart, on a chart about percentage gain where the
  // press is usually the biggest gainer.
  it("keeps the lift with the most work over the one with the most weight", () => {
    const lifts = [
      series(1, "Deadlift", [400, 405]),
      series(2, "Squat", [300, 310]),
      series(3, "Bench Press", [200, 210]),
      series(4, "Barbell Row", [150, 160]),
      series(5, "Overhead Press", [100, 120]),
      series(6, "Pause Squat", [280, 282]),
    ];
    // The press is the lightest lift and the one done most; the pause squat is
    // heavy and barely touched.
    const volume = new Map([
      [1, 9_000],
      [2, 20_000],
      [3, 14_000],
      [4, 12_000],
      [5, 18_000],
      [6, 500],
    ]);

    const names = indexedSeries(lifts, volume).shown.map((s) => s.exerciseName);
    expect(names).toContain("Overhead Press");
    expect(names).not.toContain("Pause Squat");
  });

  // A stalled main lift is worth seeing flat; gain-ranked selection would drop it.
  it("keeps a lift that did not move, if the lifter did the work", () => {
    const lifts = [
      series(1, "Squat", [300, 300]),
      series(2, "Bench Press", [200, 240]),
    ];
    const volume = new Map([
      [1, 30_000],
      [2, 4_000],
    ]);
    const names = indexedSeries(lifts, volume, 1).shown.map((s) => s.exerciseName);
    expect(names).toEqual(["Squat"]);
  });

  it("falls back to a stable order when no volume is known", () => {
    const lifts = [series(9, "Deadlift", [400, 410]), series(1, "Squat", [200, 210])];
    const first = indexedSeries(lifts).shown.map((s) => s.exerciseId);
    const second = indexedSeries([...lifts].reverse()).shown.map((s) => s.exerciseId);
    expect(first).toEqual(second);
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

describe("formatPerWeek", () => {
  it.each([
    [2.75, "2.8"],
    [3, "3.0"],
    [0.5, "0.5"],
    [0, "0"],
    [-1, "0"],
    [Number.NaN, "0"],
  ])("formats %s as %s", (perWeek, want) => {
    expect(formatPerWeek(perWeek as number)).toBe(want);
  });
});

describe("formatWeighIn", () => {
  it.each([
    [181.4, "181.4"],
    // The trailing ".0" goes: a scale that read exactly 181 should say so.
    [181, "181"],
    [181.44, "181.4"],
    [0, "—"],
    [Number.NaN, "—"],
  ])("formats %s as %s", (lb, want) => {
    expect(formatWeighIn(lb as number)).toBe(want);
  });

  // Not formatVolume, which rounds to whole pounds. That is right for a
  // five-figure tonnage and wrong for a scale, where the half-pound is most of
  // what moved.
  it("keeps the decimal a volume would round away", () => {
    expect(formatWeighIn(181.4)).not.toBe("181");
  });
});

describe("formatSignedLb", () => {
  it.each([
    [-2.6, "−2.6"],
    [1.5, "+1.5"],
    [2, "+2"],
    // Held steady is a real answer, and reads as one rather than as "+0".
    [0, "0"],
    [Number.NaN, "—"],
  ])("formats %s as %s", (lb, want) => {
    expect(formatSignedLb(lb as number)).toBe(want);
  });

  // A real minus sign, for the reason formatDelta uses one: these sit in
  // tabular-nums columns where a hyphen is narrower than a digit.
  it("uses a minus sign rather than a hyphen", () => {
    expect(formatSignedLb(-2.6).startsWith("−")).toBe(true);
    expect(formatSignedLb(-2.6).startsWith("-")).toBe(false);
  });
});

// The recap email builds the same sentence in Go (joinNames in template.go).
// Both are hand-rolled so the two surfaces punctuate a lifter's month
// identically — Intl.ListFormat would put an Oxford comma on only one of them.
describe("joinNames", () => {
  it("reads a list the way a person says it", () => {
    expect(joinNames([])).toBe("");
    expect(joinNames(["core"])).toBe("core");
    expect(joinNames(["core", "arms"])).toBe("core and arms");
    expect(joinNames(["core", "arms", "chest"])).toBe("core, arms and chest");
  });
});
