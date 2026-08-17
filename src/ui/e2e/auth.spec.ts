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
  await page.getByRole("button", { name: "Save", exact: true }).click();

  await expect(page.getByText("Saved.")).toBeVisible();
  expect(patched).toMatchObject({ displayName: "Ada B. Lovelace" });
  // The header reflects the change without a reload.
  await expect(page.getByRole("button", { name: /account menu/i })).toContainText(
    "Ada B. Lovelace",
  );
});
