import {
  getMe,
  getRegistrationStatus,
  login as loginRequest,
  logout as logoutRequest,
  register as registerRequest,
  type User,
} from "./api";

// Shared authentication state. A module-level `$state` object rather than a
// store contract: Svelte 5 runes make the object itself reactive, so every
// component that reads `auth.me` re-renders on sign-in without subscriptions.
//
// The session lives in an HttpOnly cookie, which JavaScript cannot read — so
// this is a cache of what the server said, never the source of truth. Anything
// here can be stale; the server decides on every request.
export const auth = $state<{
  me: User | null;
  /** False until the first /me has settled. */
  loaded: boolean;
  /** Whether registration is still open (no account exists yet). */
  registrationOpen: boolean;
}>({
  me: null,
  loaded: false,
  registrationOpen: false,
});

/**
 * Resolves the current session. A 401 is the expected answer when signed out,
 * not a failure — it sets `me` to null and lets the app render the sign-in page.
 */
export async function loadMe(): Promise<void> {
  const { data, error } = await getMe();
  auth.me = error || !data ? null : data;

  // Only ask whether registration is open when nobody is signed in; for a
  // signed-in user the answer is always "no" and the request is wasted.
  if (!auth.me) {
    const status = await getRegistrationStatus();
    auth.registrationOpen = status.data?.open ?? false;
  } else {
    auth.registrationOpen = false;
  }

  // Set last: components key "signed out" off `loaded && !me`, so flipping this
  // before `me` is populated would flash the sign-in page on every load.
  auth.loaded = true;
}

/** Signs in. Returns an error message on failure, or null on success. */
export async function signIn(
  username: string,
  password: string,
  rememberMe: boolean,
): Promise<string | null> {
  const { data, error } = await loginRequest({
    body: { username, password, rememberMe },
  });
  if (error || !data) {
    return errorMessage(error, "Incorrect username or password.");
  }
  auth.me = data;
  auth.registrationOpen = false;
  return null;
}

/** Creates the first account and signs in. */
export async function signUp(
  username: string,
  displayName: string,
  password: string,
  rememberMe: boolean,
): Promise<string | null> {
  const { data, error } = await registerRequest({
    body: { username, displayName, password, rememberMe },
  });
  if (error || !data) {
    return errorMessage(error, "Couldn't create the account.");
  }
  auth.me = data;
  auth.registrationOpen = false;
  return null;
}

/**
 * Signs out. The local state is cleared even if the request fails: the user
 * asked to be signed out, and leaving their name in the header would suggest
 * otherwise. The cookie survives a failed call, so the next request 401s and
 * the server has the last word either way.
 */
export async function signOut(): Promise<void> {
  try {
    await logoutRequest();
  } finally {
    auth.me = null;
    window.location.hash = "#/";
    await loadMe();
  }
}

/** Applies a profile change returned by the server. */
export function setMe(user: User): void {
  auth.me = user;
}

// The API's error shape is { code, message }; fall back when a proxy or a
// network failure produces something else.
function errorMessage(error: unknown, fallback: string): string {
  if (error && typeof error === "object" && "message" in error) {
    const message = (error as { message?: unknown }).message;
    if (typeof message === "string" && message) return message;
  }
  return fallback;
}
