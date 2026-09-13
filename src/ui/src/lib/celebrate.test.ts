import { afterEach, describe, expect, it, vi } from "vitest";
import { celebrate } from "./celebrate";

// Asserts on whether the confetti actually fires, not on whether its chunk was
// fetched. The early return does skip the dynamic import — a lifter who will
// never see confetti never downloads it — but the module caches that promise
// after the first celebration, so "was it imported" is only observable in
// whichever test happens to run first. Firing is observable every time, and is
// the behaviour that matters.
const fire = vi.hoisted(() => vi.fn());
vi.mock("canvas-confetti", () => ({ default: fire }));

function setReducedMotion(reduce: boolean) {
  vi.stubGlobal("matchMedia", (query: string) => ({
    matches: reduce && query.includes("prefers-reduced-motion"),
    media: query,
    addEventListener: () => {},
    removeEventListener: () => {},
  }));
}

afterEach(() => {
  vi.unstubAllGlobals();
  fire.mockClear();
});

describe("celebrate", () => {
  it("fires when nothing says otherwise", async () => {
    setReducedMotion(false);
    celebrate({ particleCount: 10 });
    await vi.waitFor(() => expect(fire).toHaveBeenCalledWith({ particleCount: 10 }));
  });

  // Two hundred particles thrown across the screen is exactly what the
  // preference exists to stop, and this app is used in a gym where you cannot
  // simply look away.
  it("stays still when the lifter has asked for less motion", async () => {
    setReducedMotion(true);
    celebrate({ particleCount: 10 });
    await new Promise((r) => setTimeout(r, 20));
    expect(fire).not.toHaveBeenCalled();
  });

  // No answer is not the same as "yes" — an old browser still gets confetti.
  it("celebrates where the question cannot be asked", async () => {
    vi.stubGlobal("matchMedia", undefined);
    celebrate({ particleCount: 10 });
    await vi.waitFor(() => expect(fire).toHaveBeenCalled());
  });

  // The setting is a Control Centre toggle on iOS — people reach for it
  // mid-session, so it is asked at each celebration rather than cached.
  it("notices the preference changing while the app is open", async () => {
    setReducedMotion(false);
    celebrate({ particleCount: 10 });
    await vi.waitFor(() => expect(fire).toHaveBeenCalledTimes(1));

    setReducedMotion(true);
    celebrate({ particleCount: 10 });
    await new Promise((r) => setTimeout(r, 20));
    expect(fire).toHaveBeenCalledTimes(1);
  });
});
