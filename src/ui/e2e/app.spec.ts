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
  } = {},
) {
  const {
    isOver = false,
    finishedAt = null,
    actualReps = null,
    completed = false,
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
    sets: [1, 2].map((n) => ({
      id: n,
      exerciseId: 1,
      exerciseName: "Squat",
      setNumber: n,
      targetReps: 5,
      actualReps,
      weightLb: 80,
      completed,
      restSeconds: 180,
    })),
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

test("finishes a session with sets still unlogged, after confirming", async ({ page }) => {
  let finishCalls = 0;
  await page.route("**/api/v1/sessions/1/finish", (route) => {
    finishCalls += 1;
    route.fulfill({
      json: sessionDetail({ isOver: true, finishedAt: "2026-08-01T19:30:00Z" }),
    });
  });
  await page.route("**/api/v1/sessions/1", (route) =>
    route.fulfill({ json: sessionDetail() }),
  );
  await page.route("**/api/v1/exercises/1/history", (route) => route.fulfill({ json: [] }));

  await page.goto("/#/sessions/1");
  await expect(page.getByText("0 / 2 sets logged")).toBeVisible();

  // Both sets are unlogged, so finishing routes through the confirmation.
  await page.getByRole("button", { name: "Finish workout" }).click();
  await expect(
    page.getByRole("heading", { name: "Finish with sets unlogged?" }),
  ).toBeVisible();
  await expect(page.getByText("2 of 2 sets have no reps logged")).toBeVisible();

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
