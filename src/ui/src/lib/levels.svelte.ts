import { getLevels, type LifterLevel } from "./api";

// What level everybody is, for every lifter on the install at once.
//
// A module-level `$state` object rather than a store, for the reason
// houses.svelte.ts and achievements.svelte.ts are: runes make the object itself
// reactive, so every name on the screen picks up a badge without a subscription
// anywhere.
//
// WHY IT IS GLOBAL AND FETCHED ONCE
//
// The crown's problem a third time, and it has the crown's answer. A level is
// drawn beside a name, and names are everywhere — the feed, the roster, the
// leaderboard, comment authors, the header. Asking "what level is this lifter"
// per name would be a request per row.
//
// Carrying the level on the lifter itself is the move that reads as obvious and
// is not, for exactly the reason set out in houses.svelte.ts: six queries hydrate
// a lifter for the wire, and db/queries/users.sql requires that a column added to
// one roster query is added to the other. Joining a count of sessions into all of
// them to draw an ornament is the cost /achievements already declined to pay.
//
// The payload is one entry per account, bounded by a number the install's owner
// sets by hand.
//
// WHY THE CURVE IS NOT IN HERE
//
// Every figure a surface needs is a field: the level, the lifetime XP, how far
// into the level, and what the whole level costs. Nothing in this file knows that
// a session is 100 XP or that level L costs (L-1) x 100, and nothing should — the
// curve has one owner in src/api/internal/levels, and a second implementation
// here would be a thing that can disagree with it. That includes the starting
// state: an account that has never trained is sent as level 1 with no experience
// rather than left out, so "everyone starts at level 1" is not a default this
// file has to hold either.

export const levels = $state<{
  /** Every lifter on the install. Empty until the first load lands. */
  items: LifterLevel[];
  /** Whether a load has ever completed, so a surface can tell empty from unknown. */
  loaded: boolean;
}>({
  items: [],
  loaded: false,
});

/**
 * Levels by lifter id, rebuilt whenever the list changes.
 *
 * `$derived` rather than a scan per lookup, for byLifter's reason in
 * houses.svelte.ts: a feed of thirty rows would otherwise walk every account
 * thirty times per render.
 */
const byLifter = $derived.by(() => {
  const map = new Map<number, LifterLevel>();
  for (const entry of levels.items) map.set(entry.lifterId, entry);
  return map;
});

/**
 * This lifter's standing, or null.
 *
 * Null covers two states on purpose, and both draw nothing. Before the first load
 * the map is empty, and a default here would put a "1" beside every name on the
 * feed for the first beat of a cold load and then jump — the badge appears when
 * the answer does, the way a sigil does. After a load, null means an id this
 * install does not carry: a deleted account still named by an old notification
 * row, or somebody who registered since the last poll. An untrained lifter is NOT
 * this case — they are listed, at level 1.
 *
 * `undefined` is answered the same way, per houseFor's note: callers pass
 * `auth.me?.id` and the like, and a reader with no account wearing nothing is the
 * right answer rather than a case for them to handle.
 */
export function levelFor(lifterId: number | undefined): LifterLevel | null {
  if (lifterId === undefined) return null;
  return byLifter.get(lifterId) ?? null;
}

/**
 * Read the levels once.
 *
 * Failures leave the last good list in place and are otherwise silent, for the
 * reason loadHouses is: a level is an ornament, and a lifter who cannot fetch it
 * sees names without badges — the same thing they saw before the feature existed,
 * and not worth an error banner on a screen somebody may be mid-set on.
 *
 * `loaded` is deliberately not set on failure, so a surface that wants to tell
 * "nobody has trained" from "we have not been told yet" still can.
 */
export async function loadLevels(): Promise<void> {
  const result = await getLevels();
  if (result.status !== 200) return;
  levels.items = result.data.items;
  levels.loaded = true;
}

/**
 * How often to ask again while the tab is in the foreground.
 *
 * The sigil's interval, and for a reason in between the two it sits next to. A
 * level changes only when somebody finishes a session, which is rare per lifter
 * and never something a reader is waiting on for somebody else. The one lifter
 * who IS waiting is the one who just trained, and the session finishing refreshes
 * this directly rather than leaving them to wait out the interval — see
 * ActiveSession.
 *
 * The request is ETagged, so a poll that finds nothing changed is a 304.
 */
const POLL_MS = 10 * 60 * 1000;

/**
 * Keep the levels current until the returned teardown is called.
 *
 * Owned by App.svelte for houses.svelte.ts's reason: the surfaces that draw a
 * badge are lazy-loaded, and whether anybody is keeping them current must not
 * depend on which chunk has arrived.
 */
export function startLevelPolling(): () => void {
  const tick = () => {
    if (document.visibilityState !== "visible") return;
    void loadLevels();
  };

  tick();
  const handle = setInterval(tick, POLL_MS);
  return () => clearInterval(handle);
}

/**
 * Drop everything, on sign-out.
 *
 * Same reasoning as resetHouses: this holds other lifters' identities and how much
 * they have trained, and the next person to use this browser must not be shown a
 * frame of the previous account's install while their own /me is in flight.
 */
export function resetLevels(): void {
  levels.items = [];
  levels.loaded = false;
}
