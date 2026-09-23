import { fireEvent, render, screen, waitFor, within } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import Profile from "./Profile.svelte";
import { auth } from "../lib/auth.svelte";
import { testUser } from "../lib/testFixtures";

// Two suites, because this route does two separable things.
//
// The first is the shell: the profile is five unrelated settings screens, one
// per URL, and it picks one from the slug and mounts that one alone. What is
// worth testing there is the picking — which slug resolves to what, that only
// one section is on screen at a time, and that the two ways of naming nothing
// (a bare /profile, a slug that isn't a section) both land somewhere real.
//
// The second is the equipment section, which is where a lifter tells the app
// what their gym actually is. It exists because the app had been guessing since
// 0013 and had never once asked — which is how somebody was offered a 35 lb
// plate they have never owned. Two things are pinned there and nowhere else:
// that the unconfirmed banner appears exactly when the gym is still the app's
// guess, and that saving sends the three stacks, which have no other home since
// there is no inventory of pins to derive them from.
//
// The remaining sections are forms over the API and belong to Playwright.

const replace = vi.hoisted(() => vi.fn());
vi.mock("svelte-spa-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("svelte-spa-router")>()),
  replace,
}));

const updateMe = vi.hoisted(() => vi.fn());
vi.mock("../lib/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../lib/api")>()),
  updateMe,
}));

const heading = (name: string) => screen.queryByRole("heading", { name });

/** Renders one section of the profile by the slug that owns it. */
function renderSection(slug: string) {
  return render(Profile, { params: { section: slug } });
}

describe("Profile", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    auth.me = testUser();
    auth.loaded = true;
  });

  afterEach(() => {
    auth.me = null;
    auth.loaded = false;
  });

  it("renders the section named by the URL, and only that one", () => {
    renderSection("password");

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
    renderSection(slug);
    expect(heading(title)).toBeInTheDocument();
  });

  it("lists every section in the sub-nav, marking the current one", () => {
    renderSection("equipment");

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
    renderSection("bench-press");

    await waitFor(() => expect(replace).toHaveBeenCalledWith("/profile/details"));
    expect(heading("Details")).not.toBeInTheDocument();
  });

  it("renders nothing but the title until /me settles", () => {
    auth.me = null;
    renderSection("details");

    expect(heading("Profile")).toBeInTheDocument();
    expect(heading("Details")).not.toBeInTheDocument();
    expect(replace).not.toHaveBeenCalled();
  });
});

describe("Profile equipment", () => {
  const BANNER = /never checked this with you/i;

  // The section is its own form with its own Save, and it is the only one
  // mounted here — so the button is unambiguous without scoping. Kept as a
  // helper anyway: it names what is being pressed.
  const equipmentSave = () => screen.getByRole("button", { name: "Save" });

  function signIn(over: Parameters<typeof testUser>[0] = {}) {
    auth.me = testUser({
      barWeightLb: 45,
      dumbbellStepLb: 5,
      machineStepLb: 5,
      cableStepLb: 5,
      bandStepLb: 5,
      plates: [{ plateLb: 45, pairs: 2 }],
      equipmentConfirmedAt: null,
      ...over,
    });
    auth.loaded = true;
  }

  beforeEach(() => {
    vi.clearAllMocks();
    signIn();
    updateMe.mockImplementation(async (body) => ({
      status: 200,
      data: { ...auth.me, ...body, equipmentConfirmedAt: "2026-09-23T00:00:00Z" },
    }));
  });

  afterEach(() => {
    auth.me = null;
    auth.loaded = false;
  });

  it("asks an unconfirmed lifter to check the gym we invented for them", () => {
    renderSection("equipment");
    expect(screen.getByText(BANNER)).toBeInTheDocument();
  });

  // The other side of the same branch. A lifter who has already said yes is not
  // asked again — a prompt that never goes away is one people learn to skip.
  it("says nothing to a lifter who has already confirmed", () => {
    signIn({ equipmentConfirmedAt: "2026-09-01T00:00:00Z" });
    renderSection("equipment");
    expect(screen.queryByText(BANNER)).not.toBeInTheDocument();
  });

  // Three fields and not one: a selectorized stack, a cable stack and a graded
  // band set are three different facts that only look alike at the default.
  it("offers a step for each kind of stack", () => {
    renderSection("equipment");
    expect(screen.getByLabelText("Machines")).toHaveValue(5);
    expect(screen.getByLabelText("Cables")).toHaveValue(5);
    expect(screen.getByLabelText("Bands")).toHaveValue(5);
  });

  it("sends the stacks when the gym is saved", async () => {
    renderSection("equipment");

    await fireEvent.input(screen.getByLabelText("Machines"), {
      target: { value: "15" },
    });
    await fireEvent.click(equipmentSave());

    await waitFor(() => expect(updateMe).toHaveBeenCalled());
    expect(updateMe).toHaveBeenCalledWith(
      expect.objectContaining({
        machineStepLb: 15,
        cableStepLb: 5,
        bandStepLb: 5,
      }),
    );
  });

  // Saving IS the confirmation — there is no separate "yes, this is right"
  // button, because a button like that is one more thing to not press. The
  // banner going away is how the lifter sees that it took.
  it("stops asking once the gym has been saved", async () => {
    renderSection("equipment");
    expect(screen.getByText(BANNER)).toBeInTheDocument();

    await fireEvent.click(equipmentSave());

    await waitFor(() => {
      expect(screen.queryByText(BANNER)).not.toBeInTheDocument();
    });
  });
});
