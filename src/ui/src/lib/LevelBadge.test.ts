import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/svelte";
import LevelBadge from "./LevelBadge.svelte";
import { testLifterLevel } from "./testFixtures";

// The number a lifter wears.
//
// The thing worth pinning is that the digit is never the whole of what is
// announced: beside a name, a bare "12" reads as part of the name.

describe("LevelBadge", () => {
  it("draws the number and nothing else", () => {
    render(LevelBadge, { props: { level: testLifterLevel({ level: 12 }) } });

    expect(screen.getByTestId("level-badge")).toHaveTextContent("12");
  });

  // No "Lv" in the badge itself — the word lives in the label, because beside a
  // House sigil two word-shaped tags read as one thing with a gap in it.
  it("says what the number means to a screen reader", () => {
    render(LevelBadge, { props: { level: testLifterLevel({ level: 12 }) } });

    expect(screen.getByText("Level 12")).toHaveClass("sr-only");
  });

  it("carries the same label as a tooltip for a mouse", () => {
    render(LevelBadge, { props: { level: testLifterLevel({ level: 12 }) } });

    expect(screen.getByTestId("level-badge")).toHaveAttribute(
      "title",
      "Level 12",
    );
  });

  // On the card variant the browser's own tooltip would fire a second after the
  // card and sit on top of it saying less.
  it("drops the tooltip when the card is what opens", () => {
    render(LevelBadge, {
      props: { level: testLifterLevel({ level: 12 }), card: true },
    });

    expect(screen.getByTestId("level-badge")).not.toHaveAttribute("title");
  });

  // The header suppresses the card while its account menu is up, because the two
  // would otherwise render on top of each other. The number stays either way —
  // it is the badge, not the decoration.
  it("still draws the number while the card is suppressed", () => {
    render(LevelBadge, {
      props: {
        level: testLifterLevel({ level: 12 }),
        card: true,
        suppressed: true,
      },
    });

    const badge = screen.getByTestId("level-badge");
    expect(badge).toHaveTextContent("12");
    expect(badge).not.toHaveAttribute("title");
  });

  // Level 1 is an ordinary state, not an empty one: it is what every account that
  // has never trained is, and it is drawn rather than hidden.
  it("draws level 1", () => {
    render(LevelBadge, {
      props: { level: testLifterLevel({ level: 1, xp: 0, xpIntoLevel: 0 }) },
    });

    expect(screen.getByTestId("level-badge")).toHaveTextContent("1");
    expect(screen.getByText("Level 1")).toHaveClass("sr-only");
  });
});
