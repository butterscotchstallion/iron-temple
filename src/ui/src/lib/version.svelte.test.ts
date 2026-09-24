import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import {
  version,
  hasUpdate,
  dismissUpdate,
  poll,
  startPolling,
  updateNotes,
} from "./version.svelte";

// The store's only input is /health, so stub the generated client rather than
// the network.
const getHealth = vi.hoisted(() => vi.fn());
vi.mock("./api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./api")>()),
  getHealth,
}));

const health = (v: string, environment = "production") => ({
  status: 200,
  data: { status: "ok", version: v, environment },
  headers: new Headers(),
});

// The notes are the one thing the store fetches itself, changelog.json being a
// static asset of the bundle rather than an API call. Stubbed at the global, the
// way accountExport.test.ts stubs its download.
const fetchMock = vi.fn();

/** A changelog.json response naming the release it describes. */
const changelog = (v: string, entries = [`feat(ui): something shipped in ${v} (abc1234)`]) =>
  new Response(JSON.stringify({ version: v, entries }), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });

// Module-level $state outlives a single test, so wind it back by hand.
beforeEach(() => {
  version.running = "";
  version.latest = "";
  version.environment = "";
  version.dismissed = "";
  version.notes = { version: "", entries: [] };
  getHealth.mockReset();
  fetchMock.mockReset();
  // Default to a 404, so a test that never opts in can't accidentally depend on
  // a body — and so the "nothing there" path is what's exercised by the cases
  // that are about something else.
  fetchMock.mockResolvedValue(new Response("", { status: 404 }));
  vi.stubGlobal("fetch", fetchMock);
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
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
    getHealth.mockResolvedValue({
      status: 200,
      data: { status: "ok" },
      headers: new Headers(),
    });
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

// The notes for the build being OFFERED, which by definition aren't in this
// bundle — it only knows what's in itself. They come off the deployed bundle's
// changelog.json, which means they can arrive late, arrive stale, or not arrive.
describe("release notes for the offered build", () => {
  /**
   * Poll, then let the notes fetch it may have started finish.
   *
   * poll() deliberately does NOT await loadNotes() — a changelog.json that never
   * answers must not hold the prompt back — so on return the fetch is still in
   * the air. Draining it here rather than wrapping each assertion in waitFor is
   * what makes the negative cases mean anything: an empty store looks identical
   * before an answer arrives and after a rejected one, so a test that didn't wait
   * would pass without ever exercising the guard it names.
   */
  async function pollNotes() {
    await poll();
    await Promise.allSettled(fetchMock.mock.results.map((result) => result.value));
    // response.json() is a further await inside loadNotes; yield past it.
    await new Promise((resolve) => setTimeout(resolve, 0));
  }

  /** Baseline on `running`, then have `latest` move to a new release. */
  async function release(running: string, latest: string) {
    getHealth.mockResolvedValue(health(running));
    await pollNotes();
    getHealth.mockResolvedValue(health(latest));
    await pollNotes();
  }

  it("fetches the new build's notes when a release lands", async () => {
    fetchMock.mockResolvedValue(changelog("v1.3.0", ["feat(ui): a thing (abc1234)"]));
    await release("v1.2.3", "v1.3.0");

    expect(version.notes).toEqual({
      version: "v1.3.0",
      entries: ["feat(ui): a thing (abc1234)"],
    });
    expect(updateNotes()).toEqual(["feat(ui): a thing (abc1234)"]);
  });

  // Nothing to describe: the running build's notes are already in the bundle.
  it("asks for nothing while the version holds steady", async () => {
    getHealth.mockResolvedValue(health("v1.2.3"));
    await pollNotes();
    await pollNotes();

    expect(fetchMock).not.toHaveBeenCalled();
  });

  // The pods roll one at a time, so the API can be serving the new version while
  // nginx is still handing out the old bundle — and with it the old notes.
  it("keeps nothing when the file names an older release", async () => {
    fetchMock.mockResolvedValue(changelog("v1.2.3"));
    await release("v1.2.3", "v1.3.0");

    expect(version.notes.entries).toEqual([]);
    expect(updateNotes()).toEqual([]);
    // Still offered — the notes are the trimming, not the point.
    expect(hasUpdate()).toBe(true);
  });

  // The other half of the rollout window: once the UI pods catch up, the next
  // poll has to pick the notes up rather than having given up on them.
  it("asks again after a mismatch, and takes them when they arrive", async () => {
    fetchMock.mockResolvedValue(changelog("v1.2.3"));
    await release("v1.2.3", "v1.3.0");
    expect(updateNotes()).toEqual([]);

    fetchMock.mockResolvedValue(changelog("v1.3.0", ["fix(api): later (def5678)"]));
    await pollNotes();

    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(updateNotes()).toEqual(["fix(api): later (def5678)"]);
  });

  // The counterpart: a success is final, so the five-minute poll doesn't spend a
  // request re-reading notes it already has.
  it("stops asking once it has the notes for the offered build", async () => {
    fetchMock.mockResolvedValue(changelog("v1.3.0"));
    await release("v1.2.3", "v1.3.0");

    await pollNotes();
    await pollNotes();

    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("fetches again when a newer release lands", async () => {
    fetchMock.mockResolvedValue(changelog("v1.3.0"));
    await release("v1.2.3", "v1.3.0");

    fetchMock.mockResolvedValue(changelog("v1.4.0", ["feat(ui): newer still (99aabb)"]));
    getHealth.mockResolvedValue(health("v1.4.0"));
    await pollNotes();

    expect(updateNotes()).toEqual(["feat(ui): newer still (99aabb)"]);
  });

  // A release landing while the previous fetch is still in the air: the pod may
  // now be serving the NEWER file, which is current rather than stale.
  it("accepts a file naming the release that landed mid-flight", async () => {
    let answer!: (response: Response) => void;
    fetchMock.mockReturnValue(new Promise<Response>((resolve) => (answer = resolve)));

    getHealth.mockResolvedValue(health("v1.2.3"));
    await poll();
    getHealth.mockResolvedValue(health("v1.3.0"));
    await poll();

    // v1.4.0 lands before the v1.3.0 request comes back, and the pod answers
    // with v1.4.0's notes.
    version.latest = "v1.4.0";
    answer(changelog("v1.4.0", ["feat(ui): overtook it (aa11bb)"]));
    await vi.waitFor(() => expect(version.notes.version).toBe("v1.4.0"));

    expect(updateNotes()).toEqual(["feat(ui): overtook it (aa11bb)"]);
  });

  // Declining leaves that release's notes in the store. Once a newer one lands
  // and its fetch misses, they'd be the only notes there — and captioning them
  // with the new version would be a lie about what's in it.
  it("shows nothing rather than the declined release's notes", async () => {
    fetchMock.mockResolvedValue(changelog("v1.3.0", ["feat(ui): in v1.3.0 (abc1234)"]));
    await release("v1.2.3", "v1.3.0");
    dismissUpdate();

    // v1.4.0 lands, but the pods haven't rolled, so its fetch is rejected.
    fetchMock.mockResolvedValue(changelog("v1.3.0", ["feat(ui): in v1.3.0 (abc1234)"]));
    getHealth.mockResolvedValue(health("v1.4.0"));
    await pollNotes();

    expect(hasUpdate()).toBe(true);
    expect(updateNotes()).toEqual([]);
  });

  it("asks for nothing between dev builds", async () => {
    await release("dev-0965a2e", "dev-55bdb2d");

    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("asks for nothing about a version already declined", async () => {
    fetchMock.mockResolvedValue(changelog("v1.3.0"));
    await release("v1.2.3", "v1.3.0");
    dismissUpdate();
    fetchMock.mockClear();

    await pollNotes();

    expect(fetchMock).not.toHaveBeenCalled();
  });

  // Every one of these is a normal thing to meet mid-deploy. None may throw, and
  // none may leave a half-read object in the store.
  it.each([
    ["a 404", () => new Response("", { status: 404 })],
    // What an SPA fallback answers for a path it has no file for — a 200, so
    // response.ok is not enough on its own.
    [
      "index.html from an SPA fallback",
      () => new Response("<!doctype html><html></html>", { status: 200 }),
    ],
    ["a body that isn't the right shape", () => new Response(JSON.stringify({ version: 3 }))],
    ["entries that aren't a list", () => new Response(JSON.stringify({ version: "v1.3.0", entries: "nope" }))],
  ])("survives %s", async (_label, respond) => {
    fetchMock.mockResolvedValue(respond());
    await release("v1.2.3", "v1.3.0");

    expect(version.notes).toEqual({ version: "", entries: [] });
    expect(updateNotes()).toEqual([]);
    expect(hasUpdate()).toBe(true);
  });

  it("survives the request rejecting outright", async () => {
    fetchMock.mockRejectedValue(new Error("offline"));
    await release("v1.2.3", "v1.3.0");

    expect(updateNotes()).toEqual([]);
    expect(hasUpdate()).toBe(true);
  });

  // Whatever else is in the file, only strings reach the list — the template
  // renders each entry directly.
  it("drops entries that aren't strings", async () => {
    fetchMock.mockResolvedValue(
      new Response(JSON.stringify({ version: "v1.3.0", entries: ["real", 7, null] })),
    );
    await release("v1.2.3", "v1.3.0");

    expect(updateNotes()).toEqual(["real"]);
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
