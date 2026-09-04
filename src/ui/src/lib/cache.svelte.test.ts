import { describe, it, expect, beforeEach, vi } from "vitest";
import {
  CACHE_KEYS,
  cachedValue,
  clearCache,
  fetchThrough,
  invalidate,
  invalidateTraining,
  rackedKey,
} from "./cache.svelte";

const KEY = "test:key";

beforeEach(() => {
  clearCache();
});

describe("cachedValue", () => {
  it("is undefined before anything has been fetched", () => {
    expect(cachedValue(KEY)).toBeUndefined();
  });

  it("distinguishes a cached empty result from never having loaded", async () => {
    // The difference callers key their skeleton off: a lifter with no sessions
    // should see the empty state, not a spinner.
    await fetchThrough(KEY, async () => ({ data: [] }));
    expect(cachedValue(KEY)).toEqual([]);
    expect(cachedValue(KEY)).not.toBeUndefined();
  });
});

describe("fetchThrough", () => {
  it("returns the result untouched and remembers a success", async () => {
    const result = await fetchThrough(KEY, async () => ({ data: { n: 1 } }));
    expect(result).toEqual({ data: { n: 1 } });
    expect(cachedValue(KEY)).toEqual({ n: 1 });
  });

  it("does not cache a failure", async () => {
    await fetchThrough(KEY, async () => ({ error: { message: "nope" } }));
    expect(cachedValue(KEY)).toBeUndefined();
  });

  it("keeps the last good value when a later fetch fails", async () => {
    await fetchThrough(KEY, async () => ({ data: "good" }));
    await fetchThrough(KEY, async () => ({ error: { message: "blip" } }));
    // A network blip must not evict what the lifter is reading.
    expect(cachedValue(KEY)).toBe("good");
  });

  it("dedupes concurrent calls into one request", async () => {
    // This is what makes the shell's prefetch a prefetch: the route that wants
    // the data joins the in-flight promise instead of issuing a second call.
    const call = vi.fn(async () => ({ data: "once" }));
    const [a, b] = await Promise.all([
      fetchThrough(KEY, call),
      fetchThrough(KEY, call),
    ]);
    expect(call).toHaveBeenCalledTimes(1);
    expect(a).toEqual({ data: "once" });
    expect(b).toEqual({ data: "once" });
  });

  it("fetches again once the in-flight request has settled", async () => {
    const call = vi.fn(async () => ({ data: "fresh" }));
    await fetchThrough(KEY, call);
    await fetchThrough(KEY, call);
    // Revalidation on every mount is the freshness mechanism, so a settled key
    // must not be pinned to its first answer.
    expect(call).toHaveBeenCalledTimes(2);
  });

  it("releases the in-flight slot when the call rejects", async () => {
    const boom = vi.fn(async () => {
      throw new Error("network down");
    });
    await expect(fetchThrough(KEY, boom)).rejects.toThrow("network down");
    // A rejected request that left its promise parked would wedge the key: every
    // later read would await a promise that already failed.
    await fetchThrough(KEY, async () => ({ data: "recovered" }));
    expect(cachedValue(KEY)).toBe("recovered");
  });
});

describe("invalidate", () => {
  it("drops one key and leaves the rest", async () => {
    await fetchThrough(CACHE_KEYS.homeSessions, async () => ({ data: "home" }));
    await fetchThrough(CACHE_KEYS.allExercises, async () => ({ data: "library" }));

    invalidate(CACHE_KEYS.homeSessions);
    expect(cachedValue(CACHE_KEYS.homeSessions)).toBeUndefined();
    expect(cachedValue(CACHE_KEYS.allExercises)).toBe("library");
  });
});

describe("invalidateTraining", () => {
  it("drops everything derived from performed work, and nothing else", async () => {
    for (const key of Object.values(CACHE_KEYS)) {
      await fetchThrough(key, async () => ({ data: key }));
    }

    invalidateTraining();

    expect(cachedValue(CACHE_KEYS.homeSessions)).toBeUndefined();
    expect(cachedValue(CACHE_KEYS.historyFirstPage)).toBeUndefined();
    expect(cachedValue(CACHE_KEYS.performedExercises)).toBeUndefined();
    // The library is the catalogue of movements, not a record of training, so
    // finishing a workout does not change it.
    expect(cachedValue(CACHE_KEYS.allExercises)).toBe(CACHE_KEYS.allExercises);
  });
});

describe("clearCache", () => {
  it("forgets every key", async () => {
    for (const key of Object.values(CACHE_KEYS)) {
      await fetchThrough(key, async () => ({ data: key }));
    }

    clearCache();

    for (const key of Object.values(CACHE_KEYS)) {
      expect(cachedValue(key)).toBeUndefined();
    }
  });

  it("drops a request that was already in flight when the cache was cleared", async () => {
    // The sign-out leak: A's request is in the air, A signs out, and the reply
    // lands afterwards. Storing it would put A's training data back into the
    // cache that sign-out just emptied, where B's first paint would read it.
    let land!: (result: { data: string }) => void;
    const inFlight = fetchThrough(
      CACHE_KEYS.homeSessions,
      () => new Promise<{ data: string }>((resolve) => (land = resolve)),
    );

    clearCache();
    land({ data: "previous lifter's sessions" });
    await inFlight;

    expect(cachedValue(CACHE_KEYS.homeSessions)).toBeUndefined();
  });

  it("leaves a request started after the clear able to cache normally", async () => {
    // The generation guard must silence only the requests it straddles — the
    // next lifter's own fetches have to work.
    clearCache();
    await fetchThrough(CACHE_KEYS.homeSessions, async () => ({ data: "new lifter" }));
    expect(cachedValue(CACHE_KEYS.homeSessions)).toBe("new lifter");
  });

  it("does not let a straddling request retire a newer request's dedupe slot", async () => {
    let landFirst!: (result: { data: string }) => void;
    const first = fetchThrough(
      CACHE_KEYS.homeSessions,
      () => new Promise<{ data: string }>((resolve) => (landFirst = resolve)),
    );

    clearCache();

    // The next lifter's request now owns the key, and is itself still in
    // flight — which is the only state in which the slot can be stolen.
    let landSecond!: (result: { data: string }) => void;
    const second = vi.fn(
      () => new Promise<{ data: string }>((resolve) => (landSecond = resolve)),
    );
    const a = fetchThrough(CACHE_KEYS.homeSessions, second);

    // The old one settles and cleans up after itself — it must not evict the
    // slot `a` is parked in, or the dedupe below silently stops working.
    landFirst({ data: "previous lifter" });
    await first;

    const b = fetchThrough(CACHE_KEYS.homeSessions, second);
    landSecond({ data: "new lifter" });
    await Promise.all([a, b]);

    expect(second).toHaveBeenCalledTimes(1);
    expect(cachedValue(CACHE_KEYS.homeSessions)).toBe("new lifter");
  });
});

describe("rackedKey", () => {
  it("gives each period its own entry", async () => {
    await fetchThrough(rackedKey("week"), async () => ({ data: "week report" }));
    await fetchThrough(rackedKey("month"), async () => ({ data: "month report" }));
    // Switching period and back must not re-fetch what was already read.
    expect(cachedValue(rackedKey("week"))).toBe("week report");
    expect(cachedValue(rackedKey("month"))).toBe("month report");
  });

  it("is dropped for every period when training changes", async () => {
    for (const period of ["week", "month", "year"]) {
      await fetchThrough(rackedKey(period), async () => ({ data: period }));
    }
    await fetchThrough(CACHE_KEYS.allExercises, async () => ({ data: "library" }));

    invalidateTraining();

    // A workout does not announce which periods it lands in — training on the
    // 1st moves the week, the month and the year at once.
    for (const period of ["week", "month", "year"]) {
      expect(cachedValue(rackedKey(period))).toBeUndefined();
    }
    // The catalogue of movements is not a record of training.
    expect(cachedValue(CACHE_KEYS.allExercises)).toBe("library");
  });
});

describe("persistence across a reload", () => {
  // The module hydrates once at import, so a reload is simulated by writing
  // what a previous page load would have left behind and re-importing it.
  async function reload() {
    vi.resetModules();
    return import("./cache.svelte");
  }

  it("mirrors values into sessionStorage under a per-build prefix", async () => {
    await fetchThrough(CACHE_KEYS.homeSessions, async () => ({ data: { items: [1] } }));

    const keys = Object.keys(sessionStorage);
    const mirrored = keys.filter((k) => k.endsWith(CACHE_KEYS.homeSessions));
    expect(mirrored).toHaveLength(1);
    // Namespaced by build, so a release that changes a response shape cannot
    // hydrate the previous build's values.
    expect(mirrored[0]).toMatch(/^iron-temple:cache:/);
  });

  it("hydrates what the previous page load left behind", async () => {
    await fetchThrough(CACHE_KEYS.homeSessions, async () => ({ data: "before reload" }));

    const fresh = await reload();
    expect(fresh.cachedValue(fresh.CACHE_KEYS.homeSessions)).toBe("before reload");
  });

  it("does not carry a signed-out lifter's data into the next page load", async () => {
    await fetchThrough(CACHE_KEYS.homeSessions, async () => ({ data: "private" }));
    clearCache();

    const fresh = await reload();
    expect(fresh.cachedValue(fresh.CACHE_KEYS.homeSessions)).toBeUndefined();
  });

  it("forgets an invalidated key on the next page load too", async () => {
    await fetchThrough(CACHE_KEYS.homeSessions, async () => ({ data: "stale" }));
    invalidate(CACHE_KEYS.homeSessions);

    const fresh = await reload();
    expect(fresh.cachedValue(fresh.CACHE_KEYS.homeSessions)).toBeUndefined();
  });
});
