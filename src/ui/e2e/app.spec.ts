import { test, expect } from "@playwright/test";

// The UI is driven entirely by the API, so the e2e suite mocks those responses
// (via page.route) to stay self-contained — no running backend or seeded DB.

const programs = [
  { id: 1, name: "StrongLifts 5x5", description: "Squat, bench, row · A/B", progressionKind: "linear" },
  { id: 2, name: "StrongLifts 5x5 Lite", description: "Reduced volume", progressionKind: "linear" },
  { id: 3, name: "Advanced 3x5", description: "Graduation fork", progressionKind: "linear" },
];

const emptySessions = { items: [], total: 0, totalVolumeLb: 0, limit: 100, offset: 0 };

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
      // The lifter's own additions to this day. Empty by default; the assistance
      // tests below override the route with a day that has some.
      assistance: [],
    },
  ],
};

// The exercise library, as GET /exercises returns it.
const libraryExercises = [
  { id: 1, name: "Squat", muscleGroup: "legs", equipment: "barbell", isAccessory: false, isCustom: false },
  { id: 2, name: "Bench Press", muscleGroup: "chest", equipment: "barbell", isAccessory: false, isCustom: false },
  { id: 3, name: "Dip", muscleGroup: "chest", equipment: "bodyweight", isAccessory: true, isCustom: false },
  { id: 4, name: "Barbell Curl", muscleGroup: "arms", equipment: "barbell", isAccessory: true, isCustom: false },
  { id: 5, name: "Plank", muscleGroup: "core", equipment: "bodyweight", isAccessory: true, isCustom: false },
];

const nextSession = {
  programId: 1,
  programDayId: 10,
  programDayName: "Workout A",
  exercises: [
    // A fresh lift (no history) — carries a "start" status, so no badge shows.
    {
      exerciseId: 1,
      exerciseName: "Squat",
      kind: "main",
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
      kind: "main",
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
    // 5 sets × 5 reps × 100 lb, consistent with the exercises line below.
    volumeLb: 2500,
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
        kind: "main",
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
  await page.route("**/api/v1/exercises**", (route) => route.fulfill({ json: [] }));
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

  await page.getByRole("link", { name: "Library" }).click();
  await expect(page).toHaveURL(/#\/library$/);
  await expect(page.getByRole("heading", { name: "Exercise library" })).toBeVisible();

  await page.getByRole("link", { name: "Programs" }).click();
  await expect(page).toHaveURL(/#\/programs$/);
  await expect(page.getByRole("heading", { name: "Choose a program" })).toBeVisible();
  // The active tab is exposed to assistive tech.
  await expect(page.getByRole("link", { name: "Programs" })).toHaveAttribute("aria-current", "page");
});

test("browses and searches the exercise library", async ({ page }) => {
  await page.route("**/api/v1/exercises**", (route) =>
    route.fulfill({ json: libraryExercises }),
  );

  await page.goto("/#/library");
  await expect(page.getByRole("heading", { name: "Exercise library" })).toBeVisible();

  // Grouped by muscle group, in the library's reading order.
  await expect(page.getByRole("heading", { name: "Chest" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Arms" })).toBeVisible();
  await expect(page.getByRole("link", { name: /Dip/ })).toBeVisible();

  // The lifts a program prescribes are marked as such.
  await expect(page.getByText("Barbell · Program lift").first()).toBeVisible();

  // Searching narrows to matches and drops the groups it emptied.
  await page.getByPlaceholder("Search exercises…").fill("curl");
  await expect(page.getByRole("link", { name: /Barbell Curl/ })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Chest" })).toBeHidden();

  // A search that matches nothing says so rather than showing empty headings.
  await page.getByPlaceholder("Search exercises…").fill("zercher");
  await expect(page.getByText("No exercises match that search.")).toBeVisible();
});

test("adds assistance to a program day", async ({ page }) => {
  await page.route("**/api/v1/exercises**", (route) =>
    route.fulfill({ json: libraryExercises }),
  );

  // Once the POST lands, the program and its preview come back carrying the
  // new entry — ProgramDetail reloads rather than guessing the weight itself.
  let added = false;
  const posted: unknown[] = [];
  await page.route("**/api/v1/programs/1/days/10/assistance", (route) => {
    posted.push(route.request().postDataJSON());
    added = true;
    return route.fulfill({
      status: 201,
      json: { id: 7, exerciseId: 3, exerciseName: "Dip", position: 1, sets: 3, reps: 8, weightLb: 0 },
    });
  });
  await page.route("**/api/v1/programs/1", (route) =>
    route.fulfill({
      json: added
        ? {
            ...program1,
            days: [
              {
                ...program1.days[0],
                assistance: [
                  { id: 7, exerciseId: 3, exerciseName: "Dip", position: 1, sets: 3, reps: 8, weightLb: 0 },
                ],
              },
            ],
          }
        : program1,
    }),
  );

  await page.goto("/#/programs/1");
  await expect(page.getByRole("heading", { name: "Workout A" })).toBeVisible();
  await expect(page.getByText("Nothing yet — add accessory work to finish this day off.")).toBeVisible();

  await page.getByRole("button", { name: "Add assistance" }).click();
  await page.getByRole("button", { name: /Dip/ }).click();
  await page.getByLabel("Reps").fill("8");
  await page.getByRole("button", { name: "Add to this day" }).click();

  await expect.poll(() => posted).toEqual([
    { exerciseId: 3, sets: 3, reps: 8, weightLb: 0 },
  ]);

  // It shows up under the day, below the program's own lifts, and reads as
  // bodyweight rather than "0 lb".
  await expect(page.getByRole("link", { name: "Dip" })).toBeVisible();
  await expect(page.getByText("3×8 · bodyweight")).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Remove Dip from Workout A" }),
  ).toBeVisible();
});

test("separates assistance from the program's work in a session", async ({ page }) => {
  const session = sessionDetail();
  await page.route("**/api/v1/sessions/1", (route) =>
    route.fulfill({
      json: {
        ...session,
        sets: [
          ...session.sets,
          {
            id: 3,
            exerciseId: 3,
            exerciseName: "Dip",
            kind: "assistance",
            setNumber: 1,
            targetReps: 8,
            actualReps: null,
            weightLb: 0,
            completed: false,
            restSeconds: 180,
          },
        ],
      },
    }),
  );
  await page.route("**/api/v1/exercises/*/history", (route) => route.fulfill({ json: [] }));

  await page.goto("/#/sessions/1");
  await expect(page.getByRole("heading", { name: "Squat" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Dip" })).toBeVisible();
  // One rule between the barbell work and what comes after it.
  await expect(page.getByText("Assistance", { exact: true })).toBeVisible();
});

// The box opens holding last time's number so a weigh-in is a nudge rather than
// a fresh entry — but showing it and recording it are different things, and only
// an edit crosses that line.
test("carries the last weigh-in into a session and records an edit", async ({ page }) => {
  const patched: unknown[] = [];
  const session = {
    ...sessionDetail(),
    bodyweightLb: null,
    lastWeighIn: { weightLb: 184.5, performedOn: "2026-07-31" },
  };
  await page.route("**/api/v1/sessions/1", (route) => {
    if (route.request().method() !== "PATCH") {
      return route.fulfill({ json: session });
    }
    patched.push(route.request().postDataJSON());
    return route.fulfill({ json: { ...session, bodyweightLb: 183 } });
  });
  await page.route("**/api/v1/exercises/*/history", (route) => route.fulfill({ json: [] }));

  await page.goto("/#/sessions/1");

  // Pre-filled, and captioned as last week's number rather than this session's.
  const box = page.getByLabel("Bodyweight");
  await expect(box).toHaveValue("184.5");
  await expect(page.getByText("Carried from July 31 2026")).toBeVisible();

  // Editing it is what writes it — and nothing was written before that.
  expect(patched).toEqual([]);
  await box.fill("183");
  await box.blur();
  await expect.poll(() => patched).toEqual([{ bodyweightLb: 183 }]);
  await expect(page.getByText("Logged for this session")).toBeVisible();
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
      json: {
        items: [item],
        total: 2,
        // Spans both sessions, not just the page — so it must not change when
        // the second page loads.
        totalVolumeLb: 5000,
        limit: 20,
        offset: Number(offset ?? 0),
      },
    });
  });

  await page.goto("/#/history");
  await expect(page.getByText("Alpha Program")).toBeVisible();
  await expect(page.getByText("Beta Program")).toHaveCount(0);
  await expect(page.getByText("5,000 lb lifted across 2 sessions")).toBeVisible();
  // Each row carries its own session's volume.
  await expect(page.getByText("2,500 lb lifted")).toBeVisible();

  await page.getByRole("button", { name: "Load more" }).click();
  await expect(page.getByText("Beta Program")).toBeVisible();
  // The lifetime total is a whole-history figure, so paging in more sessions
  // leaves it where it was.
  await expect(page.getByText("5,000 lb lifted across 2 sessions")).toBeVisible();
  await expect(page.getByText("2,500 lb lifted")).toHaveCount(2);
  // All loaded, so the button is gone.
  await expect(page.getByRole("button", { name: "Load more" })).toHaveCount(0);
});

test("shows each lift's top set on the progress page", async ({ page }) => {
  await page.route("**/api/v1/exercises**", (route) =>
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

// The path a tap takes, and the reason the panel is reachable on an iPad at all:
// a touch device never hovers, so without the onclick handler there would be no
// way to open it.
//
// Dispatched rather than clicked, and with no hover anywhere in the test, because
// every version that mixed the two flaked. LinkPreview opens on hover after
// openDelay (150ms) and closes after closeDelay (200ms), and Playwright's click()
// moves the pointer onto the trigger before pressing — so a real click is always
// racing those timers, and which way it goes depends on how loaded the runner is.
// Asserting "click opens it" lost that race when hover got there first; asserting
// "hover opens, click closes, click opens" then lost it on the second click, which
// landed inside the close delay.
//
// dispatchEvent fires the handler with the pointer never entering the element, so
// no timer is running and the only state in play is the component's own `open`.
// That is also a better model of a tap than a synthetic mouse click is.
test("toggles the changelog when the header version is clicked", async ({ page }) => {
  const trigger = await openHeaderWithVersion(page);
  const panel = page.getByTestId("changelog-panel");

  // Nothing has been hovered, so the panel starts closed — which is what makes
  // the next assertion about the handler rather than about hover.
  await expect(panel).toBeHidden();

  await trigger.dispatchEvent("click");
  await expect(panel).toBeVisible();

  await trigger.dispatchEvent("click");
  await expect(panel).toBeHidden();
});

// The Racked recap. Every figure here is computed by the API, so these assert
// that the page renders the report faithfully — not that the statistics are
// right, which is internal/racked's own test suite.
const rackedMarch = {
  period: {
    kind: "month",
    start: "2026-03-01",
    end: "2026-03-31",
    label: "March 2026",
    inProgress: false,
  },
  totals: { volumeLb: 84000, sessions: 12, sets: 180, reps: 900 },
  change: { volumeLb: 9000, volumePct: 0.12, sessions: 2, sessionsPct: 0.2 },
  comparison: { count: 3, label: "school buses", unitLb: 24000 },
  lifts: [
    { exerciseId: 1, exerciseName: "Squat", volumeLb: 50000, sets: 90, reps: 450, share: 0.6 },
    { exerciseId: 2, exerciseName: "Bench Press", volumeLb: 34000, sets: 90, reps: 450, share: 0.4 },
  ],
  series: [
    {
      exerciseId: 1,
      exerciseName: "Squat",
      points: [
        { performedOn: "2026-03-02", topWeightLb: 200, e1rmLb: 233 },
        { performedOn: "2026-03-16", topWeightLb: 220, e1rmLb: 256 },
      ],
    },
    {
      exerciseId: 2,
      exerciseName: "Bench Press",
      points: [
        { performedOn: "2026-03-02", topWeightLb: 150, e1rmLb: 175 },
        { performedOn: "2026-03-16", topWeightLb: 155, e1rmLb: 181 },
      ],
    },
  ],
  mostImproved: {
    exerciseId: 1,
    exerciseName: "Squat",
    fromLb: 233,
    toLb: 256,
    gainLb: 23,
    gainPct: 0.0987,
  },
  days: [
    { date: "2026-03-02", volumeLb: 7000, sessions: 1 },
    { date: "2026-03-16", volumeLb: 7500, sessions: 1 },
  ],
  weekdays: [0, 42000, 0, 30000, 0, 12000, 0],
  bestWeekday: 1,
  hours: Array.from({ length: 24 }, (_, h) => (h === 6 ? 9 : h === 18 ? 3 : 0)),
  peakHour: 6,
  hourLabel: "Early bird",
  streak: { longestWeeks: 5, currentWeeks: 3 },
  attendance: { basis: "none", expected: 0, actual: 12, rate: 0, sessionsPerWeek: 2.75 },
  prs: [
    {
      kind: "weight",
      performedOn: "2026-03-16",
      exerciseId: 1,
      exerciseName: "Squat",
      weightLb: 220,
      reps: 5,
      valueLb: 220,
      previousLb: 215,
    },
  ],
  milestones: [
    {
      kind: "plate",
      performedOn: "2026-03-16",
      label: "First 225 lb Squat",
      valueLb: 225,
      exerciseId: 1,
      exerciseName: "Squat",
    },
  ],
  heaviestSet: {
    performedOn: "2026-03-16",
    exerciseId: 1,
    exerciseName: "Squat",
    weightLb: 220,
    reps: 5,
  },
  fastestSession: {
    sessionId: 7,
    performedOn: "2026-03-09",
    programDayName: "Workout A",
    durationSeconds: 2880,
    volumeLb: 6800,
    sets: 15,
  },
  deloads: [
    {
      exerciseId: 2,
      exerciseName: "Bench Press",
      performedOn: "2026-03-09",
      fromLb: 160,
      toLb: 145,
      recovered: false,
      recoveredOn: null,
    },
  ],
  archetype: { name: "The Grinder", description: "Long sessions, no rush." },
};

test("Racked shows the month's headline figures", async ({ page }) => {
  await page.route("**/api/v1/racked**", (route) => route.fulfill({ json: rackedMarch }));
  await page.goto("/#/racked");

  await expect(page.getByRole("heading", { name: "Racked" })).toBeVisible();
  await expect(page.getByText("March 2026")).toBeVisible();

  // The headline and its restatement in something a person can picture.
  await expect(page.getByText("84,000")).toBeVisible();
  await expect(page.getByText("That's 3 school buses.")).toBeVisible();
  await expect(page.getByText("+12% vs the previous month")).toBeVisible();

  await expect(page.getByText("The Grinder")).toBeVisible();
});

test("Racked shows the hero moments", async ({ page }) => {
  await page.route("**/api/v1/racked**", (route) => route.fulfill({ json: rackedMarch }));
  await page.goto("/#/racked");

  // Scoped to each card: the same figures legitimately appear elsewhere on the
  // page — the squat's +10% also labels its line in the chart legend, and the
  // heaviest set is also the record in the PR list.
  const improved = page.getByTestId("stat-most-improved");
  await expect(improved.getByText("Squat")).toBeVisible();
  await expect(improved.getByText("+10%")).toBeVisible();
  await expect(improved.getByText("233 → 256 lb est. max")).toBeVisible();

  const heaviest = page.getByTestId("stat-heaviest-set");
  await expect(heaviest.getByText("220 lb × 5")).toBeVisible();
  await expect(heaviest.getByText("March 16 2026")).toBeVisible();

  // 2880s renders in the units a lifter would say, not as the rest timer's M:SS.
  const fastest = page.getByTestId("stat-fastest-session");
  await expect(fastest.getByText("48m")).toBeVisible();
  await expect(fastest.getByText("Workout A")).toBeVisible();
});

test("Racked charts every lift and names what it cannot draw", async ({ page }) => {
  await page.route("**/api/v1/racked**", (route) => route.fulfill({ json: rackedMarch }));
  await page.goto("/#/racked");

  // The trend chart's legend carries each lift's name and final change, so the
  // chart never depends on telling two colours apart.
  const chart = page.getByRole("img", { name: /Improvement for 2 lifts/ });
  await expect(chart).toBeVisible();

  await expect(page.getByRole("heading", { name: "Where the weight went" })).toBeVisible();
  await expect(page.getByText("50,000 lb · 60%")).toBeVisible();

  await expect(page.getByRole("img", { name: /by day of the week/ })).toBeVisible();
  // Targets the value, not the card: the card also holds this chart's data table,
  // where every weekday appears as a row header — a second legitimate match.
  await expect(page.getByTestId("best-weekday")).toHaveText("Monday");
  await expect(page.getByText("Early bird")).toBeVisible();
});

// With no weekdays on the program there is no target, so the page reports how
// often the lifter trained instead of grading them against a number nobody set.
test("Racked reports frequency when the program has no schedule", async ({ page }) => {
  await page.route("**/api/v1/racked**", (route) => route.fulfill({ json: rackedMarch }));
  await page.goto("/#/racked");

  await expect(page.getByRole("heading", { name: "How often you trained" })).toBeVisible();
  await expect(page.getByText("2.8")).toBeVisible();
  await expect(page.getByRole("heading", { name: "Attendance" })).toHaveCount(0);
});

// Charts convey their detail through hover and title attributes, which never
// reach a keyboard or a screen reader.
test("Racked exposes each chart's numbers as a table", async ({ page }) => {
  await page.route("**/api/v1/racked**", (route) => route.fulfill({ json: rackedMarch }));
  await page.goto("/#/racked");

  const table = page.getByText("Volume by weekday as a table");
  await expect(table).toBeVisible();

  // Collapsed until asked for, so it does not crowd the chart it belongs to.
  await expect(page.getByRole("rowheader", { name: "Monday" })).toBeHidden();
  await table.click();
  await expect(page.getByRole("rowheader", { name: "Monday" })).toBeVisible();
});

test("Racked lists records, milestones and stalls", async ({ page }) => {
  await page.route("**/api/v1/racked**", (route) => route.fulfill({ json: rackedMarch }));
  await page.goto("/#/racked");

  await expect(page.getByRole("heading", { name: "1 personal record" })).toBeVisible();
  await expect(page.getByText("First 225 lb Squat")).toBeVisible();

  await expect(page.getByRole("heading", { name: "Stalls and comebacks" })).toBeVisible();
  await expect(page.getByText("still climbing")).toBeVisible();
});

test("Racked switches to the year", async ({ page }) => {
  const asked: string[] = [];
  // Answer the period that was actually asked for. Returning the year's report
  // to every request would make the assertions pass before the toggle is even
  // clicked, which is the opposite of what this test is for.
  await page.route("**/api/v1/racked**", (route) => {
    const period = new URL(route.request().url()).searchParams.get("period") ?? "month";
    asked.push(period);
    route.fulfill({
      json:
        period === "year"
          ? { ...rackedMarch, period: { ...rackedMarch.period, kind: "year", label: "2026" } }
          : rackedMarch,
    });
  });
  await page.goto("/#/racked");
  await expect(page.getByText("March 2026")).toBeVisible();

  await page.getByRole("radio", { name: "This year" }).click();
  await expect(page.getByText("2026", { exact: true })).toBeVisible();
  expect(asked).toContain("year");
});

test("Racked offers a retry when the report fails to load", async ({ page }) => {
  await page.route("**/api/v1/racked**", (route) =>
    route.fulfill({ status: 500, json: { code: "internal", message: "internal server error" } }),
  );
  await page.goto("/#/racked");

  await expect(page.getByText("Couldn't load your stats.")).toBeVisible();
});

// The one test that puts the share card in front of a real canvas. Everything
// about the card that can be checked without one — what it says, whether it
// fits — is asserted in shareCard.test.ts; that it paints and encodes at all
// needs a browser, because jsdom has no 2D context to paint on.
test("Racked renders the recap as a shareable image", async ({ page }) => {
  await page.route("**/api/v1/racked**", (route) => route.fulfill({ json: rackedMarch }));
  await page.goto("/#/racked");

  await page.getByRole("button", { name: "Share" }).click();

  await expect(page.getByRole("heading", { name: "Share your recap" })).toBeVisible();

  const preview = page.getByRole("img", { name: /Racked March 2026/ });
  await expect(preview).toBeVisible();
  await expect(preview).toHaveAttribute("src", /^blob:/);

  // Decoded, not merely present: a broken blob still has a src attribute, and
  // naturalWidth is the only thing that proves the PNG came out of the canvas
  // at the size the card was drawn at.
  await expect
    .poll(() => preview.evaluate((img: HTMLImageElement) => img.naturalWidth))
    .toBe(1080);
  expect(
    await preview.evaluate((img: HTMLImageElement) => img.naturalHeight),
  ).toBe(1350);
});

test("Racked offers nothing to share from a month with no sessions", async ({ page }) => {
  await page.route("**/api/v1/racked**", (route) =>
    route.fulfill({
      json: {
        ...rackedMarch,
        totals: { volumeLb: 0, sessions: 0, sets: 0, reps: 0 },
      },
    }),
  );
  await page.goto("/#/racked");

  await expect(page.getByText(/Nothing logged in March 2026/)).toBeVisible();
  await expect(page.getByRole("button", { name: "Share" })).toBeHidden();
});
