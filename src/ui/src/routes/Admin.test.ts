import { render, screen, waitFor, fireEvent } from "@testing-library/svelte";
import { beforeEach, describe, expect, it, vi } from "vitest";
import Admin from "./Admin.svelte";
import type { AdminUser } from "../lib/api";
import { auth } from "../lib/auth.svelte";
import { testUser } from "../lib/testFixtures";

// A render test for a route, which the suite otherwise leaves to Playwright —
// for the same reason Racked.test.ts is one. This page branches on per-row
// state (who owns the install, who has not picked up their account yet) and
// turns a 409 into something the admin can act on, and each of those is a
// chance to read a property off nothing. The account MENU entry that leads here
// is not covered from jsdom: it is a bits-ui dropdown in a portal driven by
// pointer events, so it lives in e2e/auth.spec.ts with the rest of that menu.

const listUsers = vi.hoisted(() => vi.fn());
const createUser = vi.hoisted(() => vi.fn());
vi.mock("../lib/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../lib/api")>()),
  listUsers,
  createUser,
}));

function adminUser(overrides: Partial<AdminUser> = {}): AdminUser {
  return {
    id: 1,
    username: "ada",
    displayName: "Ada Lovelace",
    avatarColor: "",
    isAdmin: true,
    mustChangePassword: false,
    createdAt: "2026-01-04T09:30:00Z",
    ...overrides,
  };
}

const owner = adminUser();
const member = adminUser({
  id: 2,
  username: "grace",
  displayName: "Grace Hopper",
  isAdmin: false,
  mustChangePassword: true,
  createdAt: "2026-03-17T18:00:00Z",
});

beforeEach(() => {
  vi.clearAllMocks();
  listUsers.mockResolvedValue({ status: 200, data: [owner, member] });
  auth.me = testUser();
  auth.loaded = true;
});

describe("Admin", () => {
  it("lists every account, oldest first", async () => {
    render(Admin);

    await screen.findByText("Ada Lovelace");
    expect(screen.getByText("Grace Hopper")).toBeInTheDocument();
    expect(screen.getByText("2 accounts")).toBeInTheDocument();
    // The instant is rendered as the day it happened, not as an ISO string.
    expect(screen.getByText("March 17 2026")).toBeInTheDocument();
  });

  it("marks the owner and the caller", async () => {
    render(Admin);

    await screen.findByText("Ada Lovelace");
    // Exactly one owner per install, enforced by the schema.
    expect(screen.getAllByText("Owner")).toHaveLength(1);
    expect(screen.getByText("(you)")).toBeInTheDocument();
  });

  // The temporary password is still live and still known to whoever typed it,
  // so the roster has to say which accounts have not been picked up.
  it("flags accounts that still owe a password change", async () => {
    render(Admin);

    await screen.findByText("Grace Hopper");
    expect(screen.getAllByText("Hasn't set a password")).toHaveLength(1);
  });

  it("creates an account and appends it to the roster", async () => {
    const created = adminUser({
      id: 3,
      username: "hopper",
      displayName: "hopper",
      isAdmin: false,
      mustChangePassword: true,
      createdAt: "2026-03-18T10:00:00Z",
    });
    createUser.mockResolvedValue({ status: 201, data: created });

    render(Admin);
    await screen.findByText("Ada Lovelace");

    await fireEvent.input(screen.getByLabelText(/username/i), {
      target: { value: "hopper" },
    });
    await fireEvent.input(screen.getByLabelText(/temporary password/i), {
      target: { value: "a-long-enough-password" },
    });
    await fireEvent.click(screen.getByRole("button", { name: /create account/i }));

    await waitFor(() =>
      expect(createUser).toHaveBeenCalledWith({
        username: "hopper",
        displayName: "",
        password: "a-long-enough-password",
      }),
    );
    // Appended rather than refetched: one insertion, no second round trip.
    expect(listUsers).toHaveBeenCalledTimes(1);
    expect(await screen.findByText("3 accounts")).toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent("Created hopper.");
  });

  // The server names the reason — a taken username, a password too short — and
  // that is more use to the admin than a generic failure.
  it("surfaces the server's message when the username is taken", async () => {
    createUser.mockResolvedValue({
      status: 409,
      data: { code: "username_taken", message: "that username is already taken" },
    });

    render(Admin);
    await screen.findByText("Ada Lovelace");

    await fireEvent.input(screen.getByLabelText(/username/i), {
      target: { value: "ada" },
    });
    await fireEvent.input(screen.getByLabelText(/temporary password/i), {
      target: { value: "a-long-enough-password" },
    });
    await fireEvent.click(screen.getByRole("button", { name: /create account/i }));

    expect(await screen.findByText(/that username is already taken/i)).toBeInTheDocument();
    // The roster is untouched, and the form keeps what was typed so it can be
    // corrected rather than retyped.
    expect(screen.getByText("2 accounts")).toBeInTheDocument();
    expect(screen.getByLabelText(/username/i)).toHaveValue("ada");
  });

  it("says so when the roster cannot be loaded", async () => {
    listUsers.mockResolvedValue({ status: 500, data: { code: "internal", message: "" } });

    render(Admin);

    expect(await screen.findByText(/couldn't load the accounts/i)).toBeInTheDocument();
  });
});
