import { describe, it, expect } from "vitest";
import { nextDueOn, todayStatus } from "./trainedToday";

const TODAY = "2026-09-11";
// 2026-09-11 is a Friday.
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

describe("nextDueOn", () => {
  // 2026-09-11 is a Friday. Passing one Date keeps the weekday arithmetic and
  // the returned date on the same clock, which is the bug the old two-argument
  // shape invited.
  const FRI_11 = new Date(2026, 8, 11);

  function due(weekday: number | null, sessions: ReturnType<typeof session>[] = []) {
    return nextDueOn(sessions, { id: 7, weekday }, FRI_11);
  }

  it("is null for an unscheduled day, trained today or not", () => {
    expect(due(null)).toBeNull();
    expect(due(null, [session()])).toBeNull();
  });

  it("is today for the day scheduled today with nothing logged", () => {
    expect(due(FRIDAY)).toBe("2026-09-11");
  });

  it("counts the days ahead to a later weekday in the same week", () => {
    expect(due(0)).toBe("2026-09-13"); // Sunday
  });

  it("wraps to next week for a weekday already past", () => {
    expect(due(3)).toBe("2026-09-16"); // Wednesday
  });

  it("pushes today's day to next week once it is trained to completion", () => {
    expect(due(FRIDAY, [session()])).toBe("2026-09-18");
  });

  it("pushes it out when the session was closed out short of every set", () => {
    expect(due(FRIDAY, [session({ completedSetCount: 18, isOver: true })])).toBe(
      "2026-09-18",
    );
  });

  it("keeps today's day due today while its session is still open", () => {
    expect(due(FRIDAY, [session({ completedSetCount: 3, isOver: false })])).toBe(
      "2026-09-11",
    );
  });

  it("ignores yesterday's session for the same day", () => {
    expect(due(FRIDAY, [session({ performedOn: "2026-09-10" })])).toBe("2026-09-11");
  });

  it("ignores a session for another day of the program", () => {
    expect(due(FRIDAY, [session({ programDayId: 8 })])).toBe("2026-09-11");
  });

  it("leaves a day trained off its own weekday due on its own weekday", () => {
    // Trained today (Friday), but this day is booked for Wednesday. The schedule
    // never said Friday was its day, so it is still next due on Wednesday.
    expect(due(3, [session()])).toBe("2026-09-16");
  });

  it("crosses a month boundary", () => {
    // Monday 2026-09-28 + a week is October.
    const mon28 = new Date(2026, 8, 28);
    expect(nextDueOn([], { id: 7, weekday: 0 }, mon28)).toBe("2026-10-04");
  });

  it("defaults to the browser's clock", () => {
    const now = new Date();
    const iso = [
      now.getFullYear(),
      String(now.getMonth() + 1).padStart(2, "0"),
      String(now.getDate()).padStart(2, "0"),
    ].join("-");
    expect(nextDueOn([], { id: 7, weekday: now.getDay() })).toBe(iso);
  });

  // The behaviour the screen is actually built on: a two-day program keeps
  // showing two upcoming workouts after one of them is done today.
  it("orders a finished day behind the rest of the week", () => {
    const tue = { id: 1, weekday: 2 };
    const fri = { id: 2, weekday: FRIDAY };
    const trainedFri = [session({ programDayId: 2 })];
    const dues = [tue, fri]
      .map((d) => ({ id: d.id, on: nextDueOn(trainedFri, d, FRI_11) }))
      .sort((a, b) => String(a.on).localeCompare(String(b.on)));
    expect(dues).toEqual([
      { id: 1, on: "2026-09-15" }, // Tuesday, the rest of this week
      { id: 2, on: "2026-09-18" }, // Friday, the start of the next
    ]);
  });
});
