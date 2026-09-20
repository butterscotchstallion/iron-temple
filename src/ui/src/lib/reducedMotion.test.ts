import { describe, it, expect, afterEach, vi } from "vitest";

import { prefersReducedMotion } from "./reducedMotion";

function setMatchMedia(reduce: boolean) {
  vi.stubGlobal("matchMedia", (query: string) => ({
    matches: reduce && query.includes("prefers-reduced-motion"),
    media: query,
    addEventListener: () => {},
    removeEventListener: () => {},
  }));
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("prefersReducedMotion", () => {
  it("is false when the lifter has expressed no preference", () => {
    setMatchMedia(false);
    expect(prefersReducedMotion()).toBe(false);
  });

  it("is true when the lifter has asked for less movement", () => {
    setMatchMedia(true);
    expect(prefersReducedMotion()).toBe(true);
  });

  // No answer is not the same as "yes". A browser too old for matchMedia gets
  // the animation rather than a permanently frozen app.
  it("is false where the question cannot be asked", () => {
    vi.stubGlobal("matchMedia", undefined);
    expect(prefersReducedMotion()).toBe(false);
  });

  // The setting is a Control Centre toggle on iOS, so it can flip while the app
  // is open. Nothing may be cached across calls.
  it("notices the preference changing between calls", () => {
    setMatchMedia(false);
    expect(prefersReducedMotion()).toBe(false);
    setMatchMedia(true);
    expect(prefersReducedMotion()).toBe(true);
  });
});
