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
//
// The values are mirrored into sessionStorage so a RELOAD paints from them too.
// That is not a marginal case here: taking an offered update reloads the app,
// and so does every pull-to-refresh on a phone, both of which used to drop the
// lot and start from skeletons. sessionStorage rather than localStorage because
// the lifetime wanted is exactly a tab's — it goes away when the tab does, and
// is not shared with other tabs that may be signed in as someone else.

import changelog from "virtual:iron-temple/changelog";

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

/**
 * Which generation of the cache we are in. Bumped by clearCache().
 *
 * Clearing the two Maps is not enough on its own, because it cannot reach a
 * request that is already in the air: that request's `.then` would run after
 * the clear and write the OLD lifter's data straight back into the emptied
 * cache, where the next person to sign in would read it. A request records the
 * generation it started in and stays silent if it no longer matches.
 */
let generation = 0;

/**
 * Namespace for the mirrored copies, per build.
 *
 * The version is baked in at build time (see changelogVirtualModule in
 * vite.config.ts), which makes it exactly the right namespace: a release that
 * changes a response shape also changes this string, so the new build cannot
 * hydrate the old build's values and paint something it can no longer read.
 * Entries under the old prefix are inert and go when the tab does.
 */
const STORAGE_PREFIX = `iron-temple:cache:${changelog.version}:`;

/**
 * sessionStorage, or null where it cannot be used.
 *
 * Access itself can throw — Safari's private mode has historically thrown on
 * write, and an embedded context can deny storage outright. The cache has to
 * work without it, so every use goes through here and every failure downgrades
 * to "in memory only" rather than taking a page down for a nicety.
 */
function storage(): Storage | null {
  try {
    return window.sessionStorage;
  } catch {
    return null;
  }
}

/** Mirror one value out, or forget the attempt if the browser objects. */
function persist(key: string, value: unknown): void {
  const store = storage();
  if (!store) return;
  try {
    store.setItem(STORAGE_PREFIX + key, JSON.stringify(value));
  } catch {
    // Out of quota, or storage denied. The in-memory copy is unaffected, so
    // this costs a reload's head start and nothing else.
  }
}

/** Read every mirrored value for this build back into memory. */
function hydrate(): void {
  const store = storage();
  if (!store) return;
  try {
    for (let i = 0; i < store.length; i += 1) {
      const stored = store.key(i);
      if (!stored?.startsWith(STORAGE_PREFIX)) continue;
      const raw = store.getItem(stored);
      if (raw === null) continue;
      values.set(stored.slice(STORAGE_PREFIX.length), JSON.parse(raw));
    }
  } catch {
    // Anything unreadable — a truncated write, a hand-edited entry — is not
    // worth a broken page. Start empty and let the routes fetch.
    values.clear();
  }
}

hydrate();

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

/** Prefix for the Racked report's per-period entries, so they can be dropped together. */
const RACKED_PREFIX = "racked:";

/**
 * The Racked recap is a different report per period, so it gets a key each
 * rather than one that three periods overwrite in turn — switching week/month/
 * year and back should be instant, and it is the most expensive read the app
 * makes.
 */
export function rackedKey(period: string): string {
  return RACKED_PREFIX + period;
}

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
 * `{ data, error }` handling they already had — including a caller from a
 * signed-out session, whose component has already been unmounted and whose
 * assignments therefore go nowhere. Only successes are stored, and only for the
 * generation that asked: a failed revalidation must not evict a good answer,
 * because the alternative is replacing what the lifter is reading with an error
 * card over a network blip.
 */
export function fetchThrough<T>(
  key: string,
  call: () => Promise<ApiResult<T>>,
): Promise<ApiResult<T>> {
  const inFlight = pending.get(key) as Promise<ApiResult<T>> | undefined;
  if (inFlight) return inFlight;

  const startedIn = generation;
  const request: Promise<ApiResult<T>> = call()
    .then((result) => {
      // Landing after a sign-out means this answer belongs to whoever was
      // signed in when it was asked for, and storing it would undo the clear.
      if (generation !== startedIn) return result;
      if (!result.error && result.data !== undefined) {
        values.set(key, result.data);
        persist(key, result.data);
      }
      return result;
    })
    .finally(() => {
      // Only ever retire OUR OWN slot. A clear empties `pending` outright, so
      // by the time this runs the key may already belong to a newer request —
      // deleting that one would silently switch off deduping for the key.
      if (pending.get(key) === request) pending.delete(key);
    });

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
  storage()?.removeItem(STORAGE_PREFIX + key);
  // Deliberately does NOT touch the generation. A request in flight across an
  // invalidate belongs to the same lifter, so letting it cache its answer is at
  // worst a few seconds stale and self-corrects on the next mount. The
  // generation is for the one case where the old answer must never be stored at
  // all, which is a change of who is signed in.
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
  // Every period's recap is a summary of performed work, so all of them move.
  // Dropped by prefix because the keys are per period and a workout does not
  // announce which ones it lands in — training on the 1st changes the week, the
  // month and the year at once.
  for (const key of [...values.keys()]) {
    if (key.startsWith(RACKED_PREFIX)) invalidate(key);
  }
}

/**
 * Forget everything. Called on sign-out: the cache holds one lifter's training
 * history, and the next person to use this browser must not see any of it.
 *
 * The generation bump is what makes that a guarantee rather than a hope.
 * Emptying the Maps only forgets answers that have already arrived; a request
 * still in the air would otherwise resolve a moment later and write the
 * previous lifter's data back into the cache it was just cleared from, ready
 * for the next person's first paint to read.
 */
export function clearCache(): void {
  for (const key of values.keys()) storage()?.removeItem(STORAGE_PREFIX + key);
  values.clear();
  pending.clear();
  generation += 1;
}
