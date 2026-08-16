import { test, expect } from "@playwright/test";

// The UI is driven entirely by the API, so the e2e suite mocks those responses
// (via page.route) to stay self-contained — no running backend or seeded DB.

const programs = [
  { id: 1, name: "StrongLifts 5x5", description: "Squat, bench, row · A/B", progressionKind: "linear" },
  { id: 2, name: "StrongLifts 5x5 Lite", description: "Reduced volume", progressionKind: "linear" },
  { id: 3, name: "Advanced 3x5", description: "Graduation fork", progressionKind: "linear" },
];

const emptySessions = { items: [], total: 0, limit: 100, offset: 0 };

// Every route below the header requires a session, so the app renders the
// sign-in form until /me resolves. Mock a signed-in user for these tests; the
// signed-out path has its own spec (auth.spec.ts).
const signedInUser = {
  id: 1,
  username: "ada",
  displayName: "Ada Lovelace",
  avatarColor: "",
  isAdmin: true,
  hasAvatar: false,
};

const program1 = {
  ...programs[0],
  days: [
    {
      id: 10,
      name: "Workout A",
      position: 1,
      exercises: [
        {
          id: 100,
          exerciseId: 1,
          exerciseName: "Squat",
          position: 1,
          sets: 5,
          reps: 5,
          startingWeightLb: 45,
          restSeconds: 180,
        },
      ],
    },
  ],
};

const nextSession = {
  programId: 1,
  programDayId: 10,
  programDayName: "Workout A",
  exercises: [
    // A fresh lift (no history) — carries a "start" status, so no badge shows.
    {
      exerciseId: 1,
      exerciseName: "Squat",
      sets: 5,
      reps: 5,
      weightLb: 45,
      restSeconds: 180,
      progression: {
        status: "start",
        failureCount: 0,
        failuresBeforeDeload: 3,
        previousWeightLb: 0,
      },
    },
    // A stalled lift that hit the deload threshold — drives the "Deload" badge.
    {
      exerciseId: 2,
      exerciseName: "Bench Press",
      sets: 5,
      reps: 5,
      weightLb: 65,
      restSeconds: 180,
      progression: {
        status: "deload",
        failureCount: 3,
        failuresBeforeDeload: 3,
        previousWeightLb: 72.5,
      },
    },
  ],
};

// A SessionSummary as returned by GET /sessions (History list).
function sessionSummary(id: number, programName: string) {
  return {
    id,
    programId: 1,
    programName,
    programDayId: 10,
    programDayName: "Workout A",
    performedOn: "2026-08-01",
    setCount: 5,
    completedSetCount: 5,
    isOver: true,
    exercises: [{ exerciseName: "Squat", sets: 5, reps: 5, weightLb: 100 }],
  };
}

// A Session as returned by GET /sessions/{id} (the ActiveSession screen).
// Two sets on the bar weight, so no warm-up ramp is rendered.
function sessionDetail(
  overrides: {
    isOver?: boolean;
    finishedAt?: string | null;
    actualReps?: number | null;
    completed?: boolean;
    // How many of the two sets carry reps. Defaults to all of them, so
    // actualReps/completed apply to the whole session as before. Set to 1 for a
    // part-done workout — the only shape in which the unlogged-sets confirm can
    // be reached, now that Finish is disabled until a rep is on the board.
    loggedSets?: number;
  } = {},
) {
  const {
    isOver = false,
    finishedAt = null,
    actualReps = null,
    completed = false,
    loggedSets,
  } = overrides;
  return {
    id: 1,
    programId: 1,
    programName: "StrongLifts 5x5",
    programDayId: 10,
    programDayName: "Workout A",
    performedOn: "2026-08-01",
    notes: "",
    createdAt: "2026-08-01T18:00:00Z",
    finishedAt,
    isOver,
    sets: [1, 2].map((n) => {
      const logged = loggedSets == null || n <= loggedSets;
      return {
        id: n,
        exerciseId: 1,
        exerciseName: "Squat",
        setNumber: n,
        targetReps: 5,
        actualReps: logged ? actualReps : null,
        weightLb: 80,
        completed: logged ? completed : false,
        restSeconds: 180,
      };
    }),
  };
}

test.beforeEach(async ({ page }) => {
  await page.route("**/api/v1/me", (route) => route.fulfill({ json: signedInUser }));
  await page.route("**/api/v1/programs", (route) => route.fulfill({ json: programs }));
  await page.route("**/api/v1/sessions**", (route) => route.fulfill({ json: emptySessions }));
  await page.route("**/api/v1/programs/1", (route) => route.fulfill({ json: program1 }));
  await page.route("**/api/v1/programs/1/days/10/next-session", (route) =>
    route.fulfill({ json: nextSession }),
  );
  // Default the Progress page to no exercises; individual tests override this.
  await page.route("**/api/v1/exercises", (route) => route.fulfill({ json: [] }));
});

test("renders the programs list", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("heading", { name: /iron temple/i })).toBeVisible();
  await expect(page.getByRole("heading", { name: "StrongLifts 5x5", exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Advanced 3x5", exact: true })).toBeVisible();
});

// With nothing saved and no history the picker is the right landing screen —
// that's the fallback the next two tests are the other half of.
test("lands on the saved program instead of the picker", async ({ page }) => {
  // Registered after the beforeEach handler, so this one wins.
  await page.route("**/api/v1/me", (route) =>
    route.fulfill({ json: { ...signedInUser, currentProgramId: 1 } }),
  );

  await page.goto("/");
  await expect(page.getByRole("heading", { name: "Workout A" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Choose a program" })).toBeHidden();
});

test("remembers the program it opens as the current one", async ({ page }) => {
  const patched: unknown[] = [];
  await page.route("**/api/v1/me", (route) => {
    if (route.request().method() !== "PATCH") {
      return route.fulfill({ json: signedInUser });
    }
    patched.push(route.request().postDataJSON());
    return route.fulfill({ json: { ...signedInUser, currentProgramId: 1 } });
  });

  await page.goto("/");
  await page.getByRole("heading", { name: "StrongLifts 5x5", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Workout A" })).toBeVisible();

  // Opening the program is the whole gesture — no Start, no button.
  await expect.poll(() => patched).toEqual([{ currentProgramId: 1 }]);
});

test("navigates into a program's detail", async ({ page }) => {
  await page.goto("/");
  await page.getByRole("heading", { name: "StrongLifts 5x5", exact: true }).click();

  await expect(page).toHaveURL(/#\/programs\/1$/);
  await expect(page.getByRole("heading", { name: "Workout A" })).toBeVisible();
  // exact: the program description also contains "Squat"; match only the set line.
  await expect(page.getByText("Squat", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Start" })).toBeVisible();

  // The stalled Bench Press surfaces its deload reasoning; the fresh Squat does not.
  await expect(page.getByText("Deload", { exact: true })).toBeVisible();
  await expect(page.getByText("stalled 3× at 72.5 → 65 lb")).toBeVisible();
});

test("navigates between the main tabs via the nav bar", async ({ page }) => {
  await page.goto("/");
  // With no history, Home falls back to the program picker.
  await expect(page.getByRole("heading", { name: "Choose a program" })).toBeVisible();

  await page.getByRole("link", { name: "History" }).click();
  await expect(page).toHaveURL(/#\/history$/);
  await expect(page.getByRole("heading", { name: "History", exact: true })).toBeVisible();

  await page.getByRole("link", { name: "Progress" }).click();
  await expect(page).toHaveURL(/#\/progress$/);
  await expect(page.getByRole("heading", { name: "Progress", exact: true })).toBeVisible();

  await page.getByRole("link", { name: "Programs" }).click();
  await expect(page).toHaveURL(/#\/programs$/);
  await expect(page.getByRole("heading", { name: "Choose a program" })).toBeVisible();
  // The active tab is exposed to assistive tech.
  await expect(page.getByRole("link", { name: "Programs" })).toHaveAttribute("aria-current", "page");
});

test("shows the empty history state", async ({ page }) => {
  await page.goto("/#/history");
  await expect(page.getByRole("heading", { name: "History", exact: true })).toBeVisible();
  await expect(
    page.getByText("No sessions logged yet. Start a workout to see it here."),
  ).toBeVisible();
});

test("lists past sessions and paginates with Load more", async ({ page }) => {
  // Two sessions across two pages (page size is 20; total 2 with one item per page).
  await page.route("**/api/v1/sessions**", (route) => {
    const offset = new URL(route.request().url()).searchParams.get("offset");
    const item =
      offset === "0" ? sessionSummary(1, "Alpha Program") : sessionSummary(2, "Beta Program");
    route.fulfill({
      json: { items: [item], total: 2, limit: 20, offset: Number(offset ?? 0) },
    });
  });

  await page.goto("/#/history");
  await expect(page.getByText("Alpha Program")).toBeVisible();
  await expect(page.getByText("Beta Program")).toHaveCount(0);

  await page.getByRole("button", { name: "Load more" }).click();
  await expect(page.getByText("Beta Program")).toBeVisible();
  // All loaded, so the button is gone.
  await expect(page.getByRole("button", { name: "Load more" })).toHaveCount(0);
});

test("shows each lift's top set on the progress page", async ({ page }) => {
  await page.route("**/api/v1/exercises", (route) =>
    route.fulfill({ json: [{ id: 1, name: "Squat" }, { id: 2, name: "Bench Press" }] }),
  );
  await page.route("**/api/v1/exercises/1/history", (route) =>
    route.fulfill({
      json: [
        { performedOn: "2026-07-01", weightLb: 95, reps: 5, completed: true },
        { performedOn: "2026-08-01", weightLb: 135, reps: 5, completed: true },
      ],
    }),
  );
  await page.route("**/api/v1/exercises/2/history", (route) => route.fulfill({ json: [] }));

  await page.goto("/#/progress");
  await expect(page.getByRole("heading", { name: "Progress", exact: true })).toBeVisible();
  await expect(page.getByText("Squat")).toBeVisible();
  // topSet picks the heaviest point across the history.
  await expect(page.getByText("135 lb")).toBeVisible();
  // A lift with no history reads as "No sessions yet".
  await expect(page.getByText("No sessions yet")).toBeVisible();
});

test("can't finish a session that hasn't started", async ({ page }) => {
  await page.route("**/api/v1/sessions/1", (route) =>
    route.fulfill({ json: sessionDetail() }),
  );
  await page.route("**/api/v1/exercises/1/history", (route) => route.fulfill({ json: [] }));

  await page.goto("/#/sessions/1");
  await expect(page.getByText("0 / 2 sets logged")).toBeVisible();

  // Nothing has been lifted, so there is no workout to close.
  await expect(page.getByRole("button", { name: "Finish workout" })).toBeDisabled();
});

test("finishes a part-done session with sets still unlogged, after confirming", async ({
  page,
}) => {
  let finishCalls = 0;
  await page.route("**/api/v1/sessions/1/finish", (route) => {
    finishCalls += 1;
    route.fulfill({
      json: sessionDetail({ isOver: true, finishedAt: "2026-08-01T19:30:00Z" }),
    });
  });
  // One set logged, one not: enough to have started, so Finish is live, but
  // still short of the whole workout.
  await page.route("**/api/v1/sessions/1", (route) =>
    route.fulfill({
      json: sessionDetail({ actualReps: 5, completed: true, loggedSets: 1 }),
    }),
  );
  await page.route("**/api/v1/exercises/1/history", (route) => route.fulfill({ json: [] }));

  await page.goto("/#/sessions/1");
  await expect(page.getByText("1 / 2 sets logged")).toBeVisible();

  // A set is still unlogged, so finishing routes through the confirmation.
  await page.getByRole("button", { name: "Finish workout" }).click();
  await expect(
    page.getByRole("heading", { name: "Finish with sets unlogged?" }),
  ).toBeVisible();
  await expect(page.getByText("1 of 2 sets have no reps logged")).toBeVisible();

  await page.getByRole("button", { name: "Finish anyway" }).click();
  await expect(page.getByRole("heading", { name: /Workout finished/ })).toBeVisible();
  expect(finishCalls).toBe(1);

  await page.getByRole("button", { name: "See history" }).click();
  await expect(page.getByRole("heading", { name: "History", exact: true })).toBeVisible();
});

test("finishes without confirming once every set is logged", async ({ page }) => {
  await page.route("**/api/v1/sessions/1/finish", (route) =>
    route.fulfill({
      json: sessionDetail({
        isOver: true,
        finishedAt: "2026-08-01T19:30:00Z",
        actualReps: 5,
        completed: true,
      }),
    }),
  );
  await page.route("**/api/v1/sessions/1", (route) =>
    route.fulfill({ json: sessionDetail({ actualReps: 5, completed: true }) }),
  );
  await page.route("**/api/v1/exercises/1/history", (route) => route.fulfill({ json: [] }));

  await page.goto("/#/sessions/1");
  await page.getByRole("button", { name: "Finish workout" }).click();

  // No confirmation step — straight to the celebration.
  await expect(page.getByRole("heading", { name: /Workout complete/ })).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "Finish with sets unlogged?" }),
  ).toHaveCount(0);
});

test("renders an already-finished session read-only", async ({ page }) => {
  await page.route("**/api/v1/sessions/1", (route) =>
    route.fulfill({
      json: sessionDetail({
        isOver: true,
        finishedAt: "2026-08-01T19:30:00Z",
        actualReps: 5,
        completed: true,
      }),
    }),
  );
  await page.route("**/api/v1/exercises/1/history", (route) => route.fulfill({ json: [] }));

  await page.goto("/#/sessions/1");
  await expect(page.getByText(/^Finished ·/)).toBeVisible();
  await expect(page.getByText("This workout is finished — sets are locked.")).toBeVisible();
  await expect(page.getByRole("button", { name: "Finish workout" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Set 1: 5 reps" })).toBeDisabled();
  await expect(page.getByRole("button", { name: "Increase weight by 5 lb" })).toBeDisabled();
});

test("marks a session aged past the 12h cutoff as closed automatically", async ({ page }) => {
  await page.route("**/api/v1/sessions/1", (route) =>
    route.fulfill({ json: sessionDetail({ isOver: true, actualReps: 5, completed: true }) }),
  );
  await page.route("**/api/v1/exercises/1/history", (route) => route.fulfill({ json: [] }));

  await page.goto("/#/sessions/1");
  // No finishedAt, so the banner explains it aged out rather than being finished.
  await expect(page.getByText("Closed automatically · 12h+ old")).toBeVisible();
  await expect(page.getByRole("button", { name: "Finish workout" })).toHaveCount(0);
});

// The changelog panel hanging off the header version. Its notes are compiled into
// the bundle (see e2e/build-with-changelog.sh), not fetched, so this is the one
// fixture the suite plants at build time rather than with page.route. Hover is the
// feature's whole point and jsdom cannot do it, so this is where it's really
// exercised — VersionChangelog.test.ts covers the rest.
async function openHeaderWithVersion(page: import("@playwright/test").Page) {
  await page.route("**/api/v1/health", (route) =>
    route.fulfill({ json: { status: "ok", version: "v9.9.9", environment: "production" } }),
  );
  await page.goto("/");

  const trigger = page.getByTestId("version");
  await expect(trigger).toContainText("iron-temple v9.9.9-production");

  // Inert text rather than a button means this build carries no notes, which is
  // what you get from a dev server reused via reuseExistingServer. Nothing to test.
  const interactive = await trigger.evaluate((el) => el.tagName === "BUTTON");
  test.skip(!interactive, "build carries no release notes — see e2e/build-with-changelog.sh");

  return trigger;
}

test("opens the changelog when the header version is hovered", async ({ page }) => {
  const trigger = await openHeaderWithVersion(page);
  await trigger.hover();

  const panel = page.getByTestId("changelog-panel");
  await expect(panel).toBeVisible();
  await expect(panel).toContainText("What's new in v9.9.9");
  await expect(panel).toContainText("show the release notes when you hover the header version");
  await expect(panel).toContainText("stop 500ing on a program with no days");

  // Escape closes it — LinkPreview's escape layer, no handler of our own.
  await page.keyboard.press("Escape");
  await expect(panel).not.toBeVisible();
});

// The path a tap takes. Firefox can't emulate touch (see the note in
// playwright.config.ts), so a click is as close as this suite gets — but it is the
// same handler, and without it the panel would be unreachable on an iPad.
test("toggles the changelog when the header version is clicked", async ({ page }) => {
  const trigger = await openHeaderWithVersion(page);
  const panel = page.getByTestId("changelog-panel");

  await trigger.click();
  await expect(panel).toBeVisible();

  await trigger.click();
  await expect(panel).not.toBeVisible();
});
