import { describe, it, expect, afterEach } from "vitest";
import { flushSync } from "svelte";
import { render, screen } from "@testing-library/svelte";
import NavBar from "./NavBar.svelte";

// NavBar derives the active tab from the URL hash (svelte-spa-router is
// hash-based). Set the hash before rendering; reset it between tests.
function setHash(hash: string) {
  window.location.hash = hash;
}

afterEach(() => {
  window.location.hash = "";
});

const link = (name: string) => screen.getByRole("link", { name });

describe("NavBar", () => {
  it("renders all four tabs", () => {
    render(NavBar);
    for (const name of ["Workout", "Programs", "History", "Progress"]) {
      expect(link(name)).toBeInTheDocument();
    }
  });

  it("marks Workout active at the root", () => {
    setHash("");
    render(NavBar);
    expect(link("Workout")).toHaveAttribute("aria-current", "page");
    expect(link("Programs")).not.toHaveAttribute("aria-current");
  });

  it("marks the matching tab active for its own route", () => {
    setHash("#/history");
    render(NavBar);
    expect(link("History")).toHaveAttribute("aria-current", "page");
    expect(link("Workout")).not.toHaveAttribute("aria-current");
  });

  it("treats nested detail routes as owned by their tab", () => {
    setHash("#/programs/1");
    render(NavBar);
    expect(link("Programs")).toHaveAttribute("aria-current", "page");
  });

  it("keeps Workout active across the /sessions flow", () => {
    setHash("#/sessions/5");
    render(NavBar);
    expect(link("Workout")).toHaveAttribute("aria-current", "page");
  });

  it("updates the active tab when the hash changes", () => {
    setHash("");
    render(NavBar);
    expect(link("Workout")).toHaveAttribute("aria-current", "page");

    setHash("#/progress");
    window.dispatchEvent(new Event("hashchange"));
    flushSync();

    expect(link("Progress")).toHaveAttribute("aria-current", "page");
    expect(link("Workout")).not.toHaveAttribute("aria-current");
  });
});
