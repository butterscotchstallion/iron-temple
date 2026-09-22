/**
 * Transient notices, owned by the shell.
 *
 * The app already had two ways to tell the lifter something and neither fits
 * "this just happened". `ErrorBanner` is sticky and dismissible because a failed
 * write is not transient and must not scroll away unread; `OfflineBanner` is a
 * standing statement of fact that clears itself when the fact changes. A toast is
 * the third thing: a moment, worth reading once, worth nothing afterwards.
 *
 * Deliberately not a dependency. `sonner-svelte` and friends bring a portal, a
 * swipe gesture and a theming layer to render a box with two lines of text in it,
 * and the entry chunk is already something this codebase argues with itself about
 * — see the deferred bits-ui shell in App.svelte.
 *
 * Module-level state rather than context, for the same reason writeQueue is: what
 * it reports outlives the route that caused it. A toast raised by the generator
 * should still be legible after navigating away from the screen that started it.
 */

/**
 * How loud a notice is.
 *
 * Only three, and all three are existing theme tokens — there is no new colour
 * ramp here. `amber` would have been a fourth (OfflineBanner owns it) but nothing
 * raising a toast needs "careful" as distinct from "failed".
 */
export type ToastTone = "info" | "success" | "danger";

export interface Toast {
  id: number;
  title: string;
  /** Detail under the title. Empty when the title says everything. */
  body: string;
  tone: ToastTone;
  /**
   * True while the thing being reported is still happening.
   *
   * A pending toast never expires on its own: whatever resolves it decides when
   * it goes. That is the whole reason this flag exists rather than a fixed
   * duration — a synchronous backfill takes as long as it takes, and a notice
   * that vanished mid-generation would read as "finished".
   */
  pending: boolean;
}

/**
 * How long a resolved notice lingers.
 *
 * Long enough to read two lines without hurrying, short enough that a run of
 * them does not become a wall. The generator's live loop can raise one every few
 * seconds, which is what set this.
 */
const LINGER_MS = 6000;

/**
 * How many are shown at once.
 *
 * A cap rather than a queue: when the generator is mid-burst, the newest notices
 * are the interesting ones and holding older ones back to show them later would
 * describe the past in the present tense.
 */
const MAX_VISIBLE = 4;

let items = $state<Toast[]>([]);
let nextId = 1;

/**
 * Live expiry timers, keyed by toast id.
 *
 * Held outside the reactive state on purpose — a timer handle is not something
 * anything renders, and putting it in `$state` would invalidate every subscriber
 * each time one was armed.
 */
const timers = new Map<number, ReturnType<typeof setTimeout>>();

/** Reactive. Oldest first, which is the order they stack on screen. */
export function toasts(): Toast[] {
  return items;
}

/**
 * Raise a notice. Returns its id, so a pending one can be resolved later.
 *
 * Callers pass a `title` that stands alone. The `body` is detail, and a toast
 * with only a body would be a paragraph nobody reads at this size.
 */
export function pushToast(notice: {
  title: string;
  body?: string;
  tone?: ToastTone;
  pending?: boolean;
}): number {
  const toast: Toast = {
    id: nextId++,
    title: notice.title,
    body: notice.body ?? "",
    tone: notice.tone ?? "info",
    pending: notice.pending === true,
  };
  items = [...items, toast];
  trim();
  if (!toast.pending) arm(toast.id);
  return toast.id;
}

/**
 * Amend a notice already on screen — normally to resolve a pending one.
 *
 * Resolving in place rather than dismissing and raising a second toast: the two
 * are one event to the person reading, and a "generating…" that disappeared a
 * frame before "generated 137 sessions" appeared would flicker for no reason.
 *
 * Silently does nothing when the id is gone. A toast can be dismissed by hand or
 * pushed out by the cap while the work it describes is still running, and the
 * caller resolving it afterwards is correct behaviour rather than an error — so
 * this must not resurrect it.
 */
export function updateToast(
  id: number,
  patch: { title?: string; body?: string; tone?: ToastTone; pending?: boolean },
): void {
  if (!items.some((t) => t.id === id)) return;
  // Defaults to resolved: every caller today is finishing something, and an
  // omitted `pending` meaning "still pending" would leave notices on screen
  // forever on the path that forgot it.
  const pending = patch.pending ?? false;
  items = items.map((t) => (t.id === id ? { ...t, ...patch, pending } : t));
  // Only now does it get a clock — a pending notice had none, which is the whole
  // point of the flag.
  if (!pending) arm(id);
}

/** Take a notice down now. */
export function dismissToast(id: number): void {
  disarm(id);
  items = items.filter((t) => t.id !== id);
}

/** Test seam: put the module back in its initial state. */
export function resetToasts(): void {
  for (const id of [...timers.keys()]) disarm(id);
  items = [];
  nextId = 1;
}

/** (Re)start the expiry countdown for a resolved notice. */
function arm(id: number): void {
  disarm(id);
  timers.set(
    id,
    setTimeout(() => dismissToast(id), LINGER_MS),
  );
}

function disarm(id: number): void {
  const timer = timers.get(id);
  if (timer !== undefined) {
    clearTimeout(timer);
    timers.delete(id);
  }
}

/**
 * Enforce MAX_VISIBLE, dropping resolved notices before pending ones.
 *
 * The preference matters in one real case: a live loop ticking away raises a
 * notice every few seconds, and a backfill started underneath it would otherwise
 * have its "generating…" evicted by the very activity it is waiting on — leaving
 * a screen that looks idle while the request is still open.
 */
function trim(): void {
  while (items.length > MAX_VISIBLE) {
    const victim = items.find((t) => !t.pending) ?? items[0];
    dismissToast(victim.id);
  }
}
