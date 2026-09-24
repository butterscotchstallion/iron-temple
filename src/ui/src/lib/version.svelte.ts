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

/** Release notes for one build, in the shape changelog.json carries them. */
export type ReleaseNotes = { version: string; entries: string[] };

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
  /**
   * What shipped in the build `latest` names, fetched from the deployed bundle.
   * Its own `version` field is what makes it safe to show — see loadNotes().
   */
  notes: ReleaseNotes;
}>({
  running: "",
  latest: "",
  environment: "",
  dismissed: "",
  notes: { version: "", entries: [] },
});

/** Ask again this often while the tab is in the foreground. */
const POLL_MS = 5 * 60 * 1000;

/**
 * Floor on how often coming back to the tab can trigger a poll. Flicking
 * between tabs shouldn't mean a request per flick.
 */
const REFOCUS_MIN_GAP_MS = 60 * 1000;

/**
 * Where a deployed bundle publishes its own release notes. Emitted into the
 * build output by changelogVirtualModule() in vite.config.ts, and served with
 * `Cache-Control: no-cache` (nginx.conf) so this can't come back stale.
 */
const CHANGELOG_URL = `${import.meta.env.BASE_URL}changelog.json`;

let inFlight = false;
let notesInFlight = false;
let lastPollAt = 0;

/**
 * Whether a version string names a local build rather than a release.
 *
 * Only the release pipeline stamps a version, via -ldflags (release.yml). A
 * plain `go build` — which is what air runs on every save in `make dev` — leaves
 * main.version as "dev", and the API answers /health with "dev-<sha>", plus a
 * "-dirty" suffix while the tree has uncommitted changes.
 */
function isDevBuild(v: string): boolean {
  return v === "dev" || v.startsWith("dev-");
}

/**
 * Whether `candidate` is a release worth offering to whoever is on `running`.
 *
 * Takes the version as an argument rather than reading `latest`, because poll()
 * has to ask this about an answer it has not published yet — the notes fetch is
 * triggered from there, and firing it for a version nobody will be offered means
 * a request per `air` rebuild in development.
 *
 * `running` must be known: an unanswered /health leaves it empty, and
 * "" !== "v1.2.3" would otherwise read as a new release every time the API is
 * briefly unreachable.
 *
 * Dev builds are never offered, on either side of the comparison. The prompt
 * exists because a static bundle behind nginx cannot update itself; in
 * development Vite serves the bundle and reloads it itself, while the version
 * the API reports moves on every rebuild — a commit, a checkout, or merely
 * dirtying the tree flips the string. Comparing those two strings turned every
 * `air` rebuild into "a new version has been deployed", which was never true:
 * nothing local is deployed. A release never reports "dev", so this cannot
 * suppress a real one.
 */
function isOfferable(candidate: string): boolean {
  return (
    version.running !== "" &&
    !isDevBuild(version.running) &&
    !isDevBuild(candidate) &&
    candidate !== version.running &&
    candidate !== version.dismissed
  );
}

/**
 * Whether to offer the update. Reads reactive state, so calling it from a
 * template or a `$derived` re-evaluates when a poll lands.
 *
 * The empty check is the other half of isOfferable()'s: an unanswered /health
 * leaves `latest` empty too, and poll() rejects an empty version before it ever
 * reaches the store.
 */
export function hasUpdate(): boolean {
  return version.latest !== "" && isOfferable(version.latest);
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
 * Fetch what shipped in the build being offered.
 *
 * The notes compiled into THIS bundle describe the build that is running, so the
 * new one's have to come off the wire. The release pipeline builds and repins the
 * API and UI images at the same tag (release.yml), so by the time /health reports
 * a new version the UI pods are serving the new changelog.json.
 *
 * `version` INSIDE the file is the whole of the safety here. Between the API
 * rolling and the UI pods finishing — or from behind a cache that kept a copy —
 * this answers with the previous release's notes, and listing those under "new
 * version available" is worse than listing nothing. Nothing is stored unless the
 * file names the release on offer.
 *
 * Skipping once we hold notes for the target is also what makes a rejected answer
 * retry: a mismatch stores nothing, so the next poll asks again and picks them up
 * when the pods finish rolling. They then land reactively into a dialog that is
 * already open. An attempt-once flag would instead lose the notes permanently for
 * exactly the release whose rollout race this is guarding against.
 *
 * Failures are swallowed for the reason poll()'s are, and one more: these notes
 * hang off a dialog that has to appear either way.
 */
async function loadNotes(target: string): Promise<void> {
  if (notesInFlight || version.notes.version === target) return;
  notesInFlight = true;
  try {
    const response = await fetch(CHANGELOG_URL, { cache: "no-cache" });
    if (!response.ok) return;

    // Parsed defensively, the same way vite.config.ts reads the JSON it writes.
    // `vite preview` and any pod predating this file fall back to index.html,
    // which is a 200 of HTML that .json() throws on.
    const parsed: unknown = await response.json();
    if (!parsed || typeof parsed !== "object") return;
    const { version: released, entries } = parsed as Partial<ReleaseNotes>;
    if (typeof released !== "string" || !Array.isArray(entries)) return;

    // Against `latest` rather than the captured `target`: another release may
    // have landed while this was in the air, and a file naming THAT one is
    // current rather than stale.
    if (released !== version.latest) return;
    version.notes = {
      version: released,
      entries: entries.filter((entry) => typeof entry === "string"),
    };
  } catch {
    // Offline, a pod mid-roll, or a 200 of index.html from an SPA fallback.
  } finally {
    notesInFlight = false;
  }
}

/**
 * The notes to show beside the offer, or none. Reads reactive state, so a
 * `$derived` re-runs when a late answer lands under an open dialog.
 *
 * Empty unless they name the exact release on offer. That is a different job
 * from the check in loadNotes(): that one decides what is worth keeping, this
 * one decides what is safe to show. A rejected fetch leaves the PREVIOUS
 * release's notes in the store — decline v2 and then have v3 land while the old
 * pods are still up, and without this the dialog would caption v2's notes with
 * v3's version.
 */
export function updateNotes(): string[] {
  return version.notes.version === version.latest ? version.notes.entries : [];
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
    const health = await getHealth();
    if (health.status !== 200) return;
    const reported = health.data.version ?? "";
    if (reported === "") return;
    version.environment = health.data.environment ?? "";
    version.latest = reported;
    // First answer of this page load is the baseline: whatever the API says
    // now is what this bundle was served alongside.
    if (version.running === "") version.running = reported;

    // Deliberately not awaited. A changelog.json that never answers has to cost
    // nothing but the notes: awaiting it here would hold `latest` back, and with
    // it hasUpdate(), so a hung request for the decoration would silently
    // suppress the prompt itself. Capping it with a timer is no escape either —
    // the e2e suite installs a fake clock (see deferred.svelte.ts), under which
    // a setTimeout never fires.
    if (isOfferable(reported)) void loadNotes(reported);
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
