import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/svelte";
import LoadingFixture from "./LoadingFixture.svelte";

describe("Loading", () => {
  it("announces what is being fetched", () => {
    // The bars are aria-hidden, so without this a loading screen is an empty
    // document to anybody listening to it rather than looking at it.
    render(LoadingFixture, { label: "Loading your history" });
    expect(screen.getByRole("status")).toHaveTextContent("Loading your history…");
  });

  it("marks the region busy", () => {
    render(LoadingFixture);
    expect(screen.getByRole("status")).toHaveAttribute("aria-busy", "true");
  });

  it("lays its children out the way the loaded branch will", () => {
    // The placeholder has to sit in the same box its content will, or holding
    // each bar's height individually buys nothing.
    render(LoadingFixture, { class: "grid gap-4 sm:grid-cols-3" });
    expect(screen.getByRole("status").className).toContain("sm:grid-cols-3");
  });

  it("renders the skeletons it was given", () => {
    const { container } = render(LoadingFixture);
    expect(container.querySelectorAll("[aria-hidden='true']")).toHaveLength(1);
  });
});
