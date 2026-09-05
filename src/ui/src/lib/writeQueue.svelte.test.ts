import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  clearQueue,
  clearRejected,
  enqueue,
  flush,
  hydrateQueue,
  isTempSetId,
  mustQueue,
  nextTempSetId,
  onDrained,
  queuedCount,
  rejectedCount,
} from "./writeQueue.svelte";
import { markReachable, markUnreachable, resetConnectivity } from "./connectivity.svelte";

const addSessionSet = vi.hoisted(() => vi.fn());
const finishSession = vi.hoisted(() => vi.fn());
const removeSessionSet = vi.hoisted(() => vi.fn());
const updateSession = vi.hoisted(() => vi.fn());
const updateSessionSet = vi.hoisted(() => vi.fn());

vi.mock("./api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./api")>()),
  addSessionSet,
  finishSession,
  removeSessionSet,
  updateSession,
  updateSessionSet,
}));

/** A request that reached the server and succeeded. */
const ok = (data: unknown = undefined) => ({ data, error: undefined, response: new Response() });
/** A request that reached the server and was refused. */
const refused = () => ({
  data: undefined,
  error: { message: "no" },
  response: new Response("", { status: 409 }),
});
/** A request that never reached anything. `response` absent is the signal. */
const unreachable = () => ({ data: undefined, error: new TypeError("Failed to fetch") });

beforeEach(() => {
  localStorage.clear();
  clearQueue();
  clearRejected();
  resetConnectivity();
  onDrained(null);
});

afterEach(() => {
  vi.clearAllMocks();
});

describe("mustQueue", () => {
  it("is false when online with nothing waiting", () => {
    expect(mustQueue()).toBe(false);
  });

  it("is true when offline", () => {
    markUnreachable();
    expect(mustQueue()).toBe(true);
  });

  // The subtle half. A live write sent past older queued ones would land first,
  // and the replay behind it would then overwrite the newer value with a stale
  // one. Once anything is queued, everything queues.
  it("is true when online but writes are still waiting", () => {
    enqueue({ kind: "updateSet", sessionId: 1, setId: 2, body: { actualReps: 3 } });
    markReachable();
    expect(mustQueue()).toBe(true);
  });
});

describe("enqueue", () => {
  it("keeps writes in the order they were made", () => {
    enqueue({ kind: "updateSet", sessionId: 1, setId: 1, body: { actualReps: 1 } });
    enqueue({ kind: "removeSet", sessionId: 1, setId: 2 });
    enqueue({ kind: "finishSession", sessionId: 1 });
    expect(queuedCount()).toBe(3);
  });

  // Tapping a set through 1-2-3-4-5 offline has one lasting effect. Sending
  // five requests for it would be five round trips to arrive at the fifth.
  it("folds repeated updates to one set into a single write", () => {
    for (const actualReps of [1, 2, 3, 4, 5]) {
      enqueue({ kind: "updateSet", sessionId: 1, setId: 7, body: { actualReps } });
    }
    expect(queuedCount()).toBe(1);
  });

  it("merges the fields of folded updates rather than replacing them", async () => {
    enqueue({ kind: "updateSet", sessionId: 1, setId: 7, body: { weightLb: 185 } });
    enqueue({ kind: "updateSet", sessionId: 1, setId: 7, body: { actualReps: 5, completed: true } });
    expect(queuedCount()).toBe(1);

    updateSessionSet.mockResolvedValue(ok({ id: 7 }));
    await flush();

    expect(updateSessionSet).toHaveBeenCalledWith({
      path: { sessionId: 1, setId: 7 },
      body: { weightLb: 185, actualReps: 5, completed: true },
    });
  });

  it("does not fold updates to different sets", () => {
    enqueue({ kind: "updateSet", sessionId: 1, setId: 7, body: { actualReps: 1 } });
    enqueue({ kind: "updateSet", sessionId: 1, setId: 8, body: { actualReps: 1 } });
    expect(queuedCount()).toBe(2);
  });

  // Folding across an intervening write would reorder the queue.
  it("does not fold across another write to the same set", () => {
    enqueue({ kind: "updateSet", sessionId: 1, setId: 7, body: { actualReps: 1 } });
    enqueue({ kind: "addSet", sessionId: 1, exerciseId: 3, tempSetId: -1 });
    enqueue({ kind: "updateSet", sessionId: 1, setId: 7, body: { actualReps: 2 } });
    expect(queuedCount()).toBe(3);
  });

  // The add has no server id yet, so the remove names something that does not
  // exist. Sending the pair would add a set and then 404 trying to delete it.
  it("cancels an add and remove of the same unsent set", () => {
    const temp = nextTempSetId();
    enqueue({ kind: "addSet", sessionId: 1, exerciseId: 3, tempSetId: temp });
    enqueue({ kind: "removeSet", sessionId: 1, setId: temp });
    expect(queuedCount()).toBe(0);
  });

  it("cancels edits made to a set that was added and then removed", () => {
    const temp = nextTempSetId();
    enqueue({ kind: "addSet", sessionId: 1, exerciseId: 3, tempSetId: temp });
    enqueue({ kind: "updateSet", sessionId: 1, setId: temp, body: { actualReps: 5 } });
    enqueue({ kind: "removeSet", sessionId: 1, setId: temp });
    expect(queuedCount()).toBe(0);
  });

  // Only an UNSENT add cancels. A real set the server knows about has to be
  // deleted for real.
  it("keeps a remove of a set the server already has", () => {
    enqueue({ kind: "removeSet", sessionId: 1, setId: 42 });
    expect(queuedCount()).toBe(1);
  });
});

describe("temp ids", () => {
  it("hands out negative ids, which server ids never are", () => {
    const first = nextTempSetId();
    const second = nextTempSetId();
    expect(first).toBeLessThan(0);
    expect(second).toBeLessThan(first);
    expect(isTempSetId(first)).toBe(true);
    expect(isTempSetId(42)).toBe(false);
  });
});

describe("flush", () => {
  it("sends each kind of write to its endpoint", async () => {
    enqueue({ kind: "updateSet", sessionId: 1, setId: 2, body: { actualReps: 5 } });
    enqueue({ kind: "removeSet", sessionId: 1, setId: 3 });
    enqueue({ kind: "updateSession", sessionId: 1, bodyweightLb: 180 });
    enqueue({ kind: "finishSession", sessionId: 1 });

    updateSessionSet.mockResolvedValue(ok({ id: 2 }));
    removeSessionSet.mockResolvedValue(ok());
    updateSession.mockResolvedValue(ok({ id: 1 }));
    finishSession.mockResolvedValue(ok({ id: 1 }));

    await flush();

    expect(updateSessionSet).toHaveBeenCalledOnce();
    expect(removeSessionSet).toHaveBeenCalledWith({ path: { sessionId: 1, setId: 3 } });
    expect(updateSession).toHaveBeenCalledWith({
      path: { sessionId: 1 },
      body: { bodyweightLb: 180 },
    });
    expect(finishSession).toHaveBeenCalledWith({ path: { sessionId: 1 } });
    expect(queuedCount()).toBe(0);
  });

  // These are ordered edits to the same rows. "Add a set, then log its reps"
  // is not the same pair in the other order, and the second cannot even be
  // addressed until the first has answered.
  it("sends strictly one at a time, in order", async () => {
    const order: string[] = [];
    updateSessionSet.mockImplementation(async () => {
      order.push("start");
      await new Promise((resolve) => setTimeout(resolve, 0));
      order.push("end");
      return ok({ id: 1 });
    });

    enqueue({ kind: "updateSet", sessionId: 1, setId: 1, body: { actualReps: 1 } });
    enqueue({ kind: "updateSet", sessionId: 1, setId: 2, body: { actualReps: 1 } });
    await flush();

    expect(order).toEqual(["start", "end", "start", "end"]);
  });

  it("repoints later writes at the real id once an added set lands", async () => {
    const temp = nextTempSetId();
    enqueue({ kind: "addSet", sessionId: 1, exerciseId: 3, tempSetId: temp });
    enqueue({ kind: "updateSet", sessionId: 1, setId: temp, body: { actualReps: 8 } });

    addSessionSet.mockResolvedValue(ok({ id: 501 }));
    updateSessionSet.mockResolvedValue(ok({ id: 501 }));

    await flush();

    expect(updateSessionSet).toHaveBeenCalledWith({
      path: { sessionId: 1, setId: 501 },
      body: { actualReps: 8 },
    });
  });

  // Still offline. Nothing is lost and nothing is retried into the void.
  it("stops and keeps everything when a write cannot reach the server", async () => {
    enqueue({ kind: "updateSet", sessionId: 1, setId: 1, body: { actualReps: 1 } });
    enqueue({ kind: "updateSet", sessionId: 1, setId: 2, body: { actualReps: 1 } });

    updateSessionSet.mockResolvedValue(unreachable());
    await flush();

    expect(queuedCount()).toBe(2);
    expect(rejectedCount()).toBe(0);
    expect(updateSessionSet).toHaveBeenCalledOnce();
  });

  it("picks up where it stopped once the server answers again", async () => {
    enqueue({ kind: "updateSet", sessionId: 1, setId: 1, body: { actualReps: 1 } });
    enqueue({ kind: "updateSet", sessionId: 1, setId: 2, body: { actualReps: 1 } });

    updateSessionSet.mockResolvedValue(unreachable());
    await flush();
    expect(queuedCount()).toBe(2);

    updateSessionSet.mockResolvedValue(ok({ id: 1 }));
    await flush();
    expect(queuedCount()).toBe(0);
  });

  // The server has seen this write and said no. Retrying sends the identical
  // body for the identical answer, forever — and a queue stuck on its first
  // entry never delivers the good writes behind it.
  it("drops a refused write and counts it, rather than blocking on it", async () => {
    enqueue({ kind: "updateSet", sessionId: 1, setId: 1, body: { actualReps: 1 } });
    enqueue({ kind: "updateSet", sessionId: 1, setId: 2, body: { actualReps: 2 } });

    updateSessionSet.mockResolvedValueOnce(refused()).mockResolvedValueOnce(ok({ id: 2 }));
    await flush();

    expect(queuedCount()).toBe(0);
    expect(rejectedCount()).toBe(1);
    expect(updateSessionSet).toHaveBeenCalledTimes(2);
  });

  it("reports a drained queue exactly once", async () => {
    const drained = vi.fn();
    onDrained(drained);

    enqueue({ kind: "updateSet", sessionId: 1, setId: 1, body: { actualReps: 1 } });
    updateSessionSet.mockResolvedValue(ok({ id: 1 }));

    await flush();
    expect(drained).toHaveBeenCalledOnce();

    // Nothing waiting, so nothing drained.
    await flush();
    expect(drained).toHaveBeenCalledOnce();
  });

  it("does not report a drain when it gave up offline", async () => {
    const drained = vi.fn();
    onDrained(drained);

    enqueue({ kind: "updateSet", sessionId: 1, setId: 1, body: { actualReps: 1 } });
    updateSessionSet.mockResolvedValue(unreachable());

    await flush();
    expect(drained).not.toHaveBeenCalled();
  });

  it("will not run twice at once", async () => {
    let release: (() => void) | undefined;
    updateSessionSet.mockImplementation(async () => {
      await new Promise<void>((resolve) => (release = resolve));
      return ok({ id: 1 });
    });

    enqueue({ kind: "updateSet", sessionId: 1, setId: 1, body: { actualReps: 1 } });
    const first = flush();
    await flush(); // returns immediately, sends nothing

    release?.();
    await first;
    expect(updateSessionSet).toHaveBeenCalledOnce();
  });

  it("marks the connection reachable again after a write lands", async () => {
    markUnreachable();
    enqueue({ kind: "updateSet", sessionId: 1, setId: 1, body: { actualReps: 1 } });
    updateSessionSet.mockResolvedValue(ok({ id: 1 }));

    await flush();

    expect(mustQueue()).toBe(false);
  });
});

describe("durability", () => {
  // The case this whole module exists for: a phone kills the backgrounded tab
  // between sets, or the lifter closes the app and drives home.
  it("survives a reload", () => {
    enqueue({ kind: "updateSet", sessionId: 1, setId: 7, body: { actualReps: 5 } });
    enqueue({ kind: "finishSession", sessionId: 1 });

    clearQueueInMemoryOnly();
    hydrateQueue();

    expect(queuedCount()).toBe(2);
  });

  it("replays what it read back", async () => {
    enqueue({ kind: "updateSet", sessionId: 1, setId: 7, body: { actualReps: 5 } });
    clearQueueInMemoryOnly();
    hydrateQueue();

    updateSessionSet.mockResolvedValue(ok({ id: 7 }));
    await flush();

    expect(updateSessionSet).toHaveBeenCalledWith({
      path: { sessionId: 1, setId: 7 },
      body: { actualReps: 5 },
    });
  });

  it("drops a stored queue written by a build with a different shape", () => {
    localStorage.setItem(
      "iron-temple:writes:v1",
      JSON.stringify({ version: 99, entries: [{ id: 1, kind: "somethingElse" }] }),
    );
    clearQueueInMemoryOnly();
    hydrateQueue();

    expect(queuedCount()).toBe(0);
  });

  it("survives an unreadable stored queue", () => {
    localStorage.setItem("iron-temple:writes:v1", "{not json");
    clearQueueInMemoryOnly();
    hydrateQueue();

    expect(queuedCount()).toBe(0);
  });

  it("keeps handing out unused temp ids after a reload", () => {
    const before = nextTempSetId();
    enqueue({ kind: "addSet", sessionId: 1, exerciseId: 3, tempSetId: before });

    clearQueueInMemoryOnly();
    hydrateQueue();

    // A reload that restarted at -1 would mint an id the restored queue is
    // already using, and the remap would then rewrite the wrong entry.
    expect(nextTempSetId()).toBeLessThan(before);
  });

  it("is emptied by clearQueue, storage included", () => {
    enqueue({ kind: "updateSet", sessionId: 1, setId: 7, body: { actualReps: 5 } });
    clearQueue();

    expect(queuedCount()).toBe(0);
    expect(localStorage.getItem("iron-temple:writes:v1")).toBeNull();
  });
});

/**
 * Drop the in-memory queue while leaving localStorage alone, which is what a
 * reload looks like from this module's point of view. clearQueue() cannot be
 * used — it wipes the stored copy, which is the thing under test.
 */
function clearQueueInMemoryOnly(): void {
  const stored = localStorage.getItem("iron-temple:writes:v1");
  clearQueue();
  if (stored !== null) localStorage.setItem("iron-temple:writes:v1", stored);
}
