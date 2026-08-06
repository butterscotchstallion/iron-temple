import type { SessionSummary } from "./api";

/** The shape needed to judge a session — a subset of SessionSummary. */
type Progressable = Pick<SessionSummary, "setCount" | "completedSetCount">;

/** A session counts as completed when every prescribed set was logged. */
export function isSessionComplete(session: Progressable): boolean {
  return session.setCount > 0 && session.completedSetCount === session.setCount;
}

/**
 * The current streak: the number of consecutive completed sessions ending at
 * the most recent one. `sessions` must be ordered most-recent-first (as
 * listSessions returns them). The first non-completed session breaks the run,
 * so finishing every set of your latest workout keeps the streak alive.
 */
export function currentStreak(sessions: Progressable[]): number {
  let streak = 0;
  for (const session of sessions) {
    if (!isSessionComplete(session)) break;
    streak += 1;
  }
  return streak;
}

/**
 * A streak is only worth surfacing once it exceeds two sessions in a row, per
 * the product rule. Below that, there's nothing to show.
 */
export const STREAK_DISPLAY_THRESHOLD = 3;

export function hasStreak(sessions: Progressable[]): boolean {
  return currentStreak(sessions) >= STREAK_DISPLAY_THRESHOLD;
}
