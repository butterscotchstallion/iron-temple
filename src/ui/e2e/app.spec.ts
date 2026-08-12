import { test, expect } from "@playwright/test";

// The UI is driven entirely by the API, so the e2e suite mocks those responses
// (via page.route) to stay self-contained — no running backend or seeded DB.

const programs = [
  { id: 1, name: "StrongLifts 5x5", description: "Squat, bench, row · A/B", progressionKind: "linear" },
  { id: 2, name: "StrongLifts 5x5 Lite", description: "Reduced volume", progressionKind: "linear" },
  { id: 3, name: "Advanced 3x5", description: "Graduation fork", progressionKind: "linear" },
];

const emptySessions = { items: [], total: 0, limit: 100, offset: 0 };

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
    exercises: [{ exerciseName: "Squat", sets: 5, reps: 5, weightLb: 100 }],
  };
}

test.beforeEach(async ({ page }) => {
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
