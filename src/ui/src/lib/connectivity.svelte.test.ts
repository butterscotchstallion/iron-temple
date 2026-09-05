import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  isOnline,
  isTransportFailure,
  markReachable,
  markUnreachable,
  observe,
  resetConnectivity,
  watchConnectivity,
} from "./connectivity.svelte";

beforeEach(() => {
  resetConnectivity();
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("isTransportFailure", () => {
  // The discriminator is the absence of a response. The generated client
  // resolves rather than throws, and leaves `response` undefined only when
  // fetch itself rejected.
  it("is true for an error with no response", () => {
    expect(isTransportFailure({ error: new TypeError("Failed to fetch") })).toBe(true);
  });

  // A 500 is a REACHABLE server that had an opinion. Calling it offline would
  // queue writes the server has already refused and retry them forever.
  it("is false for an error the server answered with", () => {
    expect(
      isTransportFailure({
        error: { message: "no" },
        response: new Response("", { status: 500 }),
      }),
    ).toBe(false);
  });

  it("is false for a success", () => {
    expect(isTransportFailure({ response: new Response() })).toBe(false);
  });
});

describe("observe", () => {
  it("goes offline on a transport failure and reports it", () => {
    expect(observe({ error: new TypeError("Failed to fetch") })).toBe(true);
    expect(isOnline()).toBe(false);
  });

  it("comes back online on any answer at all, including an error", () => {
    markUnreachable();

    expect(
      observe({ error: { message: "no" }, response: new Response("", { status: 409 }) }),
    ).toBe(false);
    expect(isOnline()).toBe(true);
  });
});

describe("watchConnectivity", () => {
  it("goes offline when the browser says the network is gone", () => {
    const stop = watchConnectivity();

    window.dispatchEvent(new Event("offline"));
    expect(isOnline()).toBe(false);

    stop();
  });

  // navigator.onLine is reliable in one direction only. `online` means the OS
  // has a link, which is not the same as the API being reachable — a captive
  // portal fires it while every request still fails. So it triggers a retry
  // and nothing else; the retry's own result decides the state.
  it("does not clear the offline state on its own when the link returns", () => {
    const retry = vi.fn();
    const stop = watchConnectivity(retry);

    window.dispatchEvent(new Event("offline"));
    window.dispatchEvent(new Event("online"));

    expect(retry).toHaveBeenCalledOnce();
    expect(isOnline()).toBe(false);

    // Only something actually getting through clears it.
    markReachable();
    expect(isOnline()).toBe(true);

    stop();
  });

  it("starts offline in a tab opened with the radio off", () => {
    vi.spyOn(navigator, "onLine", "get").mockReturnValue(false);

    const stop = watchConnectivity();
    expect(isOnline()).toBe(false);

    stop();
  });

  it("stops listening once torn down", () => {
    const retry = vi.fn();
    const stop = watchConnectivity(retry);
    stop();

    window.dispatchEvent(new Event("offline"));
    window.dispatchEvent(new Event("online"));

    expect(retry).not.toHaveBeenCalled();
    expect(isOnline()).toBe(true);
  });
});
