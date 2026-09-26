import {
  getAchievements,
  type Achievement,
  type AchievementHolders,
  type Lifter,
} from "./api";
import { celebrate } from "./celebrate";
import { noteCrowns, resetCrownWatch } from "./crownWatch";
import { pushToast } from "./toast.svelte";

// Who is wearing what, for every lifter on the install at once.
//
// A module-level `$state` object rather than a store, for the reason
// auth.svelte.ts and notifications.svelte.ts are: runes make the object itself
// reactive, so every name on the screen picks up a crown changing hands without
// a subscription anywhere.
//
// WHY IT IS GLOBAL AND FETCHED ONCE
//
// A crown is drawn beside a name, and names are everywhere — the feed, the
// roster, the leaderboard, the comment authors, the notification panel, the
// header. Asking "does this lifter hold anything" per name would be a request per
// row. So the whole answer is fetched once and consulted synchronously, which is
// what makes the crown affordable on surfaces that render dozens of people.
//
// The API is shaped for exactly this — see the note on GET /achievements about
// why it does not answer per lifter — and the payload is small: one entry per
// catalogue item, each holding at most a handful of lifters.

export const achievements = $state<{
  /** The catalogue with its current holders. Empty until the first load lands. */
  items: AchievementHolders[];
  /** Whether a load has ever completed, so a surface can tell empty from unknown. */
  loaded: boolean;
}>({
  items: [],
  loaded: false,
});

/**
 * Crowns by lifter id, rebuilt whenever the list changes.
 *
 * Derived once rather than scanned per name. A feed of thirty rows asking "what
 * does this lifter hold" would otherwise walk the whole catalogue thirty times
 * per render, and the answer is the same for all of them.
 *
 * `$derived` rather than a plain function so the map is rebuilt on a load and on
 * nothing else — reading it from a component is a lookup, not a computation.
 */
const byLifter = $derived.by(() => {
  const map = new Map<number, Achievement[]>();
  for (const entry of achievements.items) {
    // CROWNS ONLY, and this filter is load-bearing rather than tidy. The catalogue
    // holds levels too now, and `crownsFor` feeds <LifterName>, which draws a Crown
    // icon per entry it returns — without this, every lifter past Level 5 would wear
    // a bogus crown beside their name on the feed, the roster, the leaderboard,
    // comment bylines and the header. A level has its own badge; see levels.svelte.ts.
    if (entry.achievement.kind !== "crown") continue;
    for (const holder of entry.holders) {
      const held = map.get(holder.id);
      if (held) {
        held.push(entry.achievement);
      } else {
        map.set(holder.id, [entry.achievement]);
      }
    }
  }
  return map;
});

/**
 * What this lifter is currently wearing, in catalogue order.
 *
 * Returns a shared empty array for the common case rather than a fresh one, so a
 * name that holds nothing — which is most names — does not allocate on every
 * render.
 *
 * `undefined` is answered the same way as a lifter who holds nothing. Callers
 * pass `auth.me?.id` and the like, and a signed-out reader wearing no crown is
 * the right answer rather than a case for them to handle.
 */
const NONE: Achievement[] = [];

export function crownsFor(lifterId: number | undefined): Achievement[] {
  if (lifterId === undefined) return NONE;
  return byLifter.get(lifterId) ?? NONE;
}

/**
 * The whole catalogue entry for one slug, or null if it is unknown.
 *
 * Wanted by any surface that has a slug and needs more than the label — the
 * achievement dialog draws the description off this. Returns null before the
 * catalogue has loaded, so callers fall back rather than render a blank.
 */
export function achievementBySlug(slug: string | undefined): Achievement | null {
  if (!slug) return null;
  for (const entry of achievements.items) {
    if (entry.achievement.slug === slug) return entry.achievement;
  }
  return null;
}

/**
 * Who holds one achievement RIGHT NOW — the reverse of `crownsFor`.
 *
 * The achievement dialog is why this exists. A crown notification is an event
 * that may be hours old, so "Grace took this" needs a second sentence saying
 * whether it is still hers, and answering that means asking who holds it now.
 *
 * Several holders is ordinary: tied figures share rank 1 and everybody on it is
 * crowned. An empty list means nobody holds it — which after a month where nobody
 * trained is the honest answer, since a board led at zero crowns nobody.
 */
const NO_HOLDERS: Lifter[] = [];

export function holdersOf(slug: string | undefined): Lifter[] {
  if (!slug) return NO_HOLDERS;
  for (const entry of achievements.items) {
    if (entry.achievement.slug === slug) return entry.holders;
  }
  return NO_HOLDERS;
}

/**
 * The label for one achievement, for a surface that has a slug and no entry.
 *
 * The notification panel is why this exists: a `crown` row carries
 * `achievementSlug` and has to turn it into "Week streak" to build its sentence.
 * Returns null when the catalogue has not loaded or does not know the slug, and
 * the caller says the unnamed thing — which is the same fallback it already needs
 * for a row that folded several boards.
 */
export function achievementLabel(slug: string | undefined): string | null {
  if (!slug) return null;
  for (const entry of achievements.items) {
    if (entry.achievement.slug === slug) return entry.achievement.label;
  }
  return null;
}

/**
 * Read the standings once.
 *
 * Failures leave the last good list in place and are otherwise silent. A crown is
 * an ornament: a lifter who cannot fetch it sees names without crowns, which is
 * the same thing they saw before the feature existed and is not worth an error
 * banner on a screen somebody may be mid-set on.
 *
 * `loaded` is deliberately not set on failure, so a surface that wants to tell
 * "nobody holds anything" from "we have not been told yet" still can.
 *
 * `viewerId` is what lets this celebrate. It is a PARAMETER rather than a read of
 * `auth.me`, and that is a hard constraint and not a preference: auth.svelte.ts
 * imports `resetAchievements` from this module, so importing auth here would close
 * a cycle. App.svelte's poller has the id and hands it over.
 */
export async function loadAchievements(viewerId?: number): Promise<void> {
  const result = await getAchievements();
  if (result.status !== 200) return;
  achievements.items = result.data.items;
  achievements.loaded = true;
  announce(viewerId, result.data.items);
}

/**
 * Mark a crown the lifter has just taken.
 *
 * The server tells everybody else and deliberately not them, so this is the
 * earner's half of the news — and it is a MOMENT rather than a panel row, which is
 * why it is a toast and confetti and not a notification.
 *
 * ONE BURST OF CONFETTI, however many crowns arrived at once. The reconciler runs
 * a whole pass at a time, so taking three boards in one sweep is ordinary; three
 * overlapping bursts is not three times the celebration, it is a stutter. Each
 * crown still gets its own toast, because each is a different thing to have won.
 *
 * Silent on the first observation and on a crown already held — see noteCrowns.
 */
function announce(viewerId: number | undefined, items: AchievementHolders[]): void {
  const fresh = noteCrowns(viewerId, items);
  if (fresh.length === 0) return;

  for (const crown of fresh) {
    pushToast({ title: "You took a crown", body: crown.label, tone: "success" });
  }
  celebrate({ particleCount: 140, spread: 75, origin: { y: 0.6 } });
}

/**
 * How often to ask again while the tab is in the foreground.
 *
 * Far slower than the notification poll, and the reason is the server's: crowns
 * are reconciled on an hourly sweeper, so asking every minute would be
 * fifty-nine requests that can only ever get the same answer. Ten minutes is a
 * compromise against the sweeper's own hour — it bounds how long a tab left open
 * can disagree with the leaderboard page without pretending this is live data.
 *
 * The request is ETagged, so a poll that finds nothing changed is a 304.
 */
const POLL_MS = 10 * 60 * 1000;

/**
 * Keep the crowns current until the returned teardown is called.
 *
 * Mirrors startPolling in notifications.svelte.ts, including the visibility
 * check — a backgrounded phone browser has nobody to draw a crown for — and is
 * owned by App.svelte for that module's reason: the surfaces that draw crowns
 * are lazy-loaded, and whether anybody is keeping them current must not depend
 * on which chunk has arrived.
 */
export function startAchievementPolling(viewerId?: number): () => void {
  const tick = () => {
    if (document.visibilityState !== "visible") return;
    void loadAchievements(viewerId);
  };

  tick();
  const handle = setInterval(tick, POLL_MS);
  return () => clearInterval(handle);
}

/**
 * Drop everything, on sign-out.
 *
 * Same reasoning as resetNotifications: this holds other lifters' identities, and
 * the next person to use this browser must not be shown a frame of the previous
 * account's install while their own /me is in flight.
 */
export function resetAchievements(): void {
  achievements.items = [];
  achievements.loaded = false;
  // The celebration's memory goes too. It names which crowns one account held, so
  // leaving it would compare the next lifter against somebody else's standing —
  // and it is cleared here rather than from auth.svelte.ts because that module
  // already imports this one, and the reverse would be a cycle.
  resetCrownWatch();
}
