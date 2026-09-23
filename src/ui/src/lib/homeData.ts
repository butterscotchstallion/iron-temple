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
    listSessions({ limit: HOME_SESSION_LIMIT }),
  );
}

// WATCHING THE LIST WHILE THE MAIN SCREEN IS OPEN
//
// The main screen used to read this list once, in onMount, and then never ask
// again — so a tab left open on it stayed at the moment it was opened. Finish
// the workout on a phone at the rack and the laptop still offered "Resume" on a
// day that was done, still ringed it as due, still dated it today, and still
// counted yesterday's streak. Nothing was wrong with any of it except its age.
//
// Everything on that screen comes off this one list, so re-reading it is the
// whole fix: Home recomputes its streak and heatmap, and ProgramDetail's cards
// re-derive their badge, their button, their ring and their order.
//
// ONE INTERVAL, NOT ONE PER SCREEN
//
// Home renders ProgramDetail inside itself, so both are mounted together and
// both want the same answer. A poller per component would be two requests a
// tick asking the same question. Listeners share one interval and one request,
// and each copies the result into its own `$state` exactly as it already does
// with a response — which is the rule cache.svelte.ts states in its header:
// the cache stays a plain Map, and the route owns the reactive copy.

/** How often the main screen re-reads the list while it is on screen. */
const WATCH_MS = 30 * 1000;

/**
 * Floor on how often returning to the tab can trigger a read, so flicking
 * between tabs is not a request per flick. Same idea, and same reason, as
 * version.svelte.ts and notifications.svelte.ts.
 */
const REFOCUS_MIN_GAP_MS = 15 * 1000;

const listeners = new Set<(data: HomeSessions) => void>();
let handle: ReturnType<typeof setInterval> | null = null;
let inFlight = false;
let lastPollAt = 0;

/**
 * Read the list once and hand it to everybody watching.
 *
 * Failures are swallowed and the listeners are left holding what they already
 * had. A revalidation that didn't land is not news — the screen is showing the
 * last answer, which is exactly where it would have been without this — and
 * blanking a streak over a network blip would be strictly worse than a stale
 * one.
 *
 * Deliberately does NOT call observe() from connectivity.svelte.ts. This is a
 * background read nobody asked for; letting it drive the offline banner would
 * raise it behind a lifter who has not touched anything and cannot act on it.
 * The banner belongs to requests somebody actually made.
 */
async function poll(): Promise<void> {
  if (inFlight) return;
  inFlight = true;
  try {
    const result = await loadHomeSessions();
    if (result.status !== 200) return;
    for (const listener of listeners) listener(result.data);
  } finally {
    // Stamped even on failure, so a refocus loop cannot turn an unreachable API
    // into a request per tab switch.
    lastPollAt = Date.now();
    inFlight = false;
  }
}

const tick = () => {
  // A backgrounded phone browser has nobody to correct a badge for.
  if (document.visibilityState !== "visible") return;
  void poll();
};

// Coming back to a tab left open on this screen is exactly when it is most
// likely to be wrong — walking from the rack to the desk is the case this whole
// thing is for — so don't wait out the interval.
const onVisibility = () => {
  if (document.visibilityState !== "visible") return;
  if (Date.now() - lastPollAt < REFOCUS_MIN_GAP_MS) return;
  void poll();
};

/**
 * Watch the list for as long as the caller is on screen. Returns a teardown.
 *
 * Route-scoped rather than started from App.svelte like the version and
 * notification pollers, because the answer is only worth anything on the screen
 * that draws it — History and Progress should not pay for a request they have
 * no badge to correct.
 *
 * Cheap on the wire despite the interval: GET /sessions sits under the API's
 * jsonETag middleware, which sends `max-age=0, must-revalidate`, so the browser
 * revalidates every time and an unchanged list comes back a bodiless 304.
 *
 * No immediate read on subscribe — the route's own onMount load just made one.
 *
 * The two handlers above are module-level rather than per-subscriber on
 * purpose: removeEventListener matches by identity, so a fresh closure per call
 * would mean the last teardown removing a listener that was never added and
 * leaving the real one attached for the life of the tab.
 */
export function watchHomeSessions(
  onFresh: (data: HomeSessions) => void,
): () => void {
  listeners.add(onFresh);
  if (handle === null) {
    handle = setInterval(tick, WATCH_MS);
    document.addEventListener("visibilitychange", onVisibility);
  }

  return () => {
    listeners.delete(onFresh);
    if (listeners.size > 0 || handle === null) return;
    clearInterval(handle);
    handle = null;
    document.removeEventListener("visibilitychange", onVisibility);
  };
}

/** Test seam: drop the watch state so one spec's poller can't leak into the next. */
export function resetHomeWatch(): void {
  if (handle !== null) {
    clearInterval(handle);
    handle = null;
    document.removeEventListener("visibilitychange", onVisibility);
  }
  listeners.clear();
  inFlight = false;
  lastPollAt = 0;
}
