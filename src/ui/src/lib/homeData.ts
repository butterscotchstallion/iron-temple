import { listSessions, type SessionList } from "./api";
import { CACHE_KEYS, fetchThrough } from "./cache.svelte";

export type HomeSessions = SessionList;

/**
 * How many sessions the home screen reads. Enough to draw the heatmap and count
 * a streak; the History tab is what paginates.
 */
const HOME_SESSION_LIMIT = 100;

/**
 * The session list the home screen renders from.
 *
 * A function rather than a call site in each place because it has two callers
 * that MUST agree: Home, which reads it, and the app shell, which starts it at
 * launch. They share a cache key, so if the two ever asked for different things
 * the prefetch would quietly stop being a prefetch — it would populate one key
 * while the screen waited on another.
 */
export function loadHomeSessions() {
  return fetchThrough(CACHE_KEYS.homeSessions, () =>
    listSessions({ query: { limit: HOME_SESSION_LIMIT } }),
  );
}
