import type { SessionSummary } from "./api";
import { todayIso } from "./calendar";
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
