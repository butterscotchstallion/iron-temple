import { render, screen, waitFor, within } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import Profile from "./Profile.svelte";
import { auth } from "../lib/auth.svelte";
import { testUser } from "../lib/testFixtures";

// The profile route is a shell: it picks one of five sections from the URL and
// mounts that one alone. What is worth testing here is the picking — which
// section a slug resolves to, that only one is on screen at a time, and that
// the two ways of naming nothing (a bare /profile, a slug that isn't a section)
// both land somewhere real. The sections themselves are forms over the API and
// belong to the Playwright suite.

const replace = vi.hoisted(() => vi.fn());
vi.mock("svelte-spa-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("svelte-spa-router")>()),
  replace,
}));

beforeEach(() => {
  auth.me = testUser();
  auth.loaded = true;
  replace.mockClear();
});

afterEach(() => {
  auth.me = null;
  auth.loaded = false;
});

const heading = (name: string) => screen.queryByRole("heading", { name });

describe("Profile", () => {
  it("renders the section named by the URL, and only that one", () => {
    render(Profile, { params: { section: "password" } });

    expect(heading("Password")).toBeInTheDocument();
    expect(heading("Details")).not.toBeInTheDocument();
    expect(heading("Avatar")).not.toBeInTheDocument();
    expect(heading("Equipment")).not.toBeInTheDocument();
    expect(heading("Your data")).not.toBeInTheDocument();
  });

  it.each([
    ["avatar", "Avatar"],
    ["details", "Details"],
    ["password", "Password"],
    ["equipment", "Equipment"],
    ["data", "Your data"],
  ])("mounts the %s section", (slug, title) => {
    render(Profile, { params: { section: slug } });
    expect(heading(title)).toBeInTheDocument();
  });

  it("lists every section in the sub-nav, marking the current one", () => {
    render(Profile, { params: { section: "equipment" } });

    const nav = screen.getByRole("navigation", { name: "Profile sections" });
    for (const label of ["Avatar", "Details", "Password", "Equipment", "Your data"]) {
      expect(within(nav).getByRole("link", { name: label })).toBeInTheDocument();
    }
    expect(within(nav).getByRole("link", { name: "Equipment" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(within(nav).getByRole("link", { name: "Avatar" })).not.toHaveAttribute(
      "aria-current",
    );
  });

  it("redirects a bare /profile to the default section", async () => {
    render(Profile, { params: {} });

    await waitFor(() => expect(replace).toHaveBeenCalledWith("/profile/details"));
    // Nothing is drawn in the meantime — not a section, and not the nav that
    // would have to guess which pill is lit.
    expect(screen.queryByRole("navigation", { name: "Profile sections" })).toBeNull();
  });

  it("redirects a slug that names no section", async () => {
    render(Profile, { params: { section: "bench-press" } });

    await waitFor(() => expect(replace).toHaveBeenCalledWith("/profile/details"));
    expect(heading("Details")).not.toBeInTheDocument();
  });

  it("renders nothing but the title until /me settles", () => {
    auth.me = null;
    render(Profile, { params: { section: "details" } });

    expect(heading("Profile")).toBeInTheDocument();
    expect(heading("Details")).not.toBeInTheDocument();
    expect(replace).not.toHaveBeenCalled();
  });
});
