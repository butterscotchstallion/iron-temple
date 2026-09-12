import type { SessionSummary } from "./api";
import { addDaysIso, todayIso } from "./calendar";
import { isSessionComplete } from "./streak";

/**
 * The shape this module needs off a session — a subset of SessionSummary, so a
 * test can state one in four fields rather than twelve.
 */
type Trainable = Pick<
  SessionSummary,
  "id" | "programDayId" | "performedOn" | "setCount" | "completedSetCount" | "isOver"
>;

/** What today's session, if there is one, means for a program day's card. */
export type TodayStatus = {
  /** The session to open — the one that was trained, or the one still going. */
  sessionId: number;
  /**
   * True once the work is behind you: every prescribed set logged, or the
   * session closed out. False means it is open and can be picked back up.
   */
  done: boolean;
};

/**
 * Whether this program day has already been trained today, and how far.
 *
 * `sessions` is the home screen's list — most recent first, which is what makes
 * "the first match" the right one to return. Today is the browser's local date,
 * matching every other date judgement the client makes (the "Today" badge's
 * weekday, the heatmap's end date); the server stamps performedOn from its own
 * clock, so a lifter training either side of midnight in a zone far from the
 * server's may see the badge a day out.
 *
 * A finished session outranks an unfinished one for the same day. Pressing
 * "Start again" leaves the first session in the list, and the honest thing to
 * say about a day trained to completion is that it is done — not to offer to
 * resume the second attempt that was abandoned after one set.
 *
 * Note that a session created and then left with nothing logged does not appear
 * in this list at all: the API counts a session as started only once it has a
 * rep in it. Such a day reads as untrained, which is also what it is.
 */
export function todayStatus(
  sessions: Trainable[],
  programDayId: number,
  today: string = todayIso(),
): TodayStatus | null {
  const forDay = sessions.filter(
    (s) => s.programDayId === programDayId && s.performedOn === today,
  );
  if (forDay.length === 0) return null;

  const finished = forDay.find((s) => isSessionComplete(s) || s.isOver);
  if (finished) return { sessionId: finished.id, done: true };
  return { sessionId: forDay[0].id, done: false };
}

/** The bit of a program day that places it in the week. */
type Scheduled = { id: number; weekday: number | null };

/**
 * The date this program day next comes round, as YYYY-MM-DD — or null when it
 * has no weekday and so is not due anything.
 *
 * This is what orders the program screen. Sorting the days by it turns the page
 * into a queue: whatever is soonest leads, and the top two cards are always the
 * next two workouts. That holds after training, which is the point — finish
 * today's session and the card does not vanish, it moves to the back with next
 * week's date on it, so a two-day program still shows the rest of this week and
 * the start of the next rather than dwindling to a single card.
 *
 * Done today means this day has been and gone for the week, so it is due again
 * on the same weekday next week. An OPEN session is still due TODAY: that
 * workout is ahead of the lifter and Resume is the point of the card. Finished
 * outranks open for the same day — see todayStatus.
 *
 * A day trained off its scheduled weekday — Workout B on the Wednesday Workout A
 * is booked for — is untouched here. It is still due on its own weekday, because
 * the schedule never claimed Wednesday was its day; what it gets is the "Done
 * today" badge, which is a statement about today rather than about the schedule.
 *
 * `today` is one Date rather than a weekday and a date string, so the two cannot
 * disagree — the weekday arithmetic and the returned date are read off the same
 * clock. Local time throughout, matching every other date judgement the client
 * makes; the server stamps performedOn from its own clock, so a lifter training
 * either side of midnight far from the server's zone may see this a day out.
 */
export function nextDueOn(
  sessions: Trainable[],
  day: Scheduled,
  today: Date = new Date(),
): string | null {
  if (day.weekday === null) return null;

  // 0 when the day is scheduled for today, else how many days until it comes up.
  const daysAhead = (day.weekday - today.getDay() + 7) % 7;
  const done =
    daysAhead === 0 &&
    todayStatus(sessions, day.id, addDaysIso(today, 0))?.done === true;

  return addDaysIso(today, done ? 7 : daysAhead);
}
