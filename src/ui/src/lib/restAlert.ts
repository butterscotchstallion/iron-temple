// The noise a finished rest makes.
//
// A countdown you have to watch is a countdown you have to hold your phone for,
// and nobody holds their phone through a set. The screen is usually off or
// buried by the time the rest runs out, so the timer needs a way to reach the
// lifter that isn't pixels: a short two-note chime and a buzz.
//
// Three things make this fiddlier than "play a sound":
//
//   - Autoplay policy. A browser will not let a page make noise until the user
//     has interacted with it, and an AudioContext created before that starts
//     suspended and stays suspended. prime() is called from the tap that starts
//     a rest — which is a gesture by definition — so by the time the countdown
//     expires the context is already running.
//   - No asset. The chime is synthesised from two oscillators rather than
//     shipped as an mp3: it is a few lines, it adds nothing to the bundle, and
//     it cannot 404 in an offline gym.
//   - Everything here is optional. A denied context, a browser with no
//     vibration, a locked-down webview — none of that is worth an exception on
//     the way to showing 0:00, so every call is wrapped and failure is silent.

const MUTE_KEY = "iron-temple:rest-muted";

/** Lazily created, and only ever from a user gesture. See prime(). */
let ctx: AudioContext | null = null;

/**
 * localStorage rather than the sessionStorage the countdown itself uses: a rest
 * is per-tab and per-workout, but "don't make noise in my gym" is a preference,
 * and having to re-mute every session is how a preference becomes a nuisance.
 *
 * Storage can throw rather than merely fail (Safari private mode, an embedded
 * webview), so both accessors swallow — the same contract as restStorage.ts.
 */
export function isMuted(): boolean {
  try {
    return window.localStorage.getItem(MUTE_KEY) === "1";
  } catch {
    return false;
  }
}

export function setMuted(muted: boolean): void {
  try {
    window.localStorage.setItem(MUTE_KEY, muted ? "1" : "0");
  } catch {
    // The preference just won't outlive the tab.
  }
}

/**
 * Open (or resume) the audio context while a user gesture is in progress.
 *
 * Safe to call on every tap: creating the context is once, and resume() on an
 * already-running context resolves immediately. Deliberately does nothing when
 * muted — a muted timer has no reason to hold an audio device open.
 */
export function prime(): void {
  if (isMuted()) return;
  try {
    ctx ??= new AudioContext();
    // Returns a promise that rejects if the gesture wasn't good enough. There is
    // nothing to do about that here; the chime simply won't sound.
    void ctx.resume().catch(() => {});
  } catch {
    ctx = null;
  }
}

/** Two short descending notes — audible over gym noise, over in half a second. */
function chime(): void {
  if (!ctx || ctx.state !== "running") return;
  const start = ctx.currentTime;
  for (const [at, hz] of [
    [0, 880],
    [0.18, 660],
  ] as const) {
    const osc = ctx.createOscillator();
    const gain = ctx.createGain();
    osc.type = "sine";
    osc.frequency.value = hz;
    // Ramp the gain rather than switching it: a square-edged start and stop on a
    // sine wave is an audible click at both ends.
    gain.gain.setValueAtTime(0, start + at);
    gain.gain.linearRampToValueAtTime(0.25, start + at + 0.02);
    gain.gain.linearRampToValueAtTime(0, start + at + 0.16);
    osc.connect(gain).connect(ctx.destination);
    osc.start(start + at);
    osc.stop(start + at + 0.18);
  }
}

/**
 * Announce that the rest is over: chime, and buzz where that exists (Android
 * and little else — iOS Safari has no Vibration API, which is why the sound is
 * not merely a bonus on top of it).
 *
 * A no-op when muted, so the caller never has to ask.
 */
export function fire(): void {
  if (isMuted()) return;
  try {
    chime();
  } catch {
    // A context that died between prime() and here.
  }
  try {
    navigator.vibrate?.([120, 80, 120]);
  } catch {
    // Some webviews expose the method and then refuse the call.
  }
}
