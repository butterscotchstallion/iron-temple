import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/svelte";
import UserMenu from "./UserMenu.svelte";
import { auth } from "./auth.svelte";
import type { User } from "./api";

const logout = vi.hoisted(() => vi.fn());
const getMe = vi.hoisted(() => vi.fn());
const getRegistrationStatus = vi.hoisted(() => vi.fn());
const push = vi.hoisted(() => vi.fn());

vi.mock("./api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./api")>()),
  logout,
  getMe,
  getRegistrationStatus,
}));
vi.mock("svelte-spa-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("svelte-spa-router")>()),
  push,
}));

const ada: User = {
  id: 1,
  username: "ada",
  displayName: "Ada Lovelace",
  avatarColor: "",
  isAdmin: true,
  hasAvatar: false,
};

beforeEach(() => {
  logout.mockResolvedValue({ data: undefined, error: undefined });
  // Signing out reloads the session, which then 401s.
  getMe.mockResolvedValue({ data: undefined, error: { code: "unauthenticated" } });
  getRegistrationStatus.mockResolvedValue({ data: { open: false } });
  auth.me = { ...ada };
  auth.loaded = true;
});

afterEach(() => {
  vi.clearAllMocks();
});

describe("UserMenu", () => {
  it("shows the signed-in user's name and avatar", () => {
    render(UserMenu);
    const trigger = screen.getByRole("button", { name: /account menu/i });
    expect(trigger).toBeInTheDocument();
    expect(screen.getByText("Ada Lovelace")).toBeInTheDocument();
    // No uploaded image, so the initials chip stands in.
    expect(screen.getByText("AL")).toBeInTheDocument();
  });

  it("falls back to the username when there is no display name", () => {
    auth.me = { ...ada, displayName: "" };
    render(UserMenu);
    expect(screen.getByText("ada")).toBeInTheDocument();
  });

  it("shows a sign-in button when signed out", () => {
    auth.me = null;
    render(UserMenu);
    expect(screen.getByRole("button", { name: /sign in/i })).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /account menu/i }),
    ).not.toBeInTheDocument();
  });

  it("routes to the sign-in page", async () => {
    auth.me = null;
    render(UserMenu);
    await fireEvent.click(screen.getByRole("button", { name: /sign in/i }));
    await waitFor(() => expect(push).toHaveBeenCalledWith("/signin"));
  });

  // Nothing is rendered until /me has settled, or every page load flashes a
  // "Sign in" link at a user who is already signed in.
  it("renders nothing before the session is resolved", () => {
    auth.me = null;
    auth.loaded = false;
    const { container } = render(UserMenu);
    expect(container.textContent?.trim()).toBe("");
  });

  // The menu itself is a bits-ui dropdown rendered into a portal and driven by
  // pointer events; opening it is exercised in the Playwright suite against a
  // real browser, where those events behave as they do for a user. See
  // e2e/auth.spec.ts.
});
