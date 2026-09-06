import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { auth, loadMe, signIn, signOut, signUp } from "./auth.svelte";
import type { User } from "./api";

const getMe = vi.hoisted(() => vi.fn());
const getRegistrationStatus = vi.hoisted(() => vi.fn());
const login = vi.hoisted(() => vi.fn());
const logout = vi.hoisted(() => vi.fn());
const register = vi.hoisted(() => vi.fn());

vi.mock("./api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./api")>()),
  getMe,
  getRegistrationStatus,
  login,
  logout,
  register,
}));

/** An answer in the shape the generated client produces. */
const answer = (status: number, data: unknown = undefined) => ({
  status,
  data,
  headers: new Headers(),
});

const ada: User = {
  id: 1,
  username: "ada",
  displayName: "Ada Lovelace",
  avatarColor: "",
  isAdmin: true,
  hasAvatar: false,
};

beforeEach(() => {
  auth.me = null;
  auth.loaded = false;
  auth.registrationOpen = false;
  getRegistrationStatus.mockResolvedValue(answer(200, { open: false }));
  logout.mockResolvedValue(answer(204));
});

afterEach(() => {
  vi.clearAllMocks();
});

describe("loadMe", () => {
  it("stores the user returned by the API", async () => {
    getMe.mockResolvedValue(answer(200, ada));
    await loadMe();
    expect(auth.me).toEqual(ada);
    expect(auth.loaded).toBe(true);
  });

  // Signed out is the expected state, not an error condition.
  it("treats a 401 as signed out rather than a failure", async () => {
    getMe.mockResolvedValue(answer(401, { code: "unauthenticated" }));
    await loadMe();
    expect(auth.me).toBeNull();
    expect(auth.loaded).toBe(true);
  });

  it("asks whether registration is open only when signed out", async () => {
    getMe.mockResolvedValue(answer(401, { code: "unauthenticated" }));
    getRegistrationStatus.mockResolvedValue(answer(200, { open: true }));
    await loadMe();
    expect(auth.registrationOpen).toBe(true);

    vi.clearAllMocks();
    getMe.mockResolvedValue(answer(200, ada));
    await loadMe();
    expect(getRegistrationStatus).not.toHaveBeenCalled();
    expect(auth.registrationOpen).toBe(false);
  });
});

describe("signIn", () => {
  it("stores the user and reports success", async () => {
    login.mockResolvedValue(answer(200, ada));
    const error = await signIn("ada", "hunter2hunter2", true);
    expect(error).toBeNull();
    expect(auth.me).toEqual(ada);
  });

  it("passes rememberMe through to the API", async () => {
    login.mockResolvedValue(answer(200, ada));
    await signIn("ada", "hunter2hunter2", true);
    expect(login).toHaveBeenCalledWith({
      username: "ada",
      password: "hunter2hunter2",
      rememberMe: true,
    });
  });

  it("surfaces the server's message on a rejected sign-in", async () => {
    login.mockResolvedValue(
      answer(401, { code: "unauthenticated", message: "invalid username or password" }),
    );
    expect(await signIn("ada", "wrong", false)).toBe("invalid username or password");
    expect(auth.me).toBeNull();
  });

  it("falls back to a readable message when the error has no shape", async () => {
    login.mockResolvedValue(answer(0));
    expect(await signIn("ada", "wrong", false)).toBe(
      "Incorrect username or password.",
    );
  });
});

describe("signUp", () => {
  it("stores the user and closes registration", async () => {
    auth.registrationOpen = true;
    register.mockResolvedValue(answer(201, ada));

    expect(await signUp("ada", "Ada Lovelace", "hunter2hunter2", true)).toBeNull();
    expect(auth.me).toEqual(ada);
    expect(auth.registrationOpen).toBe(false);
  });

  it("reports the server's reason for refusing", async () => {
    register.mockResolvedValue(
      answer(403, { code: "registration_closed", message: "registration is closed" }),
    );
    expect(await signUp("ada", "", "hunter2hunter2", false)).toBe(
      "registration is closed",
    );
  });
});

describe("signOut", () => {
  it("revokes the session server-side and clears the user", async () => {
    auth.me = { ...ada };
    getMe.mockResolvedValue(answer(401, { code: "unauthenticated" }));

    await signOut();

    expect(logout).toHaveBeenCalled();
    expect(auth.me).toBeNull();
  });

  // The user asked to be signed out; leaving their name in the header would
  // say otherwise. The cookie survives, so the server still has the last word.
  it("clears the user even when the request fails", async () => {
    auth.me = { ...ada };
    logout.mockRejectedValue(new Error("network down"));
    getMe.mockResolvedValue(answer(401, { code: "unauthenticated" }));

    await expect(signOut()).rejects.toThrow("network down");
    expect(auth.me).toBeNull();
  });
});
