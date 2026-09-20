import { render, screen, waitFor, fireEvent } from "@testing-library/svelte";
import { beforeEach, describe, expect, it, vi } from "vitest";
import ForcePasswordChange from "./ForcePasswordChange.svelte";
import { auth } from "../lib/auth.svelte";
import { testUser } from "../lib/testFixtures";

// The screen an admin-created account sees, and the only screen it can see
// until it complies. That App.svelte renders this INSTEAD of the router is
// asserted in App.test.ts; this covers the form itself.

const changePassword = vi.hoisted(() => vi.fn());
const getMe = vi.hoisted(() => vi.fn());
const getRegistrationStatus = vi.hoisted(() => vi.fn());
vi.mock("../lib/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../lib/api")>()),
  changePassword,
  getMe,
  getRegistrationStatus,
}));

async function fillAndSubmit(temporary: string, chosen: string) {
  await fireEvent.input(screen.getByLabelText(/temporary password/i), {
    target: { value: temporary },
  });
  await fireEvent.input(screen.getByLabelText(/new password/i), {
    target: { value: chosen },
  });
  await fireEvent.click(screen.getByRole("button", { name: /set password/i }));
}

beforeEach(() => {
  vi.clearAllMocks();
  auth.me = testUser({ username: "grace", mustChangePassword: true });
  auth.loaded = true;
  getRegistrationStatus.mockResolvedValue({ status: 200, data: { open: false } });
});

describe("ForcePasswordChange", () => {
  it("explains why the account is being asked", () => {
    render(ForcePasswordChange);
    expect(screen.getByRole("heading", { name: /set your password/i })).toBeInTheDocument();
    expect(screen.getByText(/whoever set it up still knows the old one/i)).toBeInTheDocument();
  });

  it("changes the password and reloads the session", async () => {
    changePassword.mockResolvedValue({ status: 204, data: undefined });
    getMe.mockResolvedValue({
      status: 200,
      data: testUser({ username: "grace", mustChangePassword: false }),
    });

    render(ForcePasswordChange);
    await fillAndSubmit("temporary-one", "a-password-i-chose");

    await waitFor(() =>
      expect(changePassword).toHaveBeenCalledWith({
        currentPassword: "temporary-one",
        newPassword: "a-password-i-chose",
      }),
    );
    // The server cleared the flag; /me is what tells this app so, and clearing
    // it here is what swaps this screen for the router.
    await waitFor(() => expect(auth.me?.mustChangePassword).toBe(false));
  });

  // 401 is the temporary password being wrong, 400 the new one failing
  // validation. Both are the user's to fix, and the account stays gated.
  it("reports a rejected change and stays put", async () => {
    changePassword.mockResolvedValue({
      status: 401,
      data: { code: "unauthenticated", message: "current password is incorrect" },
    });

    render(ForcePasswordChange);
    await fillAndSubmit("wrong-one", "a-password-i-chose");

    expect(await screen.findByText(/couldn't change your password/i)).toBeInTheDocument();
    expect(getMe).not.toHaveBeenCalled();
    expect(auth.me?.mustChangePassword).toBe(true);
  });

  // A password manager needs a username field to file the new password under,
  // and this form has no visible one.
  it("carries a hidden username for the password manager", () => {
    const { container } = render(ForcePasswordChange);
    const username = container.querySelector('input[autocomplete="username"]');
    expect(username).toHaveValue("grace");
  });
});
