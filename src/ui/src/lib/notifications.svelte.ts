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
 */
const PAGE = 20;

/** Ask again this often while the tab is in the foreground. */
const POLL_MS = 60 * 1000;

/**
 * Floor on how often returning to the tab can trigger a poll, so flicking
 * between tabs is not a request per flick. Copied from version.svelte.ts, which
 * learned it first.
 */
const REFOCUS_MIN_GAP_MS = 30 * 1000;

let inFlight = false;
let lastPollAt = 0;

/**
 * Read the panel once.
 *
 * Failures leave the last good list in place and raise `failed`. A notification
 * nobody could fetch is not an error worth interrupting anybody over — the
 * header is on every screen, including the one somebody is mid-set on — so this
 * never throws and never clears what it already had.
 */
export async function poll(): Promise<void> {
  if (inFlight) return;
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
}
