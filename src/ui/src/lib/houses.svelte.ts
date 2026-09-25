import { listHouses, type House, type HouseMembership } from "./api";

// Which House everybody is in, for every lifter on the install at once.
//
// A module-level `$state` object rather than a store, for the reason
// achievements.svelte.ts is: runes make the object itself reactive, so every name
// on the screen picks up a sigil without a subscription anywhere.
//
// WHY IT IS GLOBAL AND FETCHED ONCE
//
// This is the crown's problem again, and it has the crown's answer. A sigil is
// drawn beside a name, and names are everywhere — the feed, the roster, the
// leaderboard, comment authors, the header. Asking "which House is this lifter in"
// per name would be a request per row.
//
// The alternative was carrying the sigil on the lifter itself, which reads as the
// obvious move and is not: six queries hydrate a lifter for the wire, and
// db/queries/users.sql requires that a column added to one roster query is added
// to the other. Joining the House tables into all of them to draw an ornament is
// the cost /achievements already declined to pay.
//
// The payload is one entry per House plus one per membership, so it is bounded by
// the account count — which the install's owner sets by hand.

export const houses = $state<{
  /** Every House on the install. Empty until the first load lands. */
  items: House[];
  /** Every membership, flat. The shape `byLifter` below wants. */
  memberships: HouseMembership[];
  /** Whether a load has ever completed, so a surface can tell empty from unknown. */
  loaded: boolean;
}>({
  items: [],
  memberships: [],
  loaded: false,
});

/**
 * Houses by id, rebuilt whenever the list changes.
 *
 * `$derived` rather than a scan per lookup, for `byLifter`'s reason: a feed of
 * thirty rows would otherwise walk every House thirty times per render.
 */
const byId = $derived.by(() => {
  const map = new Map<number, House>();
  for (const house of houses.items) map.set(house.id, house);
  return map;
});

/**
 * The House each lifter belongs to, by lifter id.
 *
 * At most one entry per lifter, which is not this map's doing: house_members takes
 * the lifter as its primary key, so a second membership is a row the database
 * cannot hold. That is what makes the sigil beside a name unambiguous everywhere
 * it is drawn, and why this can be a Map rather than a Map of arrays.
 */
const byLifter = $derived.by(() => {
  const map = new Map<number, House>();
  for (const membership of houses.memberships) {
    const house = byId.get(membership.houseId);
    if (house) map.set(membership.userId, house);
  }
  return map;
});

/**
 * The House this lifter is in, or null.
 *
 * `undefined` is answered like a lifter in no House. Callers pass `auth.me?.id`
 * and the like, and a signed-out reader belonging to nothing is the right answer
 * rather than a case for them to handle.
 */
export function houseFor(lifterId: number | undefined): House | null {
  if (lifterId === undefined) return null;
  return byLifter.get(lifterId) ?? null;
}

/**
 * This lifter's sigil, or null when they are in no House.
 *
 * The narrow question `<LifterName>` asks, kept separate from `houseFor` so the
 * common case — drawing the tag — does not read like it needs the whole House.
 */
export function sigilFor(lifterId: number | undefined): string | null {
  return houseFor(lifterId)?.sigil ?? null;
}

/**
 * One House by id, for a surface that has an id and no House.
 *
 * The notification panel is why this exists: a `house-` row carries `houseId` and
 * has to turn it into a name to build its sentence. Returns null when the list has
 * not loaded yet, or for an id this install does not carry, and the caller says the
 * unnamed thing.
 *
 * The usual null is a cold cache rather than a House that went. A House outlives
 * its last member and nothing deletes one, so a notification can no longer outlive
 * the House it names.
 */
export function houseById(houseId: number | undefined): House | null {
  if (houseId === undefined) return null;
  return byId.get(houseId) ?? null;
}

// No ownsHouse here, deliberately. Every surface that needs to know whether the
// caller owns a House is the House's own page, and that page has the server's
// answer in `HouseDetail.viewer` — a second, poll-shaped answer derived from
// `memberships` would be a slightly staler copy of it, and the first time the two
// disagreed the app would have no way to say which was right. `isOwner` rides along
// on HouseMembership because the wire shape describes the row; nothing reads it.

/**
 * Read the Houses once.
 *
 * Failures leave the last good list in place and are otherwise silent, for the
 * reason loadAchievements is: a sigil is an ornament, and a lifter who cannot
 * fetch it sees names without tags — the same thing they saw before the feature
 * existed, and not worth an error banner on a screen somebody may be mid-set on.
 *
 * `loaded` is deliberately not set on failure, so a surface that wants to tell
 * "there are no Houses" from "we have not been told yet" still can.
 */
export async function loadHouses(): Promise<void> {
  const result = await listHouses();
  if (result.status !== 200) return;
  houses.items = result.data.items;
  houses.memberships = result.data.memberships;
  houses.loaded = true;
}

/**
 * How often to ask again while the tab is in the foreground.
 *
 * The crown's interval, and for a related reason rather than the same one: a
 * House changes when somebody founds one, is let into one, or leaves — all of
 * which are rare, and none of which the reader is usually waiting on. A lifter who
 * is waiting is on the House page, which fetches its own detail on mount.
 *
 * The request is ETagged, so a poll that finds nothing changed is a 304.
 */
const POLL_MS = 10 * 60 * 1000;

/**
 * Keep the Houses current until the returned teardown is called.
 *
 * Owned by App.svelte for achievements.svelte.ts's reason: the surfaces that draw
 * a sigil are lazy-loaded, and whether anybody is keeping them current must not
 * depend on which chunk has arrived.
 */
export function startHousePolling(): () => void {
  const tick = () => {
    if (document.visibilityState !== "visible") return;
    void loadHouses();
  };

  tick();
  const handle = setInterval(tick, POLL_MS);
  return () => clearInterval(handle);
}

/**
 * Drop everything, on sign-out.
 *
 * Same reasoning as resetAchievements: this holds other lifters' identities and
 * which group they belong to, and the next person to use this browser must not be
 * shown a frame of the previous account's install while their own /me is in
 * flight.
 */
export function resetHouses(): void {
  houses.items = [];
  houses.memberships = [];
  houses.loaded = false;
}
