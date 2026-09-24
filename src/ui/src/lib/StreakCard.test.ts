import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/svelte";
import { tick } from "svelte";
import StreakCard from "./StreakCard.svelte";
import { combinedHeat, heatColor, streakHeat } from "./streakHeat";

// The only thing stubbed: jsdom has no matchMedia, so prefersReducedMotion would
// take its "cannot ask" branch and always report false. The heat ladder itself is
// the real streakHeat.ts — mocking it would leave the mapping from streak count
// to what's actually on screen untested on the only surface that draws it.
const { reducedMotionMock } = vi.hoisted(() => ({ reducedMotionMock: vi.fn() }));
vi.mock("./reducedMotion", () => ({ prefersReducedMotion: reducedMotionMock }));

/** The `--blaze` custom property the card publishes, as a number. */
function blazeOf(container: HTMLElement): number {
  const card = container.querySelector<HTMLElement>("[data-testid='streak-card']");
  return Number(card?.style.getPropertyValue("--blaze"));
}

function flames(container: HTMLElement): HTMLElement | null {
  return container.querySelector("[data-testid='streak-flames']");
}

beforeEach(() => {
  reducedMotionMock.mockReset();
  reducedMotionMock.mockReturnValue(false);
});

describe("StreakCard", () => {
  it("renders nothing until the streak is worth surfacing", () => {
    for (const streak of [0, 1, 2]) {
      const { container } = render(StreakCard, { streak });
      expect(container.querySelector("[data-testid='streak-card']")).toBeNull();
    }
  });

  it("shows the count once the streak reaches the threshold", () => {
    render(StreakCard, { streak: 3 });
    expect(screen.getByText(/3-session streak/)).toBeTruthy();
    expect(screen.getByText(/Finish every set to keep it alive/)).toBeTruthy();
  });

  it("stays unlit through the run-up to six", () => {
    for (const streak of [3, 4, 5]) {
      const { container } = render(StreakCard, { streak });
      expect(flames(container)).toBeNull();
    }
  });

  it("catches fire at six", () => {
    const { container } = render(StreakCard, { streak: 6 });
    expect(flames(container)).not.toBeNull();
  });

  it("brightens across the run-up even before the flames", () => {
    const heats = [3, 4, 5].map((streak) => {
      const { container } = render(StreakCard, { streak });
      const card = container.querySelector<HTMLElement>("[data-testid='streak-card']");
      return Number(card?.style.getPropertyValue("--glow"));
    });
    expect(heats[1]).toBeGreaterThan(heats[0]);
    expect(heats[2]).toBeGreaterThan(heats[1]);
  });

  it("burns harder from six up to twelve, then holds", () => {
    const six = blazeOf(render(StreakCard, { streak: 6 }).container);
    const nine = blazeOf(render(StreakCard, { streak: 9 }).container);
    const twelve = blazeOf(render(StreakCard, { streak: 12 }).container);
    const forty = blazeOf(render(StreakCard, { streak: 40 }).container);

    expect(nine).toBeGreaterThan(six);
    expect(twelve).toBeGreaterThan(nine);
    expect(forty).toBe(twelve);
  });

  it("grows the flames as the blaze builds", () => {
    const heightAt = (streak: number) => {
      const { container } = render(StreakCard, { streak });
      return parseFloat(flames(container)!.style.height);
    };
    expect(heightAt(12)).toBeGreaterThan(heightAt(6));
  });

  it("keys its colour to the full ladder, not just the run-up ramp", () => {
    const toneAt = (streak: number) => {
      const { container } = render(StreakCard, { streak });
      const card = container.querySelector<HTMLElement>("[data-testid='streak-card']");
      return card!.style.getPropertyValue("--heat-color");
    };
    // Where the ramp goes is streakHeat's own business (and its own test). What
    // matters here is that the card reads the combined heat: driving the colour
    // off `glow` alone would leave every streak from 6 up the same shade.
    for (const streak of [3, 6, 12]) {
      expect(toneAt(streak)).toBe(heatColor(combinedHeat(streakHeat(streak))));
    }
    expect(new Set([toneAt(3), toneAt(6), toneAt(12)]).size).toBe(3);
  });

  it("keeps the fire but holds it still when the lifter asked for less motion", () => {
    reducedMotionMock.mockReturnValue(true);
    const { container } = render(StreakCard, { streak: 12 });

    // The reward still reads — colour, glow and flames are all there.
    const layer = flames(container);
    expect(layer).not.toBeNull();
    expect(
      container
        .querySelector<HTMLElement>("[data-testid='streak-card']")!
        .style.getPropertyValue("--heat-color"),
    ).toBeTruthy();

    // Nothing moves.
    expect(container.querySelectorAll("[class*='animate-']")).toHaveLength(0);
  });

  it("animates the flames when motion is allowed", () => {
    const { container } = render(StreakCard, { streak: 12 });
    expect(container.querySelector(".animate-flame-lick")).not.toBeNull();
    expect(container.querySelector(".animate-flame-flicker")).not.toBeNull();
    expect(container.querySelector(".animate-ember-pulse")).not.toBeNull();
  });

  it("stops moving when the lifter toggles reduced motion mid-session", async () => {
    // This card stays mounted for as long as the home screen is open, so reading
    // the preference once at init is not enough: someone who reaches for the
    // Control Centre toggle would keep watching the flames animate until they
    // navigated away and back. jsdom has no matchMedia, hence the stub.
    const listeners = new Set<() => void>();
    let reduced = false;
    vi.stubGlobal("matchMedia", (media: string) => ({
      media,
      get matches() {
        return reduced;
      },
      addEventListener: (_: string, fn: () => void) => void listeners.add(fn),
      removeEventListener: (_: string, fn: () => void) => void listeners.delete(fn),
    }));

    const { container } = render(StreakCard, { streak: 12 });
    expect(container.querySelector(".animate-flame-flicker")).not.toBeNull();

    reduced = true;
    listeners.forEach((fn) => fn());
    await tick();

    expect(container.querySelectorAll("[class*='animate-']")).toHaveLength(0);
    // The fire itself must survive: it is the reward, not the motion.
    expect(flames(container)).not.toBeNull();

    vi.unstubAllGlobals();
  });

  it("explains what counts when the help affordance is opened", async () => {
    render(StreakCard, { streak: 6 });

    // Nothing is said until asked — the card itself stays a reward, not a manual.
    expect(screen.queryByText(/Sessions, not days/)).toBeNull();

    // Found by its accessible name: an icon-only button with no label is the
    // failure mode this affordance is most likely to ship with.
    await fireEvent.click(screen.getByRole("button", { name: /what counts toward a streak/i }));

    // The two misreadings the copy exists to head off: that it counts days, and
    // that training off the program's schedule breaks it.
    expect(await screen.findByText(/Sessions, not days/)).toBeTruthy();
    expect(screen.getByText(/schedule doesn't come into it/)).toBeTruthy();
  });

  it("hides the decoration from assistive tech", () => {
    const { container } = render(StreakCard, { streak: 9 });
    expect(flames(container)!.getAttribute("aria-hidden")).toBe("true");
  });
});
