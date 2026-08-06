import { describe, it, expect } from "vitest";
import { currentStreak, isSessionComplete, hasStreak } from "./streak";

const done = { setCount: 5, completedSetCount: 5 };
const partial = { setCount: 5, completedSetCount: 3 };
const empty = { setCount: 0, completedSetCount: 0 };

describe("isSessionComplete", () => {
  it("is true only when every set was logged", () => {
    expect(isSessionComplete(done)).toBe(true);
    expect(isSessionComplete(partial)).toBe(false);
  });

  it("is false for a session with no sets", () => {
    expect(isSessionComplete(empty)).toBe(false);
  });
});

describe("currentStreak", () => {
  it("is zero with no sessions", () => {
    expect(currentStreak([])).toBe(0);
  });

  it("counts consecutive completed sessions from the most recent", () => {
    expect(currentStreak([done, done, done])).toBe(3);
  });

  it("stops at the first non-completed session", () => {
    expect(currentStreak([done, done, partial, done])).toBe(2);
  });

  it("is zero when the most recent session is incomplete", () => {
    expect(currentStreak([partial, done, done])).toBe(0);
  });
});

describe("hasStreak", () => {
  it("requires more than two completed in a row", () => {
    expect(hasStreak([done, done])).toBe(false);
    expect(hasStreak([done, done, done])).toBe(true);
  });
});
