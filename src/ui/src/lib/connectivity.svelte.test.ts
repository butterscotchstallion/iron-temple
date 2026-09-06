import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TRANSPORT_FAILURE_STATUS } from "./apiFetch";
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
  // The discriminator is the sentinel status. Every call resolves rather than
  // throws, and carries the sentinel only when fetch itself rejected — see
  // apiFetch.ts for why it is encoded as a status rather than an exception.
  it("is true for the transport-failure sentinel", () => {
    expect(isTransportFailure({ status: TRANSPORT_FAILURE_STATUS })).toBe(true);
  });

  // A 500 is a REACHABLE server that had an opinion. Calling it offline would
  // queue writes the server has already refused and retry them forever.
  it("is false for an error the server answered with", () => {
    expect(isTransportFailure({ status: 500 })).toBe(false);
  });

  it("is false for a success", () => {
    expect(isTransportFailure({ status: 200 })).toBe(false);
  });
});

describe("observe", () => {
  it("goes offline on a transport failure and reports it", () => {
    expect(observe({ status: TRANSPORT_FAILURE_STATUS })).toBe(true);
    expect(isOnline()).toBe(false);
  });

  it("comes back online on any answer at all, including an error", () => {
    markUnreachable();

    expect(observe({ status: 409 })).toBe(false);
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
