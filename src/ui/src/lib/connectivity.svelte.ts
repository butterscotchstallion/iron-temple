/**
 * Whether the API is reachable, as opposed to whether the browser thinks it has
 * a network.
 *
 * `navigator.onLine` alone is not good enough here, and the gym is exactly where
 * it falls down: it reports true for a phone associated with an access point
 * that has no working uplink, which is what a basement wifi looks like for the
 * ten minutes before anyone notices. It is reliable in one direction only —
 * false really does mean no network — so it is used as a fast negative signal
 * and never as a positive one.
 *
 * The positive signal is a request actually landing. Every tracked call reports
 * back through here, so "online" means "the API answered us", which is the only
 * form of the question the app cares about.
 */

/** The shape every generated client call resolves to, as far as this cares. */
type ApiResult = { error?: unknown; response?: Response };

/**
 * Start optimistic, corrected by the first request either way.
 *
 * The alternative — start unknown and show nothing until something has been
 * tried — means the banner cannot appear on a cold load in a dead spot until
 * the lifter taps something, which is the moment it is least welcome.
 */
let online = $state(true);

/** Reactive. True when the API is believed reachable. */
export function isOnline(): boolean {
  return online;
}

/**
 * Report that a request reached the server, whatever it answered.
 *
 * A 400 or a 500 is a *reachable* server: the connection worked and the API
 * had an opinion. Treating those as offline would queue writes the server has
 * already refused, and retry them forever.
 */
export function markReachable(): void {
  online = true;
}

/** Report that a request never reached the server. */
export function markUnreachable(): void {
  online = false;
}

/**
 * Whether a client result is a transport failure rather than an answer.
 *
 * The generated client resolves rather than throws, and returns `response`
 * undefined when `fetch` itself rejected — DNS, refused connection, dropped
 * radio. That absence is the discriminator: with a response, the server spoke;
 * without one, nothing did.
 */
export function isTransportFailure(result: ApiResult): boolean {
  return result.error !== undefined && result.response === undefined;
}

/**
 * Fold a result into the connectivity state and say whether it was a transport
 * failure, so a caller can both record and branch in one line.
 */
export function observe(result: ApiResult): boolean {
  const failed = isTransportFailure(result);
  if (failed) {
    markUnreachable();
  } else {
    markReachable();
  }
  return failed;
}

/**
 * Listen to the browser's own signals. Returns a teardown for `$effect`.
 *
 * `offline` is taken at face value — the OS knows when the radio is off. Going
 * `online` is treated as a hint rather than an answer: the link is back, which
 * is not the same as the API being reachable, so it only unblocks a retry. The
 * retry's own result is what actually sets the state, via observe() above.
 */
export function watchConnectivity(onRestored?: () => void): () => void {
  if (typeof window === "undefined") return () => {};

  const goneOffline = () => markUnreachable();
  const backOnline = () => {
    // Deliberately not markReachable(): the banner should stay up until
    // something has actually got through, or a captive portal would clear it
    // while every request still fails.
    onRestored?.();
  };

  window.addEventListener("offline", goneOffline);
  window.addEventListener("online", backOnline);

  // The initial reading, for a tab opened with the radio already off.
  if (navigator.onLine === false) markUnreachable();

  return () => {
    window.removeEventListener("offline", goneOffline);
    window.removeEventListener("online", backOnline);
  };
}

/** Test seam: put the module back in its initial state. */
export function resetConnectivity(): void {
  online = true;
}
