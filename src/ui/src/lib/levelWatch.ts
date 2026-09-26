/**
 * What level the lifter was last time anybody looked, so a level they have just
 * reached can be told apart from one they have had for a fortnight.
 *
 * crownWatch.ts's twin, and deliberately its twin rather than something cleverer:
 * the same storage, the same silent first observation, the same refusal to compare
 * across accounts. What differs is what is stored — one number instead of a set of
 * slugs — and what counts as news.
 *
 * WHY THE CLIENT HAS TO DO THIS AT ALL
 *
 * Nothing on the server knows that a level changed. The level is derived from a
 * count of sessions at read time (see src/api/internal/levels), so there is no
 * write to hang an announcement off and no row that says "was 11, now 12" — the
 * only place the difference exists is between two readings, and the client is what
 * holds both. That is the opposite of the crown's situation, where a reconciler
 * writes a reign and could have announced it; the shape of the answer is the same
 * anyway, which is why this file looks like that one.
 *
 * WHY IT IS PERSISTED, AND IN localStorage
 *
 * The comparison has to survive a reload, or every refresh is a fresh
 * congratulation. sessionStorage would mean one per tab, forever. So localStorage,
 * and cleared on sign-out — crownWatch's reasoning, and writeQueue's before it.
 *
 * Plain `.ts` and not `.svelte.ts`: nothing here is reactive. It is a number, a
 * comparison, and a side effect on storage.
 */

/**
 * Bumped when the stored shape changes. A mismatch is discarded rather than
 * migrated, which costs one silent first observation and nothing else.
 */
const STORAGE_VERSION = 1;
const STORAGE_KEY = "iron-temple:level:v1";

type Stored = {
  version: number;
  /** Whose level this was. A different account's must not be compared. */
  viewerId: number;
  level: number;
};

/**
 * In-memory mirror, and the fallback when storage is unavailable.
 *
 * Safari private mode has historically thrown on access and an embedded webview can
 * deny storage outright. Celebrating correctly for the life of the page is most of
 * what this is for, so an unstorable watch still works — it just forgets across a
 * reload, which shows up as one repeated toast rather than as an error.
 */
let seen: number | null = null;
let seenFor: number | null = null;

function storage(): Storage | null {
  try {
    return window.localStorage;
  } catch {
    return null;
  }
}

function persist(viewerId: number, level: number): void {
  const store = storage();
  if (!store) return;
  try {
    const payload: Stored = { version: STORAGE_VERSION, viewerId, level };
    store.setItem(STORAGE_KEY, JSON.stringify(payload));
  } catch {
    // Out of quota, or denied. The in-memory number is unaffected, so this costs
    // the reload case and nothing else.
  }
}

/**
 * Read the stored level back, or null when there is nothing trustworthy to read.
 *
 * Null for a DIFFERENT viewer as well as for absent or corrupt data. Two accounts
 * sharing a browser must not inherit each other's level — the first load as the
 * second lifter would otherwise congratulate them for reaching a level the first
 * one had, or say nothing about one they genuinely just reached.
 */
function restore(viewerId: number): number | null {
  if (seen !== null && seenFor === viewerId) return seen;

  const store = storage();
  if (!store) return null;
  const raw = store.getItem(STORAGE_KEY);
  if (raw === null) return null;

  try {
    const parsed = JSON.parse(raw) as Partial<Stored>;
    if (
      parsed.version !== STORAGE_VERSION ||
      parsed.viewerId !== viewerId ||
      typeof parsed.level !== "number"
    ) {
      return null;
    }
    return parsed.level;
  } catch {
    // Truncated or hand-edited. Start clean rather than compare against something
    // that cannot be trusted to say what it meant.
    store.removeItem(STORAGE_KEY);
    return null;
  }
}

/**
 * Record what level this lifter is now, and report it if they just reached it.
 *
 * THE FIRST OBSERVATION IS SILENT, and that is the point of the function rather
 * than a detail of it. Without it, the first load on a device greets a lifter with
 * a toast for a level they earned a year ago.
 *
 * A LEVEL THAT WENT DOWN IS SILENT TOO, and it is a real case rather than a
 * defensive one: experience is derived from sessions, so deleting one lowers it.
 * The baseline moves down with it — quietly, because losing a level is not an
 * occasion, and because leaving the baseline high would then swallow the genuine
 * re-crossing on the way back up.
 *
 * `null` for the level means "we do not know yet": the levels have not loaded, or
 * this install does not carry this lifter. Nothing is recorded and nothing is
 * reported, so a cold cache cannot be mistaken for a fall to level 1.
 */
export function noteLevel(
  viewerId: number | undefined,
  level: number | null,
): number | null {
  // No viewer means no baseline is written, which is what makes it safe for every
  // non-celebrating caller of loadLevels to pass nothing: a refresh cannot record
  // a level and thereby swallow the announcement of it.
  if (viewerId === undefined || level === null) return null;

  const before = restore(viewerId);
  seen = level;
  seenFor = viewerId;
  persist(viewerId, level);

  if (before === null || level <= before) return null;
  return level;
}

/**
 * Forget everything, on sign-out.
 *
 * This names how much one account has trained, so the next person to use the
 * browser must not be compared against it — restore() already refuses a mismatched
 * viewer, and this is the belt to that braces.
 */
export function resetLevelWatch(): void {
  seen = null;
  seenFor = null;
  storage()?.removeItem(STORAGE_KEY);
}
