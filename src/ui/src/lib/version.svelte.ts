import { getHealth } from "./api";

// Which build is running, and whether a newer one has been deployed underneath
// us. A module-level `$state` object rather than a store contract, for the same
// reason auth.svelte.ts is one: Svelte 5 runes make the object itself reactive,
// so the header and the update prompt both re-render off it without
// subscriptions.
//
// The signal is the API's /health version. The release pipeline builds and
// repins the API and UI images at the same tag (.gitea/workflows/release.yml),
// so the API reporting a new version means new UI assets are being served too.
// Nothing here is a build-time constant — see startPolling() for why.

export const version = $state<{
  /**
   * The version this page load is running against — the first answer /health
   * gave us. Empty until it does.
   */
  running: string;
  /** The most recent answer. Diverges from `running` when a release lands. */
  latest: string;
  /** Deployment environment, for the header's label. */
  environment: string;
  /** A version the lifter declined; it must not ask about that one again. */
  dismissed: string;
}>({
  running: "",
  latest: "",
  environment: "",
  dismissed: "",
});

/** Ask again this often while the tab is in the foreground. */
const POLL_MS = 5 * 60 * 1000;

/**
 * Floor on how often coming back to the tab can trigger a poll. Flicking
 * between tabs shouldn't mean a request per flick.
 */
const REFOCUS_MIN_GAP_MS = 60 * 1000;

let inFlight = false;
let lastPollAt = 0;

/**
 * Whether to offer the update. Reads reactive state, so calling it from a
 * template or a `$derived` re-evaluates when a poll lands.
 *
 * Both versions must be known: an unanswered /health leaves `latest` empty, and
 * "" !== "v1.2.3" would otherwise read as a new release every time the API is
 * briefly unreachable.
 */
export function hasUpdate(): boolean {
  return (
    version.running !== "" &&
    version.latest !== "" &&
    version.latest !== version.running &&
    version.latest !== version.dismissed
  );
}

/**
 * Stop asking about the version currently on offer. Declining is per-version,
 * not permanent: when the *next* release lands, `latest` moves past `dismissed`
 * and the prompt comes back. Nobody gets nagged mid-workout about an update
 * they already turned down.
 */
export function dismissUpdate(): void {
  version.dismissed = version.latest;
}

/**
 * Read /health once.
 *
 * Failures are swallowed. The version is decoration — an unreachable /health
 * means we simply don't know yet, which is the state we started in, and a
 * network blip must never surface as an error or (worse) as a phantom update.
 */
export async function poll(): Promise<void> {
  if (inFlight) return;
  inFlight = true;
  try {
    const { data } = await getHealth();
    const reported = data?.version ?? "";
    if (reported === "") return;
    version.environment = data?.environment ?? "";
    version.latest = reported;
    // First answer of this page load is the baseline: whatever the API says
    // now is what this bundle was served alongside.
    if (version.running === "") version.running = reported;
  } catch {
    // Offline, or the API is restarting mid-deploy. Try again next tick.
  } finally {
    // Stamped even on failure, so a refocus loop can't turn an unreachable API
    // into a request per tab switch.
    lastPollAt = Date.now();
    inFlight = false;
  }
}

/**
 * Start watching for a new release. Returns a teardown.
 *
 * The baseline is the first poll of *this page load*, not a constant baked into
 * the bundle. Two reasons: a build-time version only exists in CI builds (the
 * changelog JSON is `continue-on-error`, and a dev checkout has no tag at all),
 * and deriving it from the running API makes the post-reload state
 * self-correcting — the reloaded page re-baselines on the new version, so it
 * cannot immediately prompt again for the update it just took.
 *
 * Only polls while the tab is visible: a backgrounded phone browser doesn't
 * need a request every five minutes, and the answer is only actionable when
 * someone is there to answer the dialog.
 */
export function startPolling(): () => void {
  const tick = () => {
    if (document.visibilityState !== "visible") return;
    void poll();
  };

  // Coming back to a tab that's been open since yesterday is exactly when the
  // answer is most likely to have changed, so don't wait out the interval.
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
