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
  it("renders all five tabs", () => {
    render(NavBar);
    for (const name of ["Workout", "Programs", "Library", "History", "Progress"]) {
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

  it("marks Library active on its own route", () => {
    setHash("#/library");
    render(NavBar);
    expect(link("Library")).toHaveAttribute("aria-current", "page");
    expect(link("Progress")).not.toHaveAttribute("aria-current");
  });

  // A lift's chart is reached from both the Library and Progress, and it is
  // Progress that owns it — so browsing the library and tapping through to a
  // chart moves the highlight, rather than leaving it on Library.
  it("leaves /exercises to the Progress tab, not Library", () => {
    setHash("#/exercises/3");
    render(NavBar);
    expect(link("Library")).not.toHaveAttribute("aria-current");
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
