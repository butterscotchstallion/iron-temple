import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen } from "@testing-library/svelte";
import HeaderBar from "./HeaderBar.svelte";
import { auth } from "./auth.svelte";
import { levels, resetLevels } from "./levels.svelte";
import { version } from "./version.svelte";
import type { User } from "./api";
import { testLifterLevel, testUser } from "./testFixtures";

// The bar only renders the version store now — App.svelte owns the polling that
// fills it, and version.svelte.test.ts covers the fetching. So drive the store
// directly rather than stubbing /health at one remove.

const ada: User = testUser();

beforeEach(() => {
  version.running = "v1.2.3";
  version.latest = "v1.2.3";
  version.environment = "production";
  version.dismissed = "";
  auth.me = null;
  auth.loaded = true;
  auth.registrationOpen = false;
});

afterEach(() => {
  vi.clearAllMocks();
});

describe("HeaderBar", () => {
  // The version moved here from the footer; this is the test that it actually
  // arrived.
  it("shows the version reported by the API", () => {
    render(HeaderBar);
    expect(screen.getByTestId("version")).toHaveTextContent(
      "iron-temple v1.2.3-production",
    );
  });

  // How the label is assembled — the "-production" suffix and its absence — is
  // VersionChangelog's rule, and VersionChangelog.test.ts asserts both halves of
  // it against the component that owns it. The case above already shows the
  // environment reaching the label from this side, so a second copy here only
  // pins the same formatting in two places.

  // The label names the build you are looking at, not the newest one deployed —
  // that's the update prompt's job. Showing `latest` here would claim you were
  // running a bundle you haven't loaded yet.
  it("keeps naming the running build when a newer one is available", () => {
    version.latest = "v1.3.0";
    render(HeaderBar);
    expect(screen.getByTestId("version")).toHaveTextContent(
      "iron-temple v1.2.3-production",
    );
  });

  // The version is decoration; an unreachable /health must not blank the bar or
  // surface an error to the user.
  it("renders nothing for the version when /health hasn't answered", () => {
    version.running = "";
    version.environment = "";
    render(HeaderBar);
    expect(screen.getByTestId("version")).toHaveTextContent("");
    // The account side is unaffected.
    expect(screen.getByRole("button", { name: /sign in/i })).toBeInTheDocument();
  });

  it("offers a sign-in button when signed out", () => {
    render(HeaderBar);
    expect(screen.getByRole("button", { name: /sign in/i })).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /account menu/i }),
    ).not.toBeInTheDocument();
  });

  it("shows the user's name and avatar when signed in", () => {
    auth.me = ada;
    render(HeaderBar);
    expect(screen.getByRole("button", { name: /account menu/i })).toBeInTheDocument();
    expect(screen.getByText("Ada Lovelace")).toBeInTheDocument();
    // The initials chip stands in for the avatar she hasn't uploaded.
    expect(screen.getByText("AL")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /sign in/i })).not.toBeInTheDocument();
  });

  // Before /me settles we cannot know which side to show, and guessing means
  // flashing "Sign in" at someone who is already signed in.
  it("shows neither control until the session has been resolved", () => {
    auth.loaded = false;
    render(HeaderBar);
    expect(screen.queryByRole("button", { name: /sign in/i })).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /account menu/i }),
    ).not.toBeInTheDocument();
  });
});

// The experience line along the bottom edge of the bar.
//
// It is decoration with no text in it, which is exactly why it is worth testing: a
// line drawn from the wrong lifter's row, or drawn full for somebody who has earned
// nothing, is a claim nobody would notice was wrong. The levels module is seeded
// rather than mocked, as in LifterName.test.ts, because the per-lifter lookup is
// half of what can break.
describe("the experience line", () => {
  /** The bar's own scale factor, which is the whole of what it draws. */
  const scale = () => screen.getByTestId("level-line").style.transform;

  afterEach(resetLevels);

  it("scales to how far into their level the reader is", () => {
    auth.me = ada;
    levels.items = [
      testLifterLevel({ lifterId: 1, xpIntoLevel: 300, xpForNextLevel: 1200 }),
    ];
    levels.loaded = true;
    render(HeaderBar);

    expect(scale()).toBe("scaleX(0.25)");
  });

  // YOUR level, not the first row of a list that holds everybody on the install.
  it("reads the reader's own standing and nobody else's", () => {
    auth.me = ada;
    levels.items = [
      testLifterLevel({ lifterId: 2, xpIntoLevel: 900, xpForNextLevel: 1200 }),
      testLifterLevel({ lifterId: 1, xpIntoLevel: 600, xpForNextLevel: 1200 }),
    ];
    levels.loaded = true;
    render(HeaderBar);

    expect(scale()).toBe("scaleX(0.5)");
  });

  // A level just reached is an empty one, and the line says so by being absent
  // rather than by sitting at a sliver.
  it("draws nothing across for a level just started", () => {
    auth.me = ada;
    levels.items = [
      testLifterLevel({ lifterId: 1, level: 13, xpIntoLevel: 0, xpForNextLevel: 1200 }),
    ];
    levels.loaded = true;
    render(HeaderBar);

    expect(scale()).toBe("scaleX(0)");
  });

  // Nothing is fetched for this, so before the site-wide poll lands there is no
  // answer — and an empty track is a widget asking to be explained.
  it("draws no line until the levels have landed", () => {
    auth.me = ada;
    render(HeaderBar);

    expect(screen.queryByTestId("level-line")).toBeNull();
  });

  // Signed out there is no reader to have a level, and the list is not fetched at
  // all — the endpoint behind it answers 401.
  it("draws no line for a signed-out reader", () => {
    levels.items = [testLifterLevel({ lifterId: 1 })];
    levels.loaded = true;
    render(HeaderBar);

    expect(screen.queryByTestId("level-line")).toBeNull();
  });

  // Decoration, and it must not intercept a tap meant for the account button it
  // runs beneath. The level itself is announced beside the reader's name.
  it("is hidden from screen readers and takes no clicks", () => {
    auth.me = ada;
    levels.items = [testLifterLevel({ lifterId: 1 })];
    levels.loaded = true;
    render(HeaderBar);

    const line = screen.getByTestId("level-line");
    expect(line).toHaveAttribute("aria-hidden", "true");
    expect(line.className).toContain("pointer-events-none");
    // And it holds still for anybody who asked the OS for less motion.
    expect(line.className).toContain("motion-reduce:transition-none");
  });
});
