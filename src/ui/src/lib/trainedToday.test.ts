import { describe, it, expect } from "vitest";
import { todayStatus } from "./trainedToday";

const TODAY = "2026-09-11";

function session(over: Partial<Parameters<typeof todayStatus>[0][number]> = {}) {
  return {
    id: 1,
    programDayId: 7,
    performedOn: TODAY,
    setCount: 25,
    completedSetCount: 25,
    isOver: true,
    ...over,
  };
}

describe("todayStatus", () => {
  it("is null with no sessions at all", () => {
    expect(todayStatus([], 7, TODAY)).toBeNull();
  });

  it("ignores a session for another day of the program", () => {
    expect(todayStatus([session({ programDayId: 8 })], 7, TODAY)).toBeNull();
  });

  it("ignores the same day trained yesterday", () => {
    expect(todayStatus([session({ performedOn: "2026-09-10" })], 7, TODAY)).toBeNull();
  });

  it("reports a fully logged session as done", () => {
    expect(todayStatus([session({ id: 42 })], 7, TODAY)).toEqual({
      sessionId: 42,
      done: true,
    });
  });

  it("counts a session closed out short of every set as done", () => {
    const short = session({ id: 42, completedSetCount: 18, isOver: true });
    expect(todayStatus([short], 7, TODAY)).toEqual({ sessionId: 42, done: true });
  });

  it("reports a part-logged, still-open session as resumable", () => {
    const open = session({ id: 42, completedSetCount: 3, isOver: false });
    expect(todayStatus([open], 7, TODAY)).toEqual({ sessionId: 42, done: false });
  });

  it("prefers the finished session when the day was started twice", () => {
    const abandoned = session({ id: 43, completedSetCount: 1, isOver: false });
    const trained = session({ id: 42 });
    expect(todayStatus([abandoned, trained], 7, TODAY)).toEqual({
      sessionId: 42,
      done: true,
    });
  });

  it("resumes the most recent of two open sessions", () => {
    const newer = session({ id: 43, completedSetCount: 2, isOver: false });
    const older = session({ id: 42, completedSetCount: 1, isOver: false });
    expect(todayStatus([newer, older], 7, TODAY)).toEqual({
      sessionId: 43,
      done: false,
    });
  });

  it("defaults to the browser's today", () => {
    const now = new Date();
    const iso = [
      now.getFullYear(),
      String(now.getMonth() + 1).padStart(2, "0"),
      String(now.getDate()).padStart(2, "0"),
    ].join("-");
    expect(todayStatus([session({ performedOn: iso })], 7)).toEqual({
      sessionId: 1,
      done: true,
    });
  });
});
