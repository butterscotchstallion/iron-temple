import { describe, expect, it } from "vitest";
import {
  deloadLabel,
  layoffHeadline,
  pctLabel,
  shouldPrompt,
  weeksLabel,
} from "./layoff";
import type { Layoff } from "./api";

function layoff(weeks: number, deloadPct: number): Layoff {
  return {
    weeks,
    lastTrainedOn: "2026-08-01",
    deloadPct,
    applied: false,
  };
}

describe("pctLabel", () => {
  it("writes a fraction as whole percent", () => {
    expect(pctLabel(0.1)).toBe("10%");
    expect(pctLabel(0.3)).toBe("30%");
    expect(pctLabel(0.5)).toBe("50%");
  });

  // The API computes the fraction in integer percent so this stays clean, but
  // a badge reading "30.000000000000004%" is the failure worth pinning.
  it("survives a fraction that arrives with float dust on it", () => {
    expect(pctLabel(0.30000000000000004)).toBe("30%");
  });
});

describe("weeksLabel", () => {
  it("says a week rather than 1 weeks", () => {
    expect(weeksLabel(1)).toBe("a week");
  });

  it("counts anything longer", () => {
    expect(weeksLabel(2)).toBe("2 weeks");
    expect(weeksLabel(11)).toBe("11 weeks");
  });
});

describe("layoffHeadline", () => {
  it("reads as a sentence at one week and at several", () => {
    expect(layoffHeadline(layoff(1, 0.1))).toBe(
      "It's been a week since you trained",
    );
    expect(layoffHeadline(layoff(3, 0.3))).toBe(
      "It's been 3 weeks since you trained",
    );
  });
});

describe("deloadLabel", () => {
  it("names the cut on the button", () => {
    expect(deloadLabel(layoff(3, 0.3))).toBe("Deload 30%");
  });
});

describe("shouldPrompt", () => {
  it("asks when the server reports a layoff and nothing has been decided", () => {
    expect(shouldPrompt(layoff(2, 0.2), false)).toBe(true);
  });

  it("stays quiet once the lifter has answered either way", () => {
    expect(shouldPrompt(layoff(2, 0.2), true)).toBe(false);
  });

  it("stays quiet when there is no layoff to report", () => {
    expect(shouldPrompt(null, false)).toBe(false);
    expect(shouldPrompt(undefined, false)).toBe(false);
  });

  // Shouldn't reach the client — the server sends null instead — but the
  // component branches on this, and a zero-week banner is a nonsense question.
  it("stays quiet on a zero-week layoff", () => {
    expect(shouldPrompt(layoff(0, 0), false)).toBe(false);
  });
});
