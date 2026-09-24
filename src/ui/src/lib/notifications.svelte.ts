import {
  clearNotifications as clearNotificationsRequest,
  listNotifications,
  markNotificationRead as markNotificationReadRequest,
  markNotificationsRead as markNotificationsReadRequest,
  type Notification,
} from "./api";

// What happened to the signed-in lifter, and how many of it they haven't seen.
//
// A module-level `$state` object rather than a store, for the reason
// auth.svelte.ts is one: runes make the object itself reactive, so the bell and
// the panel inside it both re-render off a poll without subscriptions.
//
// WHY THE STATE LIVES HERE AND NOT IN THE COMPONENT
//
// The bell is inside HeaderBar, which is lazy-loaded (see deferred.svelte.ts),
// so the component does not exist for the first moments of a page load and is
// torn down and rebuilt by nothing in particular. The polling has to outlive
// that, and App.svelte — which is always mounted — is what owns it. Keeping the
// list here too means opening the panel draws what the last poll already
// fetched instead of showing a spinner for a request that is usually a 304.

// EVERYTHING HERE IS COUNTED IN GROUPS, because the API is. One item is one
// thing that happened — all the applause on a session, all the conversation on
// it, everybody who joined — and `unread` counts those same groups, so the badge
// and the panel can never disagree about how many things are waiting. Nothing in
// this module folds anything itself; it just never assumes an item is a row.

export const notifications = $state<{
  /** The most recent page, newest first. Empty until the first poll lands. */
  items: Notification[];
  /** Unread across ALL of them, not just the page above. The badge. */
  unread: number;
  /** Whether a poll has ever completed, so the panel can tell empty from unknown. */
  loaded: boolean;
  /** Set when the last attempt failed, so the panel can offer a retry. */
  failed: boolean;
}>({
  items: [],
  unread: 0,
  loaded: false,
  failed: false,
});

/**
 * How many rows the panel holds.
 *
 * There is no "load more". A notification panel is a recent-events list, not an
 * archive — anything older than this page is answered by the surface the
 * notification was about, which is still there. Paging it would be building a
 * second feed.
 *
 * Twenty GROUPS, so this is twenty things that happened. It used to be twenty
 * notifications, which a single busy session could spend on its own.
 */
const PAGE = 20;

/** Ask again this often while the tab is in the foreground. */
const POLL_MS = 60 * 1000;

/**
 * How often to ask while something else is watching in real time.
 *
 * The poller does not stop when the live socket connects — it becomes a safety
 * net. A proxy that eats upgrades, a socket the browser has quietly given up
 * on, a bug in the hub: in every one of those the badge has to keep working,
 * and five minutes of staleness in a case that should not happen is a better
 * trade than a badge that silently stops updating.
 *
 * Implemented by SKIPPING ticks rather than by rescheduling the interval, so
 * the teardown and the ownership of the timer stay exactly as they were.
 */
const SOCKET_POLL_MS = 5 * 60 * 1000;

/**
 * Floor on how often returning to the tab can trigger a poll, so flicking
 * between tabs is not a request per flick. Copied from version.svelte.ts, which
 * learned it first.
 */
const REFOCUS_MIN_GAP_MS = 30 * 1000;

let inFlight = false;
let lastPollAt = 0;
let liveBacked = false;

/**
 * Whether something is watching in real time, so this can back off.
 *
 * Told from outside rather than discovered, and this module deliberately knows
 * nothing about WebSockets — which also keeps the dependency pointing one way:
 * live.svelte.ts imports poll() from here, so an import back would be a cycle.
 */
export function setLiveBacked(value: boolean): void {
  liveBacked = value;
}

/**
 * Set when a poll was asked for while one was already running.
 *
 * The in-flight guard used to simply DROP that request, which was invisible
 * while the only caller was a once-a-minute timer and is not now: a burst of
 * pushes can arrive inside one request, and the last of them would be the one
 * swallowed — leaving the badge stale until the next tick with nothing to say
 * it had happened.
 */
let dirty = false;

/**
 * Read the panel once.
 *
 * Failures leave the last good list in place and raise `failed`. A notification
 * nobody could fetch is not an error worth interrupting anybody over — the
 * header is on every screen, including the one somebody is mid-set on — so this
 * never throws and never clears what it already had.
 */
export async function poll(): Promise<void> {
  if (inFlight) {
    dirty = true;
    return;
  }
  inFlight = true;
  try {
    const result = await listNotifications({ limit: PAGE });
    if (result.status !== 200) {
      notifications.failed = true;
      return;
    }
    notifications.items = result.data.items;
    notifications.unread = result.data.unreadCount;
    notifications.loaded = true;
    notifications.failed = false;
  } finally {
    // Stamped even on failure, so a refocus loop cannot turn an unreachable API
    // into a request per tab switch.
    lastPollAt = Date.now();
    inFlight = false;
    // Exactly one catch-up for however many were coalesced: they all asked the
    // same question, and the answer is a whole page either way.
    if (dirty) {
      dirty = false;
      void poll();
    }
  }
}

/**
 * Mark everything read. Optimistic: the badge goes at the tap and the rows are
 * stamped locally, then a poll reconciles.
 *
 * Optimism is safe here in a way it would not be for a training write. The
 * worst case is a badge that comes back on the next poll, and the API is
 * idempotent about this — `readAt` records when a notification was FIRST read,
 * so a retry cannot move it.
 */
export async function markAllRead(): Promise<void> {
  if (notifications.unread === 0) return;
  const stamp = new Date().toISOString();
  notifications.unread = 0;
  notifications.items = notifications.items.map((item) =>
    item.readAt ? item : { ...item, readAt: stamp },
  );
  await markNotificationsReadRequest();
  await poll();
}

/**
 * Mark one read, for a lifter who followed a notification through to what it
 * was about.
 *
 * The other half of `markAllRead`, not a decomposition of it. Opening the panel
 * is a glance and deliberately reads nothing — that is what lets somebody come
 * back to what was new — but following a row to the comment it quotes is
 * exactly the act of having read that one. Without this the only way to clear a
 * badge was the blunt one, which buries the rows that have not been looked at.
 *
 * Optimistic and safe for the same reason `markAllRead` is: the worst case is a
 * dot that comes back on the next poll, and the API records when a notification
 * was FIRST read, so a retry cannot move the stamp.
 *
 * Does nothing for a row already read, so the badge cannot go negative when a
 * lifter opens the same notification twice.
 *
 * The badge comes down by exactly one however many notifications the row folded,
 * which is right on both sides: the server marks the whole group read, and the
 * server was counting that group as one.
 */
export async function markRead(id: number): Promise<void> {
  const item = notifications.items.find((n) => n.id === id);
  if (!item || item.readAt) return;

  const stamp = new Date().toISOString();
  notifications.unread = Math.max(0, notifications.unread - 1);
  notifications.items = notifications.items.map((n) =>
    n.id === id ? { ...n, readAt: stamp } : n,
  );
  await markNotificationReadRequest(id);
}

/**
 * Clear the list. Also optimistic, and also reconciled by the poll that follows.
 *
 * This deletes on the server. What it does not touch is the applause and the
 * conversation it described — those live on their sessions and are still there,
 * which is what makes clearing a safe thing to offer behind a single tap with
 * no confirmation.
 */
export async function clearAll(): Promise<void> {
  if (notifications.items.length === 0) return;
  notifications.items = [];
  notifications.unread = 0;
  await clearNotificationsRequest();
  await poll();
}

/**
 * Start watching. Returns a teardown.
 *
 * Modelled on version.svelte.ts: visibility-gated, in-flight guarded, failures
 * swallowed, teardown returned, mounted from App.svelte with `$effect`. A
 * backgrounded phone browser has nobody to show a badge to.
 *
 * The caller decides WHETHER to start one at all — this does not check whether
 * anybody is signed in, because App.svelte already knows and re-runs the effect
 * when that changes.
 */
export function startPolling(): () => void {
  const tick = () => {
    if (document.visibilityState !== "visible") return;
    // Backed by a socket: this is a safety net, not the mechanism.
    if (liveBacked && Date.now() - lastPollAt < SOCKET_POLL_MS) return;
    void poll();
  };

  // Coming back to a tab left open is exactly when something is most likely to
  // have arrived, so don't wait out the interval.
  const onVisibility = () => {
    if (document.visibilityState !== "visible") return;
    if (Date.now() - lastPollAt < REFOCUS_MIN_GAP_MS) return;
    void poll();
  };

  tick();
  const handle = setInterval(tick, POLL_MS);
  document.addEventListener("visibilitychange", onVisibility);

  return () => {
    clearInterval(handle);
    document.removeEventListener("visibilitychange", onVisibility);
  };
}

/**
 * Forget everything. Called on sign-out, so the next account to use this tab
 * does not see a badge counting somebody else's notifications before the first
 * poll replaces them.
 */
export function resetNotifications(): void {
  notifications.items = [];
  notifications.unread = 0;
  notifications.loaded = false;
  notifications.failed = false;
  dirty = false;
  liveBacked = false;
}
