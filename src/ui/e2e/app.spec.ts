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

test.beforeEach(async ({ page }) => {
  await page.route("**/api/v1/programs", (route) => route.fulfill({ json: programs }));
  await page.route("**/api/v1/sessions**", (route) => route.fulfill({ json: emptySessions }));
  await page.route("**/api/v1/programs/1", (route) => route.fulfill({ json: program1 }));
  await page.route("**/api/v1/programs/1/days/10/next-session", (route) =>
    route.fulfill({ json: nextSession }),
  );
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
