/**
 * A clock the UI can react to.
 *
 * Relative timestamps — "just now", "2 days ago" — are the only text in this app
 * that goes stale while nobody touches anything. A comment posted during a
 * session reads "just now" for as long as the recap stays open, which is the
 * whole session; the notification panel is the screen people leave open longest
 * and the one where recency is the entire point. Formatting once at render is
 * what makes that happen, so the fix is a clock that ticks and a `$state` that
 * every timestamp reads.
 *
 * One clock, not one timer per timestamp. A notification panel can hold thirty
 * of them, and thirty intervals that all say the same thing is thirty wakeups a
 * tick for one number.
 */

/**
 * Two ticks a minute, not one.
 *
 * "just now" is true for sixty seconds, so a sixty-second tick can leave it on
 * screen for a hundred and twenty — the tick that should retire it fires just
 * before it expires, and the next one is a minute later. Thirty seconds halves
 * the worst case for two wakeups a minute, and no rung of the ladder above
 * minutes can drift visibly in thirty seconds.
 */
const TICK_MS = 30_000;

/**
 * Start from the real clock rather than from zero: a component may read this
 * during its first render, before the `$effect` that subscribes has run.
 */
let now = $state(Date.now());

/**
 * Refcounted, so the interval exists only while something is displaying a
 * timestamp. Most screens in this app show none, and a timer that ticks behind
 * a program editor is pure battery. The precedent is watchSession() in
 * SessionSocial.svelte, which is refcounted for the same reason.
 */
let subscribers = 0;
let timer: ReturnType<typeof setInterval> | undefined;

/** Reactive. Wall-clock ms, refreshed about twice a minute. */
export function currentTime(): number {
  return now;
}

/**
 * Catch up the moment the tab comes back.
 *
 * A backgrounded tab has its intervals throttled hard — often to once a minute,
 * sometimes to never — so a phone that went in a pocket between sets comes back
 * with every timestamp reading whatever it read when the screen went off. This
 * is the case that matters most in a gym, and it costs one listener.
 */
function catchUp(): void {
  if (document.visibilityState === "visible") now = Date.now();
}

function stop(): void {
  if (timer !== undefined) {
    clearInterval(timer);
    timer = undefined;
  }
  document.removeEventListener("visibilitychange", catchUp);
}

/**
 * Subscribe to the clock. Returns a teardown for `$effect`.
 *
 * Read the current value with currentTime(); this only keeps it fresh.
 */
export function watchClock(): () => void {
  if (typeof window === "undefined") return () => {};

  subscribers += 1;
  if (subscribers === 1) {
    // A fresh reading on the way in, for the first timestamp mounted after a
    // spell with none: `now` is otherwise as old as the last teardown.
    now = Date.now();
    timer = setInterval(() => (now = Date.now()), TICK_MS);
    document.addEventListener("visibilitychange", catchUp);
  }

  // Idempotent, because the refcount is the thing keeping the timer honest: a
  // teardown run twice would drop it to zero early and stop the clock for
  // everybody still watching.
  let released = false;
  return () => {
    if (released) return;
    released = true;
    subscribers -= 1;
    if (subscribers === 0) stop();
  };
}

/** Test seam: put the module back in its initial state. */
export function resetClock(): void {
  subscribers = 0;
  stop();
  now = Date.now();
}
