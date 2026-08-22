import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";

// The module holds one lazily-created AudioContext, so each test imports it
// fresh rather than inheriting the previous test's device.
async function load() {
  vi.resetModules();
  return import("./restAlert");
}

// A minimal Web Audio stand-in. jsdom has no AudioContext at all, which is
// itself one of the cases under test — the real one is only installed by the
// tests that care what came out of it.
function fakeAudio() {
  const notes: number[] = [];
  class Node {
    connect(next: unknown) {
      return next;
    }
  }
  class Osc extends Node {
    type = "";
    frequency = { value: 0 };
    start(at: number) {
      notes.push(at);
    }
    stop() {}
  }
  class Gain extends Node {
    gain = {
      setValueAtTime: vi.fn(),
      linearRampToValueAtTime: vi.fn(),
    };
  }
  class Ctx {
    state = "running";
    currentTime = 0;
    destination = {};
    resume = vi.fn(async () => {});
    createOscillator() {
      return new Osc();
    }
    createGain() {
      return new Gain();
    }
  }
  return { Ctx, notes };
}

const vibrate = vi.fn();

beforeEach(() => {
  localStorage.clear();
  vibrate.mockClear();
  Object.defineProperty(navigator, "vibrate", {
    value: vibrate,
    configurable: true,
  });
});

afterEach(() => {
  // @ts-expect-error — removing the stub between tests
  delete globalThis.AudioContext;
});

describe("restAlert mute preference", () => {
  it("defaults to unmuted and round-trips through localStorage", async () => {
    const { isMuted, setMuted } = await load();
    expect(isMuted()).toBe(false);

    setMuted(true);
    expect(isMuted()).toBe(true);
    setMuted(false);
    expect(isMuted()).toBe(false);
  });

  // Unlike the countdown itself, the preference outlives the tab — you mute the
  // timer because of the room you train in, not because of this workout.
  it("persists across a reload", async () => {
    const first = await load();
    first.setMuted(true);

    const second = await load();
    expect(second.isMuted()).toBe(true);
  });

  // Safari's private mode throws on write and some webviews deny access
  // outright. A lost preference is not worth an exception on the way to a chime.
  it("reads as unmuted when storage throws", async () => {
    const { isMuted, setMuted } = await load();
    const spy = vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("denied");
    });
    const write = vi
      .spyOn(Storage.prototype, "setItem")
      .mockImplementation(() => {
        throw new Error("denied");
      });

    expect(isMuted()).toBe(false);
    expect(() => setMuted(true)).not.toThrow();

    spy.mockRestore();
    write.mockRestore();
  });
});

describe("restAlert firing", () => {
  it("plays two notes and buzzes once primed", async () => {
    const { Ctx, notes } = fakeAudio();
    // @ts-expect-error — installing the stub
    globalThis.AudioContext = Ctx;

    const { prime, fire } = await load();
    prime();
    fire();

    expect(notes).toHaveLength(2);
    expect(vibrate).toHaveBeenCalledOnce();
  });

  // The chime needs a device opened from a user gesture; the buzz does not. A
  // page that never called prime() still gets the half of the alert it can have.
  it("still buzzes when no audio context was opened", async () => {
    const { fire } = await load();
    expect(() => fire()).not.toThrow();
    expect(vibrate).toHaveBeenCalledOnce();
  });

  it("does nothing at all when muted", async () => {
    const { Ctx, notes } = fakeAudio();
    // @ts-expect-error — installing the stub
    globalThis.AudioContext = Ctx;

    const { prime, fire, setMuted } = await load();
    setMuted(true);
    prime();
    fire();

    expect(notes).toHaveLength(0);
    expect(vibrate).not.toHaveBeenCalled();
  });

  // jsdom has no AudioContext, so this is the environment the constructor
  // actually fails in — the point being that it fails silently.
  it("survives a browser with no Web Audio", async () => {
    const { prime, fire } = await load();
    expect(() => prime()).not.toThrow();
    expect(() => fire()).not.toThrow();
  });

  it("survives a browser that exposes vibrate and then refuses it", async () => {
    vibrate.mockImplementation(() => {
      throw new Error("not allowed");
    });
    const { fire } = await load();
    expect(() => fire()).not.toThrow();
  });
});
