// Persistence for a running rest countdown, so a reload doesn't cost the lifter
// their rest.
//
// Everything else in the active session is already on the server — each set tap
// is a request. The countdown is the exception: it lives entirely in the
// component, so a reload used to put someone forty seconds into a rest back at a
// stopped 3:00. This is what makes taking an update mid-workout free, and it
// covers the accidental refresh and the phone evicting a backgrounded tab too.
//
// Two deliberate choices:
//
//   - Only a RUNNING countdown is stored, as an absolute deadline. Ticks
//     therefore cost no writes (the deadline doesn't move), and staleness needs
//     no expiry rule: a deadline in the past simply reads as "no rest in
//     progress" and the timer comes up fresh rather than at a stopped 0:00.
//   - sessionStorage, not localStorage. Per-tab, survives a reload, and clears
//     itself when the tab closes — a rest from yesterday is not a rest.

const PREFIX = "iron-temple:rest:";

/**
 * Storage can throw rather than merely fail: Safari's private mode raises on
 * write, and an embedded webview may deny access outright. A lost countdown is
 * not worth an exception, so every access is best-effort.
 */
function storage(): Storage | null {
  try {
    return window.sessionStorage;
  } catch {
    return null;
  }
}

/** Remember that a countdown is running, and when it ends (epoch ms). */
export function saveRest(key: string, endsAt: number): void {
  try {
    storage()?.setItem(PREFIX + key, String(endsAt));
  } catch {
    // Quota or a denied store — the countdown just won't survive a reload.
  }
}

/**
 * Seconds left on a stored countdown, or null if there isn't a live one.
 *
 * `now` is a parameter rather than a Date.now() call so the behaviour around the
 * deadline is testable without fake timers.
 */
export function loadRest(key: string, now: number): number | null {
  let raw: string | null = null;
  try {
    raw = storage()?.getItem(PREFIX + key) ?? null;
  } catch {
    return null;
  }
  if (raw === null) return null;

  const endsAt = Number(raw);
  // A hand-edited or half-written value is not a rest.
  if (!Number.isFinite(endsAt)) {
    clearRest(key);
    return null;
  }

  // Round up, so a rest with 400ms left reads as one second rather than zero —
  // the same direction the on-screen countdown rounds.
  const remaining = Math.ceil((endsAt - now) / 1000);
  if (remaining <= 0) {
    clearRest(key);
    return null;
  }
  return remaining;
}

/** Forget the countdown — it finished, was reset, or was stopped by hand. */
export function clearRest(key: string): void {
  try {
    storage()?.removeItem(PREFIX + key);
  } catch {
    // Nothing to do; a stale entry expires on its own deadline anyway.
  }
}
