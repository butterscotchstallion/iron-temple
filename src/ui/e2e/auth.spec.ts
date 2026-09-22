import { test, expect } from "@playwright/test";

// Sign-in, the header bar, and the account menu. Like app.spec.ts these mock
// the API via page.route, so the suite needs no backend. The account menu is
// exercised here rather than in Vitest: it is a bits-ui dropdown rendered into
// a portal and driven by pointer events, which a real browser handles honestly
// and jsdom approximates.

const ada = {
  id: 1,
  username: "ada",
  displayName: "Ada Lovelace",
  avatarColor: "",
  isAdmin: true,
  hasAvatar: false,
};

// `ada` deliberately carries no `mustChangePassword`. The field is required by
// the spec, but a client must read an absent one as false — that is what the
// API's own documentation promises — and leaving it off here is what keeps that
// tolerance tested rather than assumed.

/** The same account as `ada`, in the shape the admin roster returns. */
const adaAdminRow = {
  id: 1,
  username: "ada",
  displayName: "Ada Lovelace",
  avatarColor: "",
  isAdmin: true,
  mustChangePassword: false,
  createdAt: "2026-01-04T09:30:00Z",
};

const emptySessions = { items: [], total: 0, totalVolumeLb: 0, limit: 100, offset: 0 };

const health = { status: "ok", version: "v9.9.9", environment: "production" };

// A month with nothing logged. Every nullable field is explicitly null rather
// than absent, because that is what the API sends and the page branches on it.
const emptyRacked = {
  period: { kind: "month", start: "2026-03-01", end: "2026-03-31", label: "March 2026" },
  totals: { volumeLb: 0, sessions: 0, sets: 0, reps: 0 },
  change: null,
  comparison: { count: 0, label: "", unitLb: 0 },
  lifts: [],
  series: [],
  mostImproved: null,
  days: [],
  weekdays: [0, 0, 0, 0, 0, 0, 0],
  bestWeekday: -1,
  hours: Array.from({ length: 24 }, () => 0),
  hourLabel: "",
  streak: { longestWeeks: 0, currentWeeks: 0 },
  attendance: { basis: "none", expected: 0, actual: 0, rate: 0 },
  prs: [],
  milestones: [],
  heaviestSet: null,
  fastestSession: null,
  deloads: [],
  archetype: { name: "", description: "" },
};

/** Routes shared by both signed-in and signed-out cases. */
async function mockCommon(page: import("@playwright/test").Page) {
  await page.route("**/api/v1/health", (route) => route.fulfill({ json: health }));
  await page.route("**/api/v1/programs", (route) => route.fulfill({ json: [] }));
  await page.route("**/api/v1/sessions**", (route) =>
    route.fulfill({ json: emptySessions }),
  );
  await page.route("**/api/v1/exercises", (route) => route.fulfill({ json: [] }));
  await page.route("**/api/v1/racked**", (route) => route.fulfill({ json: emptyRacked }));

  // AFTER the `**/api/v1/sessions**` wildcard above, which otherwise claims these
  // and answers two array endpoints with a session-list object — see the longer
  // note in app.spec.ts. Playwright's last matching route wins, so the order here
  // is the fix.
  await page.route("**/api/v1/sessions/*/reactions**", (route) =>
    route.fulfill({ json: [] }),
  );
  await page.route("**/api/v1/sessions/*/comments**", (route) =>
    route.fulfill({ json: [] }),
  );

  // The feed card on Home, and the roster the account menu links to.
  await page.route("**/api/v1/feed**", (route) =>
    route.fulfill({ json: { items: [], limit: 20, offset: 0 } }),
  );
  await page.route("**/api/v1/lifters", (route) => route.fulfill({ json: [] }));

  // The two the Astroturfing screen reads on mount. Idle and switched off, which
  // is the state that screen's assertions here assume.
  //
  // Exact paths rather than one `**/admin/activity**` wildcard, so each answers
  // its own shape: a status object and a schedule object are different types, and
  // a wildcard would hand the panel the wrong one for whichever it claimed. The
  // schedule is the specific case that caught this — nothing was stubbing it, so
  // the request fell through to a proxy error in the log.
  await page.route("**/api/v1/admin/activity", (route) =>
    route.fulfill({
      json: {
        running: false,
        lifters: 0,
        tickSeconds: 0,
        actions: 0,
        maxLifters: 8,
        maxWeeks: 26,
        roster: ["mara.quinn", "dev.oyelaran"],
      },
    }),
  );
  await page.route("**/api/v1/admin/activity/schedule", (route) =>
    route.fulfill({ json: { enabled: false, lifters: 4 } }),
  );
}

async function mockSignedOut(
  page: import("@playwright/test").Page,
  { registrationOpen = false } = {},
) {
  await mockCommon(page);
  await page.route("**/api/v1/me", (route) =>
    route.fulfill({
      status: 401,
      json: { code: "unauthenticated", message: "authentication required" },
    }),
  );
  await page.route("**/api/v1/auth/registration-status", (route) =>
    route.fulfill({ json: { open: registrationOpen } }),
  );
}

async function mockSignedIn(page: import("@playwright/test").Page) {
  await mockCommon(page);
  await page.route("**/api/v1/me", (route) => route.fulfill({ json: ada }));
}

/** Signed in as an ordinary lifter — no install to administer. */
async function mockSignedInAsNonAdmin(page: import("@playwright/test").Page) {
  await mockCommon(page);
  await page.route("**/api/v1/me", (route) =>
    route.fulfill({ json: { ...ada, id: 2, username: "grace", isAdmin: false } }),
  );
}

test("shows the version in the header bar", async ({ page }) => {
  await mockSignedIn(page);
  await page.goto("/");

  // It moved here from the footer.
  await expect(page.getByTestId("version")).toHaveText("iron-temple v9.9.9-production");
});

test("signed out, the app offers sign in instead of the workout", async ({ page }) => {
  await mockSignedOut(page);
  await page.goto("/");

  await expect(page.getByRole("heading", { name: "Sign in" })).toBeVisible();
  // Two of them, by design: one in the header, one submitting the form.
  await expect(page.getByRole("button", { name: /sign in/i })).toHaveCount(2);
  // The nav tabs are hidden until there is someone to navigate as.
  await expect(page.getByRole("link", { name: "History" })).toBeHidden();
});

test("signed out, a protected route still lands on sign in", async ({ page }) => {
  await mockSignedOut(page);
  await page.goto("/#/history");

  await expect(page.getByRole("heading", { name: "Sign in" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "History", exact: true })).toBeHidden();
});

test("offers account creation while registration is open", async ({ page }) => {
  await mockSignedOut(page, { registrationOpen: true });
  await page.goto("/");

  await expect(page.getByRole("heading", { name: "Claim this install" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Create account" })).toBeVisible();
  // The display-name field only exists in the registration form.
  await expect(page.getByLabel("Display name")).toBeVisible();
});

test("signs in and reveals the app", async ({ page }) => {
  await mockSignedOut(page);
  let loginBody: unknown;
  await page.route("**/api/v1/auth/login", async (route) => {
    loginBody = route.request().postDataJSON();
    // From here on the user is signed in.
    await page.route("**/api/v1/me", (r) => r.fulfill({ json: ada }));
    await route.fulfill({ json: ada });
  });

  await page.goto("/");
  await page.getByLabel("Username").fill("ada");
  await page.getByLabel("Password").fill("hunter2hunter2");
  // Scoped to the form: the header carries a "Sign in" button too, so an
  // unscoped lookup is ambiguous under strict mode.
  await page.locator("form").getByRole("button", { name: "Sign in" }).click();

  await expect(page.getByRole("button", { name: /account menu/i })).toBeVisible();
  await expect(page.getByRole("link", { name: "History" })).toBeVisible();
  // Remember me defaults on, so a returning user isn't signed out overnight.
  expect(loginBody).toMatchObject({ username: "ada", rememberMe: true });
});

test("the account menu offers racked, profile and sign out", async ({ page }) => {
  await mockSignedIn(page);
  await page.goto("/");

  await page.getByRole("button", { name: /account menu/i }).click();
  await expect(page.getByRole("menuitem", { name: /^racked$/i })).toBeVisible();
  await expect(page.getByRole("menuitem", { name: /configure profile/i })).toBeVisible();
  await expect(page.getByRole("menuitem", { name: /sign out/i })).toBeVisible();
});

// Account management is the owner's, and the menu is where it is discovered.
// Asserted here rather than in Vitest for the reason at the top of this file:
// the menu is a portal-rendered bits-ui dropdown driven by pointer events.
test("the account menu offers account management to the owner", async ({ page }) => {
  await mockSignedIn(page);
  await page.goto("/");

  await page.getByRole("button", { name: /account menu/i }).click();
  await expect(page.getByRole("menuitem", { name: /manage accounts/i })).toBeVisible();
  // Generated activity sits in the same isAdmin block, and is the entry it would
  // be worst to leak: it names the install's own astroturfing.
  await expect(page.getByRole("menuitem", { name: /astroturfing/i })).toBeVisible();
});

test("the account menu hides account management from everyone else", async ({ page }) => {
  await mockSignedInAsNonAdmin(page);
  await page.goto("/");

  await page.getByRole("button", { name: /account menu/i }).click();
  // The menu is open — this is the entry missing, not the menu failing to open.
  await expect(page.getByRole("menuitem", { name: /sign out/i })).toBeVisible();
  await expect(page.getByRole("menuitem", { name: /manage accounts/i })).toHaveCount(0);
  await expect(page.getByRole("menuitem", { name: /astroturfing/i })).toHaveCount(0);
});

// Hiding the link is tidiness; the route condition is what makes typing the
// hash a dead end. (The API refuses /admin/* regardless — that is the boundary,
// and it is asserted in the Go suite.)
test("a non-admin typing the admin hash lands on the home screen", async ({ page }) => {
  await mockSignedInAsNonAdmin(page);
  await page.goto("/#/admin");

  // The condition fails, so the route falls through to the catch-all, which is
  // Home. No roster, and no wall of 403s either.
  // exact, like the assertion in the owner's test below — an inexact "Accounts"
  // also matches the roster card's "N accounts".
  await expect(page.getByRole("heading", { name: "Accounts", exact: true })).toHaveCount(0);
  await expect(page.getByRole("navigation")).toBeVisible();
});

// The same dead end for the generated-activity screen, which has its own route
// and therefore its own condition to get wrong. Worth its own test rather than
// trusting that /admin's condition covers both: they are two entries in the
// route table, and a copied one is exactly the kind that loses its guard.
test("a non-admin typing the astroturfing hash lands on the home screen", async ({
  page,
}) => {
  await mockSignedInAsNonAdmin(page);
  await page.goto("/#/astroturfing");

  await expect(page.getByRole("heading", { name: "Astroturfing", exact: true })).toHaveCount(
    0,
  );
  await expect(page.getByRole("navigation")).toBeVisible();
});

test("the owner can reach the astroturfing screen", async ({ page }) => {
  await mockSignedIn(page);
  await page.goto("/#/astroturfing");

  await expect(page.getByRole("heading", { name: "Astroturfing", exact: true })).toBeVisible();
  // The controls, not just the heading — the panel reads two endpoints on mount
  // and a failure there would leave a titled but empty screen.
  await expect(page.getByRole("button", { name: /generate history/i })).toBeVisible();
});

test("the owner can open the roster and add an account", async ({ page }) => {
  await mockSignedIn(page);
  await page.route("**/api/v1/admin/users", async (route) => {
    if (route.request().method() === "POST") {
      await route.fulfill({
        status: 201,
        json: {
          id: 2,
          username: "grace",
          displayName: "Grace Hopper",
          avatarColor: "",
          isAdmin: false,
          mustChangePassword: true,
          createdAt: "2026-03-17T18:00:00Z",
        },
      });
      return;
    }
    await route.fulfill({ json: [adaAdminRow] });
  });

  await page.goto("/");
  await page.getByRole("button", { name: /account menu/i }).click();
  await page.getByRole("menuitem", { name: /manage accounts/i }).click();

  await expect(page).toHaveURL(/#\/admin$/);
  // exact: the roster card's heading is "N accounts", which an inexact name
  // also matches — two headings, and a strict-mode violation rather than a
  // failure that says anything useful.
  await expect(page.getByRole("heading", { name: "Accounts", exact: true })).toBeVisible();
  await expect(page.getByText("1 account")).toBeVisible();

  // The username opens pre-filled with a suggested name, and the dice beside it
  // rolls another. Exercised here rather than only in jsdom because the button
  // sits inside the form: a stray submit would be invisible to a unit test that
  // never had a form to submit.
  const usernameField = page.getByLabel(/username/i);
  const firstSuggestion = await usernameField.inputValue();
  expect(firstSuggestion).not.toBe("");
  await page.getByRole("button", { name: /suggest another name/i }).click();
  await expect(usernameField).not.toHaveValue(firstSuggestion);
  await expect(page.getByText("Created grace.")).toHaveCount(0);

  await usernameField.fill("grace");
  await page.getByLabel(/display name/i).fill("Grace Hopper");

  // The password field arrives filled in with four random words, and the admin
  // is expected to use it as-is — so the test does too, rather than typing over
  // it. Asserted in a real browser as well as in jsdom because the generator
  // runs on crypto.getRandomValues, and "does that exist here" is exactly the
  // kind of question a jsdom shim answers too generously.
  const temporary = page.getByLabel(/temporary password/i);
  await expect(temporary).toHaveValue(/^[a-z]+-[a-z]+-[a-z]+-[a-z]+$/);
  const handedOut = await temporary.inputValue();

  await page.getByRole("button", { name: /create account/i }).click();

  await expect(page.getByText("Created grace.")).toBeVisible();
  await expect(page.getByText("2 accounts")).toBeVisible();
  // The new account has not been picked up yet, and the roster says so.
  await expect(page.getByText("Hasn't set a password")).toBeVisible();

  // A fresh passphrase for the next account, never the one just handed out.
  await expect(temporary).toHaveValue(/^[a-z]+-[a-z]+-[a-z]+-[a-z]+$/);
  await expect(temporary).not.toHaveValue(handedOut);
  // And a fresh name, which can't be the account that was just made.
  await expect(usernameField).not.toHaveValue("grace");
  expect(await usernameField.inputValue()).not.toBe("");
});

// An account created by the admin sees one screen and nothing else. The API
// enforces it (403 password_change_required); this is the client not painting a
// wall of failures behind a prompt.
test("an account owing a password change sees only that screen", async ({ page }) => {
  await mockCommon(page);
  await page.route("**/api/v1/me", (route) =>
    route.fulfill({
      json: { ...ada, id: 2, username: "grace", isAdmin: false, mustChangePassword: true },
    }),
  );

  await page.goto("/#/history");

  await expect(page.getByRole("heading", { name: /set your password/i })).toBeVisible();
  // No nav, and the route named in the hash did not mount.
  await expect(page.getByRole("navigation")).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "History" })).toHaveCount(0);
});

// Racked is reachable only from this menu — it has no nav-bar tab — so the
// menu entry working is the whole of its discoverability.
test("the account menu navigates to Racked", async ({ page }) => {
  await mockSignedIn(page);
  await page.goto("/");

  await page.getByRole("button", { name: /account menu/i }).click();
  await page.getByRole("menuitem", { name: /^racked$/i }).click();

  await expect(page).toHaveURL(/#\/racked$/);
  await expect(page.getByRole("heading", { name: "Racked" })).toBeVisible();
  // Nothing logged in the fixture month, so it says so rather than rendering
  // a page of zeroes.
  await expect(page.getByText(/Nothing logged in March 2026/)).toBeVisible();
});

test("the account menu navigates to the profile page", async ({ page }) => {
  await mockSignedIn(page);
  await page.goto("/");

  await page.getByRole("button", { name: /account menu/i }).click();
  await page.getByRole("menuitem", { name: /configure profile/i }).click();

  await expect(page).toHaveURL(/#\/profile$/);
  await expect(page.getByRole("heading", { name: "Profile" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Avatar" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Password" })).toBeVisible();
});

test("signing out returns to the sign-in form", async ({ page }) => {
  await mockSignedIn(page);
  let logoutCalled = false;
  await page.route("**/api/v1/auth/logout", async (route) => {
    logoutCalled = true;
    // The session is gone from here on.
    await page.route("**/api/v1/me", (r) =>
      r.fulfill({ status: 401, json: { code: "unauthenticated", message: "gone" } }),
    );
    await page.route("**/api/v1/auth/registration-status", (r) =>
      r.fulfill({ json: { open: false } }),
    );
    await route.fulfill({ status: 204, body: "" });
  });

  await page.goto("/");
  await page.getByRole("button", { name: /account menu/i }).click();
  await page.getByRole("menuitem", { name: /sign out/i }).click();

  await expect(page.getByRole("heading", { name: "Sign in" })).toBeVisible();
  expect(logoutCalled).toBe(true);
});

test("the profile page saves a display name", async ({ page }) => {
  await mockSignedIn(page);
  let patched: unknown;
  await page.route("**/api/v1/me", async (route) => {
    if (route.request().method() === "PATCH") {
      patched = route.request().postDataJSON();
      await route.fulfill({ json: { ...ada, displayName: "Ada B. Lovelace" } });
      return;
    }
    await route.fulfill({ json: ada });
  });

  await page.goto("/#/profile");
  await page.getByLabel("Display name").fill("Ada B. Lovelace");
  // Scoped to the Details form: the gym-setup card below it has a Save of its
  // own, so the label alone no longer names one button.
  await page
    .locator("form")
    .filter({ has: page.getByLabel("Display name") })
    .getByRole("button", { name: "Save", exact: true })
    .click();

  await expect(page.getByText("Saved.")).toBeVisible();
  expect(patched).toMatchObject({ displayName: "Ada B. Lovelace" });
  // The header reflects the change without a reload.
  await expect(page.getByRole("button", { name: /account menu/i })).toContainText(
    "Ada B. Lovelace",
  );
});
