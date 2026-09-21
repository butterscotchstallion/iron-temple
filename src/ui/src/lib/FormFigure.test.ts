import { describe, it, expect, afterEach, vi } from "vitest";
import { render } from "@testing-library/svelte";

import FormFigure from "./FormFigure.svelte";
import { FRAMES } from "./formKinematics";

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
});

describe("FormFigure", () => {
  it("draws a movement it knows", () => {
    const { container } = render(FormFigure, { name: "Squat" });
    expect(container.querySelector("svg")).toBeInTheDocument();
    expect(container.querySelectorAll("polyline").length).toBeGreaterThan(1);
  });

  // The whole animation. A mis-namespaced <animate> still renders and still
  // fails silently — it just never animates — so existence alone proves
  // nothing; the frame count is what says the timeline is really there.
  it("animates across every sampled frame", () => {
    const { container } = render(FormFigure, { name: "Squat" });
    const animate = container.querySelector("animate");
    expect(animate).not.toBeNull();
    expect(animate!.getAttribute("values")!.split(";")).toHaveLength(FRAMES);
  });

  // Emitting nothing rather than calling pauseAnimations(): it needs no element
  // reference, it never builds the forty-frame strings, and pauseAnimations is
  // not implemented in jsdom, so testing that path would assert a stub.
  it("emits no animation at all when the lifter has asked for less movement", () => {
    setReducedMotion(true);
    const { container } = render(FormFigure, { name: "Squat" });
    expect(container.querySelector("svg")).toBeInTheDocument();
    expect(container.querySelector("animate")).toBeNull();
  });

  it("says so in the label when it is a still diagram", () => {
    setReducedMotion(true);
    const { container } = render(FormFigure, { name: "Squat" });
    expect(container.querySelector("svg")).toHaveAttribute(
      "aria-label",
      "Diagram of the Squat position",
    );
  });

  it("holds still without animating for a static hold", () => {
    // A plank's demonstration is that nothing moves, so the "identical values
    // emit no animation" rule makes it a still image with no special case.
    const { container } = render(FormFigure, { name: "Plank" });
    expect(container.querySelector("svg")).toBeInTheDocument();
    expect(container.querySelector("animate")).toBeNull();
  });

  it("renders nothing for a movement it has no figure for", () => {
    const { container } = render(FormFigure, { name: "Copenhagen Plank" });
    expect(container.querySelector("svg")).toBeNull();
  });
});
