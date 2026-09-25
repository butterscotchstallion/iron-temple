import { describe, expect, it } from "vitest";
import { formatOrdinal, formatOutOf, formatPRGain, formatPace } from "./recap";

describe("formatPace", () => {
  // The sign flip is the whole reason this function exists rather than a call
  // to formatDelta: a negative deltaPct means the workout was QUICKER.
  it("reads a negative delta as faster", () => {
    expect(formatPace(-0.12)).toBe("12% faster");
  });

  it("reads a positive delta as slower", () => {
    expect(formatPace(0.08)).toBe("8% slower");
  });

  // A 48-minute workout swinging 2% is under a minute — one conversation by
  // the rack, not a change in pace.
  it("declines to call a small swing anything", () => {
    expect(formatPace(0.02)).toBe("about the same");
    expect(formatPace(-0.049)).toBe("about the same");
    expect(formatPace(0)).toBe("about the same");
  });

  it("takes the band's edge as a real difference", () => {
    expect(formatPace(-0.05)).toBe("5% faster");
    expect(formatPace(0.05)).toBe("5% slower");
  });

  it("says nothing rather than NaN on bad input", () => {
    expect(formatPace(Number.NaN)).toBe("about the same");
    expect(formatPace(Number.POSITIVE_INFINITY)).toBe("about the same");
  });
});

describe("formatOrdinal", () => {
  it("suffixes by last digit", () => {
    expect(formatOrdinal(1)).toBe("1st");
    expect(formatOrdinal(2)).toBe("2nd");
    expect(formatOrdinal(3)).toBe("3rd");
    expect(formatOrdinal(4)).toBe("4th");
    expect(formatOrdinal(21)).toBe("21st");
    expect(formatOrdinal(102)).toBe("102nd");
  });

  // The teens are the only trap: they do not follow their last digit.
  it("handles the teens", () => {
    expect(formatOrdinal(11)).toBe("11th");
    expect(formatOrdinal(12)).toBe("12th");
    expect(formatOrdinal(13)).toBe("13th");
    expect(formatOrdinal(111)).toBe("111th");
  });

  it("has nothing to say about a non-placing", () => {
    expect(formatOrdinal(0)).toBe("—");
    expect(formatOrdinal(Number.NaN)).toBe("—");
  });
});

describe("formatPRGain", () => {
  it("measures the record against the mark it replaced", () => {
    expect(formatPRGain(245, 240)).toBe("+2%");
    expect(formatPRGain(300, 200)).toBe("+50%");
  });

  // A previousLb of 0 is a record over a prior best of zero, and a rise from
  // nothing is a ratio with no meaning. Live rather than defensive: bodyweight
  // work is recorded at 0 lb, so a chin-up IS in the history with a best of 0,
  // and the first set done wearing a belt is a genuine weight record against it.
  // Divide instead of guarding and a share card goes out reading "+Infinity%".
  it("says nothing about a record over a prior best of zero", () => {
    expect(formatPRGain(25, 0)).toBeNull();
  });

  // "+0%" against a row announcing a personal record reads as a contradiction.
  // Reachable in practice: one extra rep can move an estimated max by a pound.
  it("stays quiet about a gain that rounds away", () => {
    expect(formatPRGain(234, 233)).toBeNull();
    expect(formatPRGain(240.5, 240)).toBeNull();
  });

  it("reports the smallest gain that still rounds to a percent", () => {
    expect(formatPRGain(242, 240)).toBe("+1%");
  });

  it("has nothing to say about nonsense", () => {
    expect(formatPRGain(Number.NaN, 240)).toBeNull();
    expect(formatPRGain(245, Number.NaN)).toBeNull();
    expect(formatPRGain(245, -10)).toBeNull();
  });
});

describe("formatOutOf", () => {
  it("pairs what was done with what was asked", () => {
    expect(formatOutOf(23, 25)).toBe("23 / 25");
    expect(formatOutOf(0, 25)).toBe("0 / 25");
  });

  // A session with no prescription is not "0 / 0", which reads as a failure.
  it("has nothing to show without a denominator", () => {
    expect(formatOutOf(0, 0)).toBe("—");
    expect(formatOutOf(5, Number.NaN)).toBe("—");
  });
});
