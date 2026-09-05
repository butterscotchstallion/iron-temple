/**
 * A durable queue for the writes an active session makes, so a dropped
 * connection costs nothing.
 *
 * The active session logs straight to the server: every rep tap, weight nudge
 * and weigh-in is a request, and there is no local draft of the workout. That
 * is a fine design on a desk and the wrong one at a rack in a basement, where
 * the failure is not "the request was slow" but "there is no network for the
 * next forty minutes". Before this, each of those taps showed "Couldn't save
 * that set." and the rep was simply gone.
 *
 * So a write that cannot reach the server is put here instead, applied to the
 * screen as though it had succeeded, and replayed in order when the API answers
 * again. The lifter keeps training; the sync is somebody else's problem.
 *
 * WHY LOCALSTORAGE AND NOT INDEXEDDB
 *
 * The obvious reach is IndexedDB, and it is the wrong tool at this size. What is
 * stored is at most a few dozen records of five small fields — a whole session
 * is well under 10 KB, orders of magnitude inside the quota. Against that,
 * localStorage is synchronous, which means the queue can be written inside the
 * same turn as the tap that caused it and cannot be lost to a tab closing
 * mid-transaction; and it works in jsdom, so every rule below is testable
 * without a fake-indexeddb dependency this repo would otherwise have to add.
 * The persistence idiom is the one restStorage.ts and cache.svelte.ts already
 * use, down to the best-effort access.
 *
 * WHY IT SURVIVES THE TAB, UNLIKE THE OTHER TWO
 *
 * cache.svelte.ts and restStorage.ts both use sessionStorage, deliberately: a
 * cached list and a running countdown are worth exactly one tab's lifetime.
 * This is not. A phone that kills a backgrounded tab between sets, or a lifter
 * who closes the app and drives home, is still owed the reps they logged — so
 * this is localStorage, and it is cleared on sign-out rather than on close.
 */

import {
  addSessionSet,
  finishSession,
  removeSessionSet,
  updateSession,
  updateSessionSet,
} from "./api";
import { isOnline, observe } from "./connectivity.svelte";

/** Fields a set update may carry. Absolute values, never deltas — see coalesce. */
export type SetPatch = {
  actualReps?: number | null;
  completed?: boolean;
  weightLb?: number;
};

/**
 * A write as a caller describes it.
 *
 * The id is added on the way in rather than being part of the union, because
 * `Omit<Union, "id">` is not the union minus a key — it collapses to the keys
 * every member shares, which for a discriminated union like this one is
 * `kind` and `sessionId` and nothing else. An intersection distributes
 * properly, so the discriminant still narrows on both sides of the queue.
 */
export type PendingWrite =
  | { kind: "updateSet"; sessionId: number; setId: number; body: SetPatch }
  | { kind: "addSet"; sessionId: number; exerciseId: number; tempSetId: number }
  | { kind: "removeSet"; sessionId: number; setId: number }
  | { kind: "updateSession"; sessionId: number; bodyweightLb: number | null }
  | { kind: "finishSession"; sessionId: number };

export type QueuedWrite = PendingWrite & { id: number };

/**
 * Bumped when the entry shape changes in a way an older queue cannot be
 * replayed under. A stored queue at a version this build does not know is
 * dropped rather than guessed at: replaying a misread write is worse than
 * losing it, because the lifter can see a lost rep and cannot see a wrong one.
 */
const STORAGE_VERSION = 1;
const STORAGE_KEY = "iron-temple:writes:v1";

let queue = $state<QueuedWrite[]>([]);
/** Writes the server refused outright. Surfaced so the lifter is told. */
let rejected = $state(0);
let nextId = 1;
/**
 * Temp ids count DOWN from -1. Server ids are SERIAL and therefore positive, so
 * a negative id cannot collide with a real one, and any code that mistakes one
 * for the other fails loudly (a 404 on /sets/-3) rather than editing the wrong
 * set.
 */
let nextTemp = -1;
let flushing = false;
let drainedHandler: (() => void) | null = null;

/** How many writes are waiting. Reactive. */
export function queuedCount(): number {
  return queue.length;
}

/** How many were refused by the server and will not be retried. Reactive. */
export function rejectedCount(): number {
  return rejected;
}

export function clearRejected(): void {
  rejected = 0;
}

/**
 * Called when a non-empty queue has been fully replayed.
 *
 * The route re-reads the session from the server rather than trying to patch
 * its local copy. That is the whole reason the ids below need no reconciling on
 * screen: after a flush the server's version is authoritative and simply
 * replaces what the optimistic writes built, temp ids and all.
 */
export function onDrained(handler: (() => void) | null): void {
  drainedHandler = handler;
}

/**
 * Whether a new write has to go through the queue rather than straight out.
 *
 * Offline is the obvious half. The other half is the one easy to miss: **a
 * non-empty queue also forces queueing**, even with the network back. These are
 * ordered edits to the same rows, so a write sent live while older ones are
 * still waiting would overtake them — the server would apply the new reps and
 * then, a moment later, the replay of an older tap would overwrite them with a
 * stale value. Once anything is queued, everything queues until it drains.
 */
export function mustQueue(): boolean {
  return !isOnline() || queue.length > 0;
}

/** A placeholder id for a set that does not exist on the server yet. */
export function nextTempSetId(): number {
  const id = nextTemp;
  nextTemp -= 1;
  return id;
}

export function isTempSetId(id: number): boolean {
  return id < 0;
}

// ---- persistence ----

function storage(): Storage | null {
  try {
    return window.localStorage;
  } catch {
    // Safari private mode has historically thrown on access, and an embedded
    // webview can deny storage outright. An unstorable queue still works for
    // the life of the page, which is most of what it is for.
    return null;
  }
}

function persist(): void {
  const store = storage();
  if (!store) return;
  try {
    store.setItem(
      STORAGE_KEY,
      JSON.stringify({ version: STORAGE_VERSION, nextId, nextTemp, entries: queue }),
    );
  } catch {
    // Out of quota, or denied. The in-memory queue is unaffected, so this costs
    // the reload case and nothing else.
  }
}

/** Read a stored queue back. Called once, at module load. */
export function hydrateQueue(): void {
  const store = storage();
  if (!store) return;
  try {
    const raw = store.getItem(STORAGE_KEY);
    if (!raw) return;
    const parsed = JSON.parse(raw) as {
      version?: number;
      nextId?: number;
      nextTemp?: number;
      entries?: QueuedWrite[];
    };
    if (parsed.version !== STORAGE_VERSION || !Array.isArray(parsed.entries)) {
      store.removeItem(STORAGE_KEY);
      return;
    }
    queue = parsed.entries;
    nextId = parsed.nextId ?? queue.length + 1;
    nextTemp = parsed.nextTemp ?? -1;
  } catch {
    // Truncated or hand-edited. Start clean rather than replay something that
    // cannot be trusted to say what it meant.
    queue = [];
    store.removeItem(STORAGE_KEY);
  }
}

/** Forget everything. Sign-out: the queue holds one lifter's training. */
export function clearQueue(): void {
  queue = [];
  rejected = 0;
  nextTemp = -1;
  storage()?.removeItem(STORAGE_KEY);
}

// ---- enqueueing ----

/**
 * Add a write, or fold it into the one already at the back.
 *
 * Two rules do real work here, and both depend on set patches being ABSOLUTE
 * rather than incremental — `actualReps: 3`, never `actualReps: +1`.
 *
 *   Coalescing. Tapping a set through 1-2-3-4-5 offline is five writes whose
 *   only lasting effect is the fifth. Merged into the last queued entry when it
 *   targets the same set, so a workout logged in a dead spot syncs as one
 *   request per set rather than one per tap. Only against the LAST entry, never
 *   scanning backwards: a later add or remove between them means the order
 *   matters, and merging across it would reorder the queue.
 *
 *   Cancellation. Adding a set and then removing it before either reached the
 *   server is a pair that must not be sent at all — the remove names an id the
 *   server has never heard of, and would 404 the moment the add gave it a real
 *   one. Both come out, along with anything queued against that temp id.
 */
export function enqueue(write: PendingWrite): void {
  if (write.kind === "removeSet" && isTempSetId(write.setId)) {
    const pending = queue.some(
      (entry) => entry.kind === "addSet" && entry.tempSetId === write.setId,
    );
    if (pending) {
      queue = queue.filter((entry) => {
        if (entry.kind === "addSet") return entry.tempSetId !== write.setId;
        if (entry.kind === "updateSet") return entry.setId !== write.setId;
        return true;
      });
      persist();
      return;
    }
  }

  const last = queue[queue.length - 1];
  if (
    write.kind === "updateSet" &&
    last?.kind === "updateSet" &&
    last.setId === write.setId &&
    last.sessionId === write.sessionId
  ) {
    last.body = { ...last.body, ...write.body };
    queue = [...queue];
    persist();
    return;
  }

  queue = [...queue, { ...write, id: nextId++ }];
  persist();
}

// ---- replay ----

/** Send one entry. Resolves to the client's `{ data, error, response }`. */
function send(entry: QueuedWrite) {
  switch (entry.kind) {
    case "updateSet":
      return updateSessionSet({
        path: { sessionId: entry.sessionId, setId: entry.setId },
        body: entry.body,
      });
    case "addSet":
      return addSessionSet({
        path: { sessionId: entry.sessionId },
        body: { exerciseId: entry.exerciseId },
      });
    case "removeSet":
      return removeSessionSet({
        path: { sessionId: entry.sessionId, setId: entry.setId },
      });
    case "updateSession":
      return updateSession({
        path: { sessionId: entry.sessionId },
        body: { bodyweightLb: entry.bodyweightLb },
      });
    case "finishSession":
      return finishSession({ path: { sessionId: entry.sessionId } });
  }
}

/**
 * Rewrite a temp set id to the real one the server just assigned.
 *
 * Every write queued behind an offline "add a set" refers to it by the
 * placeholder, so the moment the add lands they all have to be repointed or
 * they will PATCH an id that does not exist.
 */
function remapTempSetId(tempId: number, realId: number): void {
  queue = queue.map((entry) =>
    (entry.kind === "updateSet" || entry.kind === "removeSet") && entry.setId === tempId
      ? { ...entry, setId: realId }
      : entry,
  );
}

/**
 * Replay everything, oldest first, one at a time.
 *
 * Strictly serial, not a Promise.all: these are ordered edits to the same rows.
 * "Add a set, then set its reps to 5" is not the same request pair in the other
 * order, and the second cannot even be addressed until the first has answered.
 *
 * Two failure modes, treated differently on purpose:
 *
 *   The request never arrived. Still offline. Stop, leave the queue exactly as
 *   it is, try again on the next reconnect. Nothing is lost.
 *
 *   The server answered with an error. It has seen this write and refused it —
 *   a set deleted on another device, a session already finished. Retrying sends
 *   the identical body for the identical answer, forever, and a queue that
 *   cannot get past its first entry never delivers the fifty good writes behind
 *   it. So the entry is dropped and counted, and the lifter is told how many.
 */
export async function flush(): Promise<void> {
  if (flushing) return;
  flushing = true;
  const startedWith = queue.length;

  try {
    while (queue.length > 0) {
      const entry = queue[0];
      const result = await send(entry);

      if (observe(result)) return; // transport failure: still offline

      if (result.error) {
        rejected += 1;
      } else if (entry.kind === "addSet") {
        const realId = (result.data as { id?: number } | undefined)?.id;
        if (typeof realId === "number") remapTempSetId(entry.tempSetId, realId);
      }

      queue = queue.slice(1);
      persist();
    }

    if (startedWith > 0) drainedHandler?.();
  } finally {
    flushing = false;
  }
}

/**
 * Retry on a slow timer for as long as anything is waiting.
 *
 * The `online` event is the primary trigger and this is the backstop, because
 * that event is not dependable in the case that matters most: walking out of a
 * basement re-associates to a network the OS never considered lost, and a
 * captive portal fires `online` while every request still fails. Neither
 * produces an event that means "the API is reachable now" — only trying does.
 *
 * Fifteen seconds because nothing is waiting on it. The reps are already on
 * screen and already durable; this is a background errand, and polling harder
 * would spend a phone's radio to finish it a few seconds sooner.
 */
const RETRY_MS = 15_000;

export function startQueueRetry(): () => void {
  const timer = setInterval(() => {
    if (queue.length > 0) void flush();
  }, RETRY_MS);
  return () => clearInterval(timer);
}

hydrateQueue();
