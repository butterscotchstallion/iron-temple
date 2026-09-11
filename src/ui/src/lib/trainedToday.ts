import type { SessionSummary } from "./api";
import { todayIso } from "./calendar";
import { isSessionComplete } from "./streak";
import { todayWeekday } from "./weekday";

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
 * Whether this day is the one scheduled for today and the work is already done.
 *
 * The program screen drops such a card entirely. Once today's workout is behind
 * you it is no longer a decision you have to make, and a card is a screenful of
 * prescribed weights offering to start something you just finished — the badge
 * and the demoted "Start again" said so, but they still asked to be read. What
 * the screen should show is what's left, which on a rest day is the rest of the
 * program and on a done day is the same.
 *
 * Only the day actually scheduled for today. A day trained off its weekday —
 * Workout B on the Wednesday Workout A is booked for — keeps its card and its
 * "Done today" badge: the schedule never claimed it was today's work, and
 * hiding it would quietly shorten the program the lifter came here to look at.
 * An unscheduled day is never today's, so it is never hidden.
 *
 * Done means finished, not merely started. A session left open keeps its card,
 * because that workout is still ahead of the lifter and Resume is the whole
 * reason the card is worth the room.
 *
 * `weekday` and `today` default to the browser's clock and are injectable for
 * testing; they must agree, so a caller overriding one should override both.
 */
export function isTodaysWorkoutDone(
  sessions: Trainable[],
  day: Scheduled,
  weekday: number = todayWeekday(),
  today: string = todayIso(),
): boolean {
  if (day.weekday !== weekday) return false;
  return todayStatus(sessions, day.id, today)?.done === true;
}
