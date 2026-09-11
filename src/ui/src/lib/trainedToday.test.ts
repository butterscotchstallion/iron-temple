import { describe, it, expect } from "vitest";
import { isTodaysWorkoutDone, todayStatus } from "./trainedToday";

const TODAY = "2026-09-11";
// 2026-09-11 is a Friday. The two have to agree: isTodaysWorkoutDone asks both
// "is this day scheduled for today" and "was it trained today".
const FRIDAY = 5;

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

describe("isTodaysWorkoutDone", () => {
  const friday = { id: 7, weekday: FRIDAY };

  function done(day: { id: number; weekday: number | null }, sessions = [session()]) {
    return isTodaysWorkoutDone(sessions, day, FRIDAY, TODAY);
  }

  it("hides today's day once it is trained to completion", () => {
    expect(done(friday)).toBe(true);
  });

  it("hides today's day when the session was closed out short", () => {
    expect(done(friday, [session({ completedSetCount: 18, isOver: true })])).toBe(true);
  });

  it("keeps today's day when nothing has been logged", () => {
    expect(done(friday, [])).toBe(false);
  });

  it("keeps today's day while its session is still open", () => {
    expect(done(friday, [session({ completedSetCount: 3, isOver: false })])).toBe(false);
  });

  it("keeps today's day when only yesterday's session is in the list", () => {
    expect(done(friday, [session({ performedOn: "2026-09-10" })])).toBe(false);
  });

  it("keeps a day scheduled for another weekday, trained today or not", () => {
    expect(done({ id: 7, weekday: 3 })).toBe(false);
  });

  it("keeps an unscheduled day trained today", () => {
    expect(done({ id: 7, weekday: null })).toBe(false);
  });

  it("keeps today's day when it was another day of the program that got trained", () => {
    expect(done(friday, [session({ programDayId: 8 })])).toBe(false);
  });

  it("defaults to the browser's weekday and date", () => {
    const now = new Date();
    const iso = [
      now.getFullYear(),
      String(now.getMonth() + 1).padStart(2, "0"),
      String(now.getDate()).padStart(2, "0"),
    ].join("-");
    const today = { id: 7, weekday: now.getDay() };
    expect(isTodaysWorkoutDone([session({ performedOn: iso })], today)).toBe(true);
  });
});
