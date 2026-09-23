import { describe, it, expect } from "vitest";
import { render } from "@testing-library/svelte";
import Skeleton from "./Skeleton.svelte";

/** The bar itself — it has no role, so it is read off the container. */
function bar(container: HTMLElement): HTMLElement {
  const el = container.querySelector("div");
  if (!el) throw new Error("Skeleton rendered nothing");
  return el;
}

describe("Skeleton", () => {
  it("is hidden from assistive tech", () => {
    // The <Loading> region around it carries the announcement; a dozen empty
    // boxes announcing themselves would bury it.
    const { container } = render(Skeleton, { class: "w-10" });
    expect(bar(container)).toHaveAttribute("aria-hidden", "true");
  });

  it("holds still for a lifter who has asked for less movement", () => {
    const { container } = render(Skeleton, { class: "w-10" });
    expect(bar(container).className).toContain("motion-reduce:animate-none");
  });

  // The whole point of the component: a placeholder that is not the height of
  // the text it replaces does not prevent a reflow, it only delays one.
  it.each([
    ["xs", "h-4"],
    ["sm", "h-5"],
    ["2xl", "h-8"],
    ["5xl", "h-12"],
  ] as const)("gives a %s line the height of its line box", (text, height) => {
    const { container } = render(Skeleton, { text });
    expect(bar(container).className.split(/\s+/)).toContain(height);
  });

  it("takes a height from the caller when it isn't standing in for text", () => {
    const { container } = render(Skeleton, { class: "h-40 w-full" });
    const classes = bar(container).className.split(/\s+/);
    expect(classes).toContain("h-40");
    // Nothing from the type scale, since no `text` was asked for.
    expect(classes.filter((c) => /^h-\d/.test(c))).toEqual(["h-40"]);
  });
});
