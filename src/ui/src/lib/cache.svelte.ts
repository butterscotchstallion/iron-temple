// Stale-while-revalidate cache for route data.
//
// Every route loads in onMount with no memory, so moving between the four tabs
// re-ran each one's requests from scratch and the screen fell back to skeletons
// for data that had not changed since a moment ago. This keeps the last good
// answer per key, so a revisit paints immediately and the request that follows
// only corrects it.
//
// Plain Maps rather than `$state`: a route copies what it reads into its own
// reactive state, exactly as it already did with a response. Making the cache
// itself reactive would put two owners on the same value — the route's `$state`
// and the cache's — and the route's is the one the template reads.
//
// Nothing here expires by time. The revalidation on every mount IS the freshness
// mechanism; a TTL would only decide how long to also show nothing.

/** The shape every generated client call resolves to. */
type ApiResult<T> = { data?: T; error?: unknown };

/** Last good value per key. Survives unmount, cleared on sign-out. */
const values = new Map<string, unknown>();

/**
 * In-flight request per key.
 *
 * This is what makes prefetching work rather than merely duplicating: the app
 * shell can start a request at startup and the route that wants it — mounting a
 * moment later, once /me has settled — joins the same promise instead of firing
 * a second one.
 */
const pending = new Map<string, Promise<ApiResult<unknown>>>();

/** Keys are spelled out here so two callers cannot disagree about one. */
export const CACHE_KEYS = {
  /** listSessions at the limit Home asks for. */
  homeSessions: "home:sessions",
  /** listExercises, scoped to lifts with a history. */
  performedExercises: "exercises:performed",
  /** listExercises over the whole library. */
  allExercises: "exercises:all",
  /** The first page of listSessions that History opens on. */
  historyFirstPage: "history:first-page",
} as const;

/**
 * The last good value for a key, or undefined if there has never been one.
 *
 * Undefined is the "nothing to show yet" signal a caller keys its skeleton off,
 * and is deliberately distinct from a cached value that happens to be empty: a
 * lifter with no sessions should see the empty state, not a spinner.
 */
export function cachedValue<T>(key: string): T | undefined {
  return values.get(key) as T | undefined;
}

/**
 * Run a request through the cache: dedupe it against one already in flight, and
 * remember the result if it succeeds.
 *
 * The result is returned untouched, errors included, so callers keep the
 * `{ data, error }` handling they already had. Only successes are stored — a
 * failed revalidation must not evict a good answer, because the alternative is
 * replacing what the lifter is reading with an error card over a network blip.
 */
export function fetchThrough<T>(
  key: string,
  call: () => Promise<ApiResult<T>>,
): Promise<ApiResult<T>> {
  const inFlight = pending.get(key) as Promise<ApiResult<T>> | undefined;
  if (inFlight) return inFlight;

  const request = call()
    .then((result) => {
      if (!result.error && result.data !== undefined) values.set(key, result.data);
      return result;
    })
    .finally(() => pending.delete(key));

  pending.set(key, request as Promise<ApiResult<unknown>>);
  return request;
}

/**
 * Drop a key's remembered value, so the next read shows a skeleton and waits
 * for the server rather than painting something known to be wrong.
 *
 * For mutations whose effect the cached shape cannot be patched to reflect —
 * finishing a session changes the streak, the heatmap and the history page at
 * once, and none of that is derivable from the response.
 */
export function invalidate(key: string): void {
  values.delete(key);
}

/**
 * Drop everything derived from performed work.
 *
 * Training a session moves all three of these at once — the streak and heatmap
 * on Home, the list and lifetime volume on History, which lifts have a history
 * and what their heaviest set is on Progress — and none of it can be patched
 * from a mutation's response, because each is an aggregate over the whole
 * history rather than over the session in hand.
 *
 * Called at the two moments that bound a workout rather than after each of the
 * half-dozen writes inside one: when a session is created, and when the active
 * session screen goes away. Every set tap happens between those, so the second
 * catches all of them without a call site per mutation to forget to add.
 */
export function invalidateTraining(): void {
  invalidate(CACHE_KEYS.homeSessions);
  invalidate(CACHE_KEYS.historyFirstPage);
  invalidate(CACHE_KEYS.performedExercises);
}

/**
 * Forget everything. Called on sign-out: the cache holds one lifter's training
 * history, and the next person to use this browser must not see any of it.
 */
export function clearCache(): void {
  values.clear();
  pending.clear();
}
