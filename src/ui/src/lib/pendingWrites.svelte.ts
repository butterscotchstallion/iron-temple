// How many writes are in the air right now, so a reload can wait for them.
//
// The active session logs straight to the server — every set tap, weight nudge
// and weigh-in is a request, and there is no local draft of the workout to lose.
// The one thing a reload *can* destroy is a request that hasn't come back yet:
// tap a rep, take the update, and that rep was never recorded anywhere.
//
// So mutating calls are wrapped in track() and the update prompt holds the
// reload until the count reaches zero. Reads are deliberately not tracked —
// nothing is lost by abandoning a GET.

let pending = $state(0);

// Resolvers waiting on quiescence, flushed together when the count hits zero.
let idleWaiters: Array<() => void> = [];

/** In-flight tracked writes. Reactive — safe to read from a template. */
export function pendingCount(): number {
  return pending;
}

/**
 * Count a write while it's in flight. Returns the promise untouched, so a call
 * site only has to wrap: `await track(updateSessionSet(...))`.
 *
 * Rejections still decrement — a failed write is finished, and the caller's own
 * error handling is unaffected because the rejection is passed straight through.
 */
export function track<T>(promise: Promise<T>): Promise<T> {
  pending += 1;
  return promise.finally(() => {
    pending -= 1;
    if (pending === 0) {
      const waiters = idleWaiters;
      idleWaiters = [];
      for (const resolve of waiters) resolve();
    }
  });
}

/**
 * Resolve once nothing is in flight.
 *
 * The timeout is a ceiling, not a deadline: a request that never settles — a
 * dropped connection on a gym wifi — must not strand someone in a dialog that
 * won't close. Reloading a second late is a much better failure than not
 * reloading at all, and the worst case is losing the one rep we were waiting on,
 * which is exactly where we'd have been without any of this.
 */
export function whenIdle(timeoutMs = 5000): Promise<void> {
  if (pending === 0) return Promise.resolve();

  return new Promise<void>((resolve) => {
    let settled = false;
    const done = () => {
      if (settled) return;
      settled = true;
      clearTimeout(handle);
      resolve();
    };
    const handle = setTimeout(done, timeoutMs);
    idleWaiters.push(done);
  });
}

/** Test seam: drop all state so one spec's in-flight writes can't leak. */
export function resetPendingWrites(): void {
  pending = 0;
  idleWaiters = [];
}
