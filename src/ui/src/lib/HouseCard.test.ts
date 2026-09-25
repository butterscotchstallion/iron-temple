import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/svelte";
import HouseCard from "./HouseCard.svelte";
import { testHouse } from "./testFixtures";

// What a reader sees when they hover a sigil.
//
// Tested directly rather than through <Sigil>, because the hover itself is
// bits-ui's and simulating it in jsdom would assert on a floating-layer
// implementation rather than on what the card says. What this file owns is the
// content: the House's identity, its tagline, and a way to the page.

describe("the card", () => {
  it("names the House and shows its sigil", () => {
    render(HouseCard, { props: { house: testHouse({ name: "House Iron", sigil: "IRON" }) } });

    expect(screen.getByText("House Iron")).toBeInTheDocument();
    expect(screen.getByText(/IRON/)).toBeInTheDocument();
  });

  it("shows the tagline, which is the point of hovering", () => {
    render(HouseCard, {
      props: { house: testHouse({ tagline: "We lift at dawn" }) },
    });
    expect(screen.getByText("We lift at dawn")).toBeInTheDocument();
  });

  // Empty is ordinary — a House founded without one — and must not leave an empty
  // paragraph sitting in the card.
  it("omits the tagline when the House has none", () => {
    const { container } = render(HouseCard, { props: { house: testHouse({ tagline: "" }) } });
    expect(container.querySelector("p")).toBeNull();
  });

  it("counts a single member in the singular", () => {
    render(HouseCard, { props: { house: testHouse({ memberCount: 1 }) } });
    expect(screen.getByText(/1 member$/)).toBeInTheDocument();
  });

  it("counts several in the plural", () => {
    render(HouseCard, { props: { house: testHouse({ memberCount: 4 }) } });
    expect(screen.getByText(/4 members/)).toBeInTheDocument();
  });

  // The card carries the only link, because the sigil that opened it cannot be
  // one — see the note in Sigil.svelte on nested anchors.
  it("links to the House's page", () => {
    render(HouseCard, { props: { house: testHouse({ id: 7 }) } });
    expect(screen.getByRole("link", { name: /view house/i })).toHaveAttribute(
      "href",
      "#/houses/7",
    );
  });

  it("draws the icon a House chose", () => {
    const { container } = render(HouseCard, {
      props: { house: testHouse({ icon: "anvil", iconColor: "#b026ff" }) },
    });
    const icon = container.querySelector("svg");
    expect(icon).not.toBeNull();
    // Decorative: the name is rendered beside it, so it must not be announced too.
    expect(icon).toHaveAttribute("aria-hidden", "true");
  });

  // A House with no icon falls back to the sigil, which every House has, rather
  // than leaving a gap where one would be.
  it("draws no icon when the House has not chosen one", () => {
    const { container } = render(HouseCard, { props: { house: testHouse({ icon: "" }) } });
    expect(container.querySelector("svg")).toBeNull();
  });
});
