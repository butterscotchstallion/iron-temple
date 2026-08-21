import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen } from "@testing-library/svelte";
import HeaderBar from "./HeaderBar.svelte";
import { auth } from "./auth.svelte";
import { version } from "./version.svelte";
import type { User } from "./api";

// The bar only renders the version store now — App.svelte owns the polling that
// fills it, and version.svelte.test.ts covers the fetching. So drive the store
// directly rather than stubbing /health at one remove.

const ada: User = {
  id: 1,
  username: "ada",
  displayName: "Ada Lovelace",
  avatarColor: "",
  isAdmin: true,
  hasAvatar: false,
};

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

  it("omits the environment suffix when the API doesn't report one", () => {
    version.environment = "";
    render(HeaderBar);
    expect(screen.getByTestId("version")).toHaveTextContent("iron-temple v1.2.3");
  });

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
