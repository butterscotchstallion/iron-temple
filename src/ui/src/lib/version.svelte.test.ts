import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import {
  version,
  hasUpdate,
  dismissUpdate,
  poll,
  startPolling,
} from "./version.svelte";

// The store's only input is /health, so stub the generated client rather than
// the network.
const getHealth = vi.hoisted(() => vi.fn());
vi.mock("./api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./api")>()),
  getHealth,
}));

const health = (v: string, environment = "production") => ({
  data: { status: "ok", version: v, environment },
});

// Module-level $state outlives a single test, so wind it back by hand.
beforeEach(() => {
  version.running = "";
  version.latest = "";
  version.environment = "";
  version.dismissed = "";
  getHealth.mockReset();
});

afterEach(() => {
  vi.useRealTimers();
});

describe("version store", () => {
  it("takes the first answer as the running build", async () => {
    getHealth.mockResolvedValue(health("v1.2.3"));
    await poll();

    expect(version.running).toBe("v1.2.3");
    expect(version.latest).toBe("v1.2.3");
    expect(version.environment).toBe("production");
    expect(hasUpdate()).toBe(false);
  });

  // The baseline is the first answer of this page load, so re-polling the same
  // deployment must never look like a release.
  it("stays quiet while the version doesn't move", async () => {
    getHealth.mockResolvedValue(health("v1.2.3"));
    await poll();
    await poll();
    await poll();

    expect(hasUpdate()).toBe(false);
  });

  it("reports an update once the version changes underneath it", async () => {
    getHealth.mockResolvedValue(health("v1.2.3"));
    await poll();

    getHealth.mockResolvedValue(health("v1.3.0"));
    await poll();

    expect(hasUpdate()).toBe(true);
    // The label still names the build actually running.
    expect(version.running).toBe("v1.2.3");
    expect(version.latest).toBe("v1.3.0");
  });

  it("stops asking about a version that was declined", async () => {
    getHealth.mockResolvedValue(health("v1.2.3"));
    await poll();
    getHealth.mockResolvedValue(health("v1.3.0"));
    await poll();

    dismissUpdate();
    expect(hasUpdate()).toBe(false);

    // Polling again doesn't resurrect it.
    await poll();
    expect(hasUpdate()).toBe(false);
  });

  // Declining is per-version, not permanent — otherwise one "not now" mid-
  // workout would silence every release after it.
  it("asks again when a newer version lands after a decline", async () => {
    getHealth.mockResolvedValue(health("v1.2.3"));
    await poll();
    getHealth.mockResolvedValue(health("v1.3.0"));
    await poll();
    dismissUpdate();

    getHealth.mockResolvedValue(health("v1.4.0"));
    await poll();

    expect(hasUpdate()).toBe(true);
  });

  // An unreachable API is not a release. Without the "both known" guard, an
  // empty latest would compare unequal to running and prompt on every blip.
  it("stays silent when /health fails", async () => {
    getHealth.mockResolvedValue(health("v1.2.3"));
    await poll();

    getHealth.mockRejectedValue(new Error("network down"));
    await poll();

    expect(hasUpdate()).toBe(false);
    expect(version.running).toBe("v1.2.3");
    expect(version.latest).toBe("v1.2.3");
  });

  // `air` rebuilds the API on every save in `make dev`, and an unstamped build
  // reports "dev-<sha>" — so the string moves on a commit, a checkout, or merely
  // dirtying the tree. None of those is a deployment, and offering to reload for
  // one is both wrong and constant.
  it("never offers an update between dev builds", async () => {
    getHealth.mockResolvedValue(health("dev-0965a2e"));
    await poll();

    getHealth.mockResolvedValue(health("dev-55bdb2d"));
    await poll();

    expect(hasUpdate()).toBe(false);
    expect(version.running).toBe("dev-0965a2e");
    expect(version.latest).toBe("dev-55bdb2d");
  });

  it("ignores the dirty marker a local edit adds", async () => {
    getHealth.mockResolvedValue(health("dev-55bdb2d"));
    await poll();

    getHealth.mockResolvedValue(health("dev-55bdb2d-dirty"));
    await poll();

    expect(hasUpdate()).toBe(false);
  });

  // A build carrying no VCS info at all reports the bare fallback.
  it("treats a bare dev build as local too", async () => {
    getHealth.mockResolvedValue(health("dev"));
    await poll();

    getHealth.mockResolvedValue(health("dev-55bdb2d"));
    await poll();

    expect(hasUpdate()).toBe(false);
  });

  // Pointing a released bundle at a dev API is a misconfiguration, and reloading
  // would not fix it — so that direction is silent as well.
  it("does not offer a dev build to a release", async () => {
    getHealth.mockResolvedValue(health("v1.2.3"));
    await poll();

    getHealth.mockResolvedValue(health("dev-55bdb2d"));
    await poll();

    expect(hasUpdate()).toBe(false);
  });

  // The guard keys on "dev", not on "doesn't look like a tag" — a real release
  // must still be offered.
  it("still offers a real release", async () => {
    getHealth.mockResolvedValue(health("v0.27.0"));
    await poll();

    getHealth.mockResolvedValue(health("v0.28.0"));
    await poll();

    expect(hasUpdate()).toBe(true);
  });

  it("ignores an answer with no version in it", async () => {
    getHealth.mockResolvedValue({ data: { status: "ok" } });
    await poll();

    expect(version.running).toBe("");
    expect(hasUpdate()).toBe(false);
  });

  it("collapses overlapping polls into one request", async () => {
    let release!: () => void;
    getHealth.mockReturnValue(
      new Promise((resolve) => {
        release = () => resolve(health("v1.2.3"));
      }),
    );

    const first = poll();
    const second = poll(); // in flight — must not fire a second request
    release();
    await Promise.all([first, second]);

    expect(getHealth).toHaveBeenCalledTimes(1);
  });
});

describe("startPolling", () => {
  it("polls immediately and then on the interval", async () => {
    getHealth.mockResolvedValue(health("v1.2.3"));
    vi.useFakeTimers();

    const stop = startPolling();
    expect(getHealth).toHaveBeenCalledTimes(1);

    await vi.advanceTimersByTimeAsync(5 * 60 * 1000);
    expect(getHealth).toHaveBeenCalledTimes(2);

    stop();
    await vi.advanceTimersByTimeAsync(5 * 60 * 1000);
    expect(getHealth).toHaveBeenCalledTimes(2);
  });

  // A backgrounded tab has nobody to answer the dialog, so the request is
  // wasted — and on a phone it's wasted battery.
  it("skips the tick while the tab is hidden", async () => {
    getHealth.mockResolvedValue(health("v1.2.3"));
    vi.useFakeTimers();
    const hidden = vi
      .spyOn(document, "visibilityState", "get")
      .mockReturnValue("hidden");

    const stop = startPolling();
    expect(getHealth).not.toHaveBeenCalled();

    await vi.advanceTimersByTimeAsync(5 * 60 * 1000);
    expect(getHealth).not.toHaveBeenCalled();

    hidden.mockReturnValue("visible");
    await vi.advanceTimersByTimeAsync(5 * 60 * 1000);
    expect(getHealth).toHaveBeenCalledTimes(1);

    stop();
    hidden.mockRestore();
  });

  // Coming back to a tab that's been open for hours is when the answer is most
  // likely to have moved — but flicking between tabs mustn't mean a request per
  // flick.
  it("polls on refocus, but not more often than the floor allows", async () => {
    getHealth.mockResolvedValue(health("v1.2.3"));
    vi.useFakeTimers();

    const stop = startPolling();
    await vi.advanceTimersByTimeAsync(0);
    expect(getHealth).toHaveBeenCalledTimes(1);

    // Straight back: inside the floor, so nothing is asked.
    document.dispatchEvent(new Event("visibilitychange"));
    await vi.advanceTimersByTimeAsync(0);
    expect(getHealth).toHaveBeenCalledTimes(1);

    // Away long enough to be worth re-asking, but short of the interval — so
    // this second call can only have come from the refocus.
    await vi.advanceTimersByTimeAsync(90 * 1000);
    document.dispatchEvent(new Event("visibilitychange"));
    await vi.advanceTimersByTimeAsync(0);
    expect(getHealth).toHaveBeenCalledTimes(2);

    stop();
  });

  it("removes its listener on teardown", () => {
    vi.useFakeTimers();
    const remove = vi.spyOn(document, "removeEventListener");

    startPolling()();

    expect(remove).toHaveBeenCalledWith(
      "visibilitychange",
      expect.any(Function),
    );
    remove.mockRestore();
  });
});
