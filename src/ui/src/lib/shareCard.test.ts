import { describe, expect, it } from "vitest";
import type { RackedReport } from "./api";
import {
  LIFT_ROWS,
  MOMENT_ROWS,
  SHARE_CARD,
  fitText,
  paintShareCard,
  shareCardContent,
  shareCardLayout,
  type Painter,
  type ShareCardContent,
} from "./shareCard";

// The share card, tested everywhere except the pixels.
//
// jsdom has no 2D context, so a test can never look at the image. That is why
// selection and geometry are pure functions: what the card says and whether it
// fits inside itself are the two things that go wrong, and both are decided
// before anything is drawn. What remains untested by construction is only
// whether the paint is pretty.

/** A recap with nothing in it — a month with no sessions logged. */
function bareReport(): RackedReport {
  return {
    period: {
      kind: "month",
      start: "2026-03-01",
      end: "2026-03-31",
      label: "March 2026",
      inProgress: false,
    },
    totals: { volumeLb: 0, sessions: 0, sets: 0, reps: 0 },
    change: null,
    comparison: { count: 0, label: "", unitLb: 0 },
    lifts: [],
    series: [],
    mostImproved: null,
    days: [],
    weekdays: [0, 0, 0, 0, 0, 0, 0],
    bestWeekday: -1,
    hours: Array.from({ length: 24 }, () => 0) as RackedReport["hours"],
    peakHour: -1,
    hourLabel: "",
    streak: { longestWeeks: 0, currentWeeks: 0 },
    attendance: { basis: "none", expected: 0, actual: 0, rate: 0, sessionsPerWeek: 0 },
    prs: [],
    milestones: [],
    heaviestSet: null,
    fastestSession: null,
    deloads: [],
    archetype: { name: "", description: "" },
  };
}

function lift(id: number, name: string, volumeLb: number) {
  return { exerciseId: id, exerciseName: name, volumeLb, sets: 12, reps: 60, share: 0.2 };
}

/** A recap with every section the card can draw, at its fullest. */
function fullReport(): RackedReport {
  return {
    ...bareReport(),
    totals: { volumeLb: 84_000, sessions: 12, sets: 180, reps: 900 },
    change: { volumeLb: 9_000, volumePct: 0.12, sessions: 2, sessionsPct: 0.2 },
    comparison: { count: 3, label: "school buses", unitLb: 24_000 },
    lifts: [
      lift(1, "Squat", 50_000),
      lift(2, "Deadlift", 34_000),
      lift(3, "Bench Press", 22_000),
      lift(4, "Barbell Row", 18_000),
      lift(5, "Overhead Press", 12_000),
      lift(6, "Barbell Curl", 4_000),
    ],
    mostImproved: {
      exerciseId: 1,
      exerciseName: "Squat",
      fromLb: 233,
      toLb: 256,
      gainLb: 23,
      gainPct: 0.0987,
    },
    streak: { longestWeeks: 5, currentWeeks: 3 },
    attendance: { basis: "none", expected: 0, actual: 12, rate: 0, sessionsPerWeek: 2.75 },
    prs: [
      {
        kind: "weight",
        performedOn: "2026-03-16",
        exerciseId: 1,
        exerciseName: "Squat",
        weightLb: 220,
        reps: 5,
        valueLb: 220,
        previousLb: 215,
      },
    ],
    heaviestSet: {
      performedOn: "2026-03-16",
      exerciseId: 1,
      exerciseName: "Squat",
      weightLb: 220,
      reps: 5,
    },
    archetype: { name: "The Grinder", description: "Long sessions, no rush." },
  };
}

/** A glyph's average width as a fraction of the font size. Inter, eyeballed. */
const GLYPH = 0.55;

function px(value: string, fallback: number): number {
  return Number(/(-?\d+(?:\.\d+)?)px/.exec(value)?.[1] ?? fallback);
}

/**
 * A recording stand-in for a canvas context.
 *
 * measureText scales with the font that is currently set, which is the whole
 * point of it. A stub that charged a flat width per character — as this one used
 * to — cannot tell a string measured at 96px from the same string measured at
 * 30px, and so cannot see the bug where text is measured in whatever font the
 * previous line left behind and drawn in another. Proportional-to-font-size is
 * still an approximation, but it is wrong by a constant factor rather than wrong
 * about which font was in effect.
 */
function stubPainter() {
  const texts: string[] = [];
  const drawn: {
    text: string;
    x: number;
    y: number;
    width: number;
    size: number;
    align: CanvasTextAlign;
  }[] = [];
  const fills: { x: number; y: number; w: number; h: number }[] = [];

  const width = (text: string) =>
    text.length * (px(ctx.font, 16) * GLYPH + px(ctx.letterSpacing, 0));

  const ctx = {
    fillStyle: "",
    font: "",
    textAlign: "left" as CanvasTextAlign,
    textBaseline: "top" as CanvasTextBaseline,
    letterSpacing: "0px",
    fillText: (text: string, x: number, y: number) => {
      texts.push(text);
      drawn.push({
        text,
        x,
        y,
        width: width(text),
        size: px(ctx.font, 16),
        align: ctx.textAlign,
      });
    },
    measureText: (text: string) => ({ width: width(text) }),
    fillRect: (x: number, y: number, w: number, h: number) => void fills.push({ x, y, w, h }),
    createLinearGradient: () => ({ addColorStop: () => {} }),
    save: () => {},
    restore: () => {},
    beginPath: () => {},
    fill: () => {},
  };
  return { ctx: ctx as unknown as Painter, texts, drawn, fills };
}

/** Where a drawn string actually starts and ends, given how it was aligned. */
function extent(d: { x: number; width: number; align: CanvasTextAlign }) {
  if (d.align === "right") return { left: d.x - d.width, right: d.x };
  if (d.align === "center") return { left: d.x - d.width / 2, right: d.x + d.width / 2 };
  return { left: d.x, right: d.x + d.width };
}

describe("shareCardContent", () => {
  it("reads the headline the way the page and the email do", () => {
    const content = shareCardContent(fullReport(), "Ada Lovelace");

    expect(content.eyebrow).toBe("RACKED · MARCH 2026");
    expect(content.lede).toBe("Ada Lovelace lifted");
    expect(content.headline).toBe("84,000 LB");
    expect(content.comparison).toBe("That's 3 school buses.");
    expect(content.change).toBe("+12% on the previous month");
  });

  it("addresses a lifter with no display name without leaving a gap", () => {
    expect(shareCardContent(fullReport(), "").lede).toBe("You lifted");
    expect(shareCardContent(fullReport(), "   ").lede).toBe("You lifted");
    expect(shareCardContent(fullReport()).lede).toBe("You lifted");
  });

  it("carries the heaviest lifts, scaled against the heaviest of them", () => {
    const content = shareCardContent(fullReport());

    expect(content.lifts).toHaveLength(LIFT_ROWS);
    expect(content.lifts.map((l) => l.name)).toEqual([
      "Squat",
      "Deadlift",
      "Bench Press",
      "Barbell Row",
      "Overhead Press",
    ]);
    expect(content.lifts[0].fraction).toBe(1);
    expect(content.lifts[0].value).toBe("50,000 lb");
    expect(content.lifts[4].fraction).toBeCloseTo(12_000 / 50_000);
  });

  it("caps the moments and names the record count in agreement", () => {
    const content = shareCardContent(fullReport());

    expect(content.moments).toHaveLength(MOMENT_ROWS);
    expect(content.moments[0]).toEqual({ label: "Most improved", value: "Squat +10%" });
    expect(content.moments[1]).toEqual({ label: "Heaviest set", value: "220 lb × 5" });
    expect(content.moments[2]).toEqual({ label: "Personal record", value: "1" });

    const many = fullReport();
    many.prs = [many.prs[0], { ...many.prs[0] }];
    expect(shareCardContent(many).moments[2]).toEqual({ label: "Personal records", value: "2" });
  });

  // Every one of these is a field the API sends as null or empty for a first
  // month, and every one of them would otherwise print a heading over nothing.
  it("drops each section the period has no answer for", () => {
    const content = shareCardContent(bareReport(), "Ada");

    expect(content.comparison).toBeNull();
    expect(content.change).toBeNull();
    expect(content.archetype).toBeNull();
    expect(content.lifts).toEqual([]);
    expect(content.moments).toEqual([]);
    expect(content.headline).toBe("0 LB");
  });

  // Growth from zero is not a ratio, so the API sends the change object with a
  // null percentage. Asking only whether `change` exists would print "null%".
  it("drops the comparison when there is a previous period but no ratio", () => {
    const report = fullReport();
    report.change = { volumeLb: 9_000, volumePct: null, sessions: 2, sessionsPct: null };

    expect(shareCardContent(report).change).toBeNull();
  });

  it("quotes a rate only where the program carries a schedule", () => {
    expect(shareCardContent(fullReport()).footnote).toBe("2.8 sessions a week");

    const scheduled = fullReport();
    scheduled.attendance = {
      basis: "weekday",
      expected: 14,
      actual: 12,
      rate: 0.857,
      sessionsPerWeek: 2.75,
    };
    expect(shareCardContent(scheduled).footnote).toBe("86% of scheduled sessions");
  });

  // The report compares a running period against the same stretch of the one
  // before it, so the card cannot say "on the previous month" — that is a claim
  // about two whole months.
  it("names the elapsed comparison while the period is still running", () => {
    const report = fullReport();
    report.period = { ...report.period, inProgress: true };

    expect(shareCardContent(report).change).toBe("+12% on the same point last month");
  });

  it("names the year, not the month, on a yearly recap", () => {
    const report = fullReport();
    report.period = {
      kind: "year",
      start: "2026-01-01",
      end: "2026-12-31",
      label: "2026",
      inProgress: false,
    };

    const content = shareCardContent(report);
    expect(content.eyebrow).toBe("RACKED · 2026");
    expect(content.change).toBe("+12% on the previous year");
  });
});

describe("shareCardLayout", () => {
  /** Every block, in order, with no two occupying the same pixel. */
  function assertStacked(content: ShareCardContent) {
    const { blocks, footerY, contentBottom } = shareCardLayout(content);
    expect(blocks.length).toBeGreaterThan(0);
    expect(blocks[0].y).toBeGreaterThanOrEqual(SHARE_CARD.pad);

    for (let i = 1; i < blocks.length; i++) {
      const previousEnd = blocks[i - 1].y + blocks[i - 1].height;
      expect(blocks[i].y).toBeGreaterThan(previousEnd);
    }

    const last = blocks[blocks.length - 1];
    return { end: last.y + last.height, footerY, contentBottom };
  }

  // The assertion nobody can make by looking at the running app: an overflow
  // only appears on the one month that happens to fill every section at once.
  //
  // Against contentBottom rather than footerY deliberately. Layout clamps an
  // oversized stack to the top pad instead of centring it, so measuring against
  // the footer would keep passing for the first 32 pixels of overflow and then
  // fail as a mystery. This fails on the pixel the card outgrows its region.
  it("fits the fullest possible card in the space above the footer", () => {
    const { end, contentBottom } = assertStacked(shareCardContent(fullReport(), "Ada Lovelace"));
    expect(end).toBeLessThanOrEqual(contentBottom);
  });

  it("fits a card with nothing on it but the totals", () => {
    const { end, contentBottom } = assertStacked(shareCardContent(bareReport()));
    expect(end).toBeLessThanOrEqual(contentBottom);
  });

  // The longest display name and exercise names a lifter can supply do not
  // change the geometry — they are truncated at paint time, not wrapped, so a
  // verbose account cannot push the card past its own footer.
  it("keeps the geometry fixed however long the names are", () => {
    const wordy = fullReport();
    wordy.archetype = { name: "The ".repeat(20), description: "Long. ".repeat(40) };
    wordy.lifts = wordy.lifts.map((l) => ({ ...l, exerciseName: l.exerciseName.repeat(12) }));

    const { end, contentBottom } = assertStacked(shareCardContent(wordy, "A".repeat(200)));
    expect(end).toBeLessThanOrEqual(contentBottom);
  });

  // A quiet month has three blocks where a full one has five. Top-aligning it
  // would leave the bottom third of the image empty.
  it("centres a short card rather than stranding it at the top", () => {
    const sparse = shareCardLayout(shareCardContent(bareReport()));
    const full = shareCardLayout(shareCardContent(fullReport()));

    expect(sparse.blocks[0].y).toBeGreaterThan(full.blocks[0].y);
    expect(full.blocks[0].y).toBeGreaterThanOrEqual(SHARE_CARD.pad);
  });

  it("gives an absent section no space at all", () => {
    const kinds = (content: ShareCardContent) =>
      shareCardLayout(content).blocks.map((b) => b.kind);

    expect(kinds(shareCardContent(fullReport()))).toEqual([
      "header",
      "archetype",
      "tiles",
      "lifts",
      "moments",
    ]);
    expect(kinds(shareCardContent(bareReport()))).toEqual(["header", "tiles"]);
  });

  it("grows the header for the lines a fuller period adds", () => {
    const full = shareCardLayout(shareCardContent(fullReport()));
    const bare = shareCardLayout(shareCardContent(bareReport()));

    const header = (l: typeof full) => l.blocks.find((b) => b.kind === "header")!.height;
    expect(header(full)).toBeGreaterThan(header(bare));
  });
});

describe("fitText", () => {
  const { ctx } = stubPainter();

  it("leaves text that already fits alone", () => {
    expect(fitText(ctx, "Squat", 1000)).toBe("Squat");
  });

  it("truncates to an ellipsis inside the width", () => {
    // No font set, so the stub charges its 16px fallback: a handful of glyphs.
    const fitted = fitText(ctx, "Romanian Deadlift", 60);
    expect(fitted.endsWith("…")).toBe(true);
    expect(fitted.length).toBeLessThan("Romanian Deadlift".length);
    expect(ctx.measureText(fitted).width).toBeLessThanOrEqual(60);
  });

  it("does not truncate away the whole string", () => {
    expect(fitText(ctx, "Squat", 1).length).toBeGreaterThan(1);
  });
});

describe("paintShareCard", () => {
  it("draws the headline and every section of a full recap", () => {
    const { ctx, texts } = stubPainter();
    paintShareCard(ctx, shareCardContent(fullReport(), "Ada Lovelace"));

    expect(texts).toContain("84,000 LB");
    expect(texts).toContain("Ada Lovelace lifted");
    expect(texts).toContain("RACKED · MARCH 2026");
    expect(texts).toContain("THE GRINDER");
    expect(texts).toContain("WHERE THE WEIGHT WENT");
    expect(texts).toContain("MOMENTS");
    expect(texts).toContain("IRON TEMPLE");
    expect(texts).toContain("Squat");
    expect(texts).toContain("2.8 sessions a week");
  });

  // The painter walks nullable content. A month with no archetype and no lifts
  // must paint a shorter card, not throw halfway through one.
  it("paints a bare recap without reaching into an absent section", () => {
    const { ctx, texts } = stubPainter();
    expect(() => paintShareCard(ctx, shareCardContent(bareReport()))).not.toThrow();

    expect(texts).toContain("0 LB");
    expect(texts).not.toContain("WHERE THE WEIGHT WENT");
    expect(texts).not.toContain("MOMENTS");
  });

  it("fills the full canvas so no transparent edge survives into the PNG", () => {
    const { ctx, fills } = stubPainter();
    paintShareCard(ctx, shareCardContent(fullReport()));

    expect(fills[0]).toEqual({ x: 0, y: 0, w: SHARE_CARD.width, h: SHARE_CARD.height });
  });

  // Without roundRect the bars must still be drawn, square, rather than skipped.
  it("falls back to square corners where roundRect is unavailable", () => {
    const { ctx, fills } = stubPainter();
    paintShareCard(ctx, shareCardContent(fullReport()));

    // The five lift bars and their tracks, plus the background, footer rule and
    // archetype panel: every one a fillRect once roundRect is missing.
    expect(fills.length).toBeGreaterThan(LIFT_ROWS * 2);
  });

  // Every string is truncated against the font it is drawn in, never the one
  // the previous line left set. Measuring the comparison at the headline's 96px
  // and drawing it at 30px cut "That's 3 school buses." down to "That's 3 school
  // b…" — a sentence that fits with room to spare, ellipsised for no reason.
  it("measures each line in the font that line is drawn in", () => {
    const { ctx, texts } = stubPainter();
    paintShareCard(ctx, shareCardContent(fullReport(), "Ada Lovelace"));

    expect(texts).toContain("That's 3 school buses.");
    expect(texts).toContain("Ada Lovelace lifted");
    expect(texts).toContain("Long sessions, no rush.");
    expect(texts.filter((t) => t.endsWith("…"))).toEqual([]);
  });

  // The other half of the same bug: a string measured in a smaller font than it
  // is drawn in is under-truncated, and runs off the right edge instead.
  it("truncates a long name against its own font, not a smaller one", () => {
    const { ctx, drawn } = stubPainter();
    const wordy = fullReport();
    wordy.archetype = { name: "The Extraordinarily Verbose Metronome", description: "x" };

    paintShareCard(ctx, shareCardContent(wordy, "Bartholomew ".repeat(6)));

    const lede = drawn.find((d) => d.text.startsWith("Bartholomew"))!;
    expect(lede.text.endsWith("…")).toBe(true);
    expect(extent(lede).right).toBeLessThanOrEqual(SHARE_CARD.width - SHARE_CARD.pad);
  });

  // The other check nobody can make by looking at the app, and the one that
  // catches a column arithmetic slip: anything drawn outside the canvas is
  // simply not in the PNG, and the card would come out looking merely sparse
  // rather than broken.
  it("draws everything inside the canvas", () => {
    const { ctx, drawn, fills } = stubPainter();
    paintShareCard(ctx, shareCardContent(fullReport(), "Ada Lovelace"));

    for (const d of drawn) {
      const { left, right } = extent(d);
      expect.soft(left, `left of "${d.text}"`).toBeGreaterThanOrEqual(0);
      expect.soft(right, `right of "${d.text}"`).toBeLessThanOrEqual(SHARE_CARD.width);
      expect.soft(d.y, `y of "${d.text}"`).toBeGreaterThanOrEqual(0);
      expect.soft(d.y, `y of "${d.text}"`).toBeLessThan(SHARE_CARD.height);
    }

    // The background covers the whole card, so it is the only fill allowed to
    // reach the edges; every other rectangle stays inside the padding.
    for (const rect of fills.slice(1)) {
      expect.soft(rect.w).toBeGreaterThanOrEqual(0);
      expect.soft(rect.x).toBeGreaterThanOrEqual(SHARE_CARD.pad);
      expect.soft(rect.x + rect.w).toBeLessThanOrEqual(SHARE_CARD.width - SHARE_CARD.pad);
      expect.soft(rect.y + rect.h).toBeLessThanOrEqual(SHARE_CARD.height);
    }
  });

  // The archetype panel closed the same padding under its description as above
  // its title only by coincidence of two numbers kept in different places, and
  // for a while it did not: 22px above, 4px below. Both now come from PANEL, and
  // this is what says so.
  it("pads the lifter type panel evenly, top and bottom", () => {
    const { ctx, drawn, fills } = stubPainter();
    paintShareCard(ctx, shareCardContent(fullReport()));

    // The first full-width fill after the background. The footer rule is also
    // full width, hence the height condition — it is two pixels tall.
    const panel = fills
      .slice(1)
      .find(
        (r) =>
          r.x === SHARE_CARD.pad && r.w === SHARE_CARD.width - SHARE_CARD.pad * 2 && r.h > 10,
      )!;
    expect(panel).toBeDefined();

    const title = drawn.find((d) => d.text === "YOUR LIFTER TYPE")!;
    const description = drawn.find((d) => d.text === "Long sessions, no rush.")!;

    const above = title.y - panel.y;
    const below = panel.y + panel.h - (description.y + description.size);
    expect(above).toBeGreaterThan(0);
    expect(below).toBe(above);
  });

  // A lift with no volume against a heaviest of zero is a zero-width bar, not a
  // negative one — barFraction floors it, and roundRect throws on a negative
  // width where fillRect merely draws nothing.
  it("draws no negative bars when nothing was lifted", () => {
    const { ctx, fills } = stubPainter();
    const nothing = fullReport();
    nothing.lifts = nothing.lifts.map((l) => ({ ...l, volumeLb: 0 }));

    paintShareCard(ctx, shareCardContent(nothing));
    for (const rect of fills) expect(rect.w).toBeGreaterThanOrEqual(0);
  });
});
