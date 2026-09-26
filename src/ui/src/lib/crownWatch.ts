import type { Achievement, AchievementHolders } from "./api";

/**
 * What the lifter held last time anybody looked, so a crown that is new can be
 * told apart from one they have had for a fortnight.
 *
 * WHY THE CLIENT HAS TO DO THIS AT ALL
 *
 * The server tells everybody about a crown EXCEPT the lifter who took it. That is
 * the notifications table's rule — the actor is always filtered out of the
 * recipients — and it is the right rule for a panel: "you did the thing you are
 * looking at" is noise. But it left the person who earned it as the only one not
 * told, finding out by noticing an ornament beside their own name. This is the
 * other half of that decision rather than a reversal of it: the earner gets a
 * moment, not a panel row.
 *
 * WHY IT IS PERSISTED, AND IN localStorage
 *
 * The comparison has to survive a reload, or every refresh is a fresh
 * congratulation. sessionStorage — what cache.svelte.ts and restStorage.ts use —
 * would mean one per tab, forever. So localStorage, and cleared on sign-out,
 * exactly as writeQueue.svelte.ts reasons about the same choice.
 *
 * Plain `.ts` and not `.svelte.ts`: nothing here is reactive. It is a set, a
 * comparison, and a side effect on storage.
 */

/**
 * Bumped when the stored shape changes. A mismatch is discarded rather than
 * migrated, which costs one silent first observation and nothing else.
 */
const STORAGE_VERSION = 1;
const STORAGE_KEY = "iron-temple:crowns:v1";

type Stored = {
  version: number;
  /** Whose crowns these were. A different account's set must not be compared. */
  viewerId: number;
  slugs: string[];
};

/**
 * In-memory mirror, and the fallback when storage is unavailable.
 *
 * Safari private mode has historically thrown on access and an embedded webview
 * can deny storage outright. Celebrating correctly for the life of the page is
 * most of what this is for, so an unstorable watch still works — it just forgets
 * across a reload, which shows up as one repeated toast rather than as an error.
 */
let held: Set<string> | null = null;
let heldFor: number | null = null;

function storage(): Storage | null {
  try {
    return window.localStorage;
  } catch {
    return null;
  }
}

function persist(viewerId: number, slugs: Set<string>): void {
  const store = storage();
  if (!store) return;
  try {
    const payload: Stored = {
      version: STORAGE_VERSION,
      viewerId,
      slugs: [...slugs],
    };
    store.setItem(STORAGE_KEY, JSON.stringify(payload));
  } catch {
    // Out of quota, or denied. The in-memory set is unaffected, so this costs the
    // reload case and nothing else.
  }
}

/**
 * Read the stored set back, or null when there is nothing trustworthy to read.
 *
 * Null for a DIFFERENT viewer as well as for absent or corrupt data. Two accounts
 * sharing a browser must not inherit each other's crowns — the first load as the
 * second lifter would otherwise announce every crown the first one held as newly
 * theirs, or hide one they genuinely just took.
 */
function restore(viewerId: number): Set<string> | null {
  if (held !== null && heldFor === viewerId) return held;

  const store = storage();
  if (!store) return null;
  const raw = store.getItem(STORAGE_KEY);
  if (raw === null) return null;

  try {
    const parsed = JSON.parse(raw) as Partial<Stored>;
    if (
      parsed.version !== STORAGE_VERSION ||
      parsed.viewerId !== viewerId ||
      !Array.isArray(parsed.slugs)
    ) {
      return null;
    }
    return new Set(parsed.slugs.filter((s): s is string => typeof s === "string"));
  } catch {
    // Truncated or hand-edited. Start clean rather than compare against something
    // that cannot be trusted to say what it meant.
    store.removeItem(STORAGE_KEY);
    return null;
  }
}

/**
 * Record what this lifter holds now, and report what is newly theirs.
 *
 * THE FIRST OBSERVATION IS SILENT, and that is the point of the function rather
 * than a detail of it. Without it, the first load on a device greets a lifter with
 * a toast for every crown they already had — which is the same mistake
 * ActivityPanel avoids by comparing against the previous reading rather than a
 * count of its own: "a loop that has been ticking for an hour does not greet the
 * screen with a toast for each of the forty things it did before anyone was
 * looking."
 *
 * A crown lost and then retaken IS reported again. It is genuinely news the second
 * time — a crown describes a standing, and taking one back is taking it back.
 */
export function noteCrowns(
  viewerId: number | undefined,
  items: AchievementHolders[],
): Achievement[] {
  if (viewerId === undefined) return [];

  const mine = new Map<string, Achievement>();
  for (const entry of items) {
    // CROWNS ONLY. The catalogue holds level rungs too, and this file's toast says
    // "You took a crown" — so without the filter the first poll after a deploy would
    // congratulate every lifter for "taking a crown" once per rung they had passed
    // months ago. A level reached has its own moment; see levelWatch.ts.
    //
    // A filter rather than a STORAGE_VERSION bump, which would also have silenced the
    // false toasts and would have cost a real crown its moment on deploy day: a
    // version mismatch is discarded, and a discarded baseline makes the next
    // observation the silent first one.
    if (entry.achievement.kind !== "crown") continue;
    if (entry.holders.some((h) => h.id === viewerId)) {
      mine.set(entry.achievement.slug, entry.achievement);
    }
  }
  const slugs = new Set(mine.keys());

  const before = restore(viewerId);
  held = slugs;
  heldFor = viewerId;
  persist(viewerId, slugs);

  if (before === null) return [];
  return [...mine.entries()].filter(([slug]) => !before.has(slug)).map(([, a]) => a);
}

/**
 * Forget everything, on sign-out.
 *
 * This names which crowns one account held, so the next person to use the browser
 * must not be compared against it — restore() already refuses a mismatched viewer,
 * and this is the belt to that braces.
 */
export function resetCrownWatch(): void {
  held = null;
  heldFor = null;
  storage()?.removeItem(STORAGE_KEY);
}
