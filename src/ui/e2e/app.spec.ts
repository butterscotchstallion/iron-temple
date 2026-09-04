import { test, expect, type Page } from "@playwright/test";
import type { RackedReport } from "../src/lib/api";

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
        layoffPct: 0,
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
        layoffPct: 0,
      },
    },
  ],
  // Trained recently, so there is nothing to be asked about. The layoff tests
  // below override this.
  layoff: null,
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

/**
 * The batched preview the program screen loads from, built out of a single-day
 * fixture so the two shapes cannot drift apart.
 *
 * It mirrors what the real endpoints do: the same prescription, with the layoff
 * lifted off the day and onto the wrapper, because a layoff describes the
 * lifter rather than any one day.
 */
function nextSessionsFrom(single: {
  programId: number;
  programDayId: number;
  programDayName: string;
  exercises: unknown[];
  layoff?: unknown;
}) {
  return {
    programId: single.programId,
    layoff: single.layoff ?? null,
    days: [
      {
        programDayId: single.programDayId,
        programDayName: single.programDayName,
        exercises: single.exercises,
      },
    ],
  };
}

test.beforeEach(async ({ page }) => {
  await page.route("**/api/v1/me", (route) => route.fulfill({ json: signedInUser }));
  await page.route("**/api/v1/programs", (route) => route.fulfill({ json: programs }));
  await page.route("**/api/v1/sessions**", (route) => route.fulfill({ json: emptySessions }));
  await page.route("**/api/v1/programs/1", (route) => route.fulfill({ json: program1 }));
  // Trailing ** so the glob still matches once a query string is on the URL —
  // the preview carries ?deload=<answer>, the same reason /sessions and
  // /exercises above are wildcarded.
  await page.route("**/api/v1/programs/1/days/10/next-session**", (route) =>
    route.fulfill({ json: nextSession }),
  );
  // The program screen loads through the batched preview and only falls back to
  // the per-day one above when a single day's assistance changes under it, so
  // both have to answer.
  await page.route("**/api/v1/programs/1/next-sessions**", (route) =>
    route.fulfill({ json: nextSessionsFrom(nextSession) }),
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
  await expect(
    page.getByRole("button", { name: "Start", exact: true }),
  ).toBeVisible();

  // The stalled Bench Press surfaces its deload reasoning; the fresh Squat does not.
  await expect(page.getByText("Deload", { exact: true })).toBeVisible();
  await expect(page.getByText("stalled 3× at 72.5 → 65 lb")).toBeVisible();

  // Trained recently, so nothing is asked.
  await expect(page.getByRole("heading", { name: "Welcome back" })).toBeHidden();
});

// ---- Layoff deload ----

// The preview for a lifter three weeks out of the gym, as the server would
// answer it either way. `deload` is the answer the client asked for in the
// query string — the whole point being that the weights on screen are the ones
// Start will use.
//
// The Bench Press is the interesting row: it had already deloaded for a stall
// to 65, and a 30% layoff off the 72.5 it last worked is 50 — deeper, so the
// layoff takes over rather than compounding with it. The Squat has never been
// performed, so it is never cut.
function layoffNextSession(deload: boolean) {
  return {
    ...nextSession,
    exercises: [
      nextSession.exercises[0],
      {
        ...nextSession.exercises[1],
        weightLb: deload ? 50 : 65,
        progression: {
          ...nextSession.exercises[1].progression,
          status: deload ? "layoff" : "deload",
          layoffPct: deload ? 0.3 : 0,
        },
      },
    ],
    layoff: { weeks: 3, lastTrainedOn: "2026-08-01", deloadPct: 0.3, applied: deload },
  };
}

/** Answer the preview according to the deload the client asked for. */
async function routeLayoffPreview(page: Page) {
  await page.route("**/api/v1/programs/1/days/10/next-session**", (route) => {
    const deload = new URL(route.request().url()).searchParams.get("deload") === "true";
    return route.fulfill({ json: layoffNextSession(deload) });
  });
  // Accepting the deload re-previews the whole program in one request, so this
  // is the route the "Deload 30%" button actually goes through.
  await page.route("**/api/v1/programs/1/next-sessions**", (route) => {
    const deload = new URL(route.request().url()).searchParams.get("deload") === "true";
    return route.fulfill({ json: nextSessionsFrom(layoffNextSession(deload)) });
  });
}

test("offers a deload after time away, and applies it to the weights on screen", async ({
  page,
}) => {
  await routeLayoffPreview(page);

  await page.goto("/#/programs/1");
  await expect(page.getByRole("heading", { name: "Welcome back" })).toBeVisible();
  await expect(page.getByText("It's been 3 weeks since you trained", { exact: false })).toBeVisible();

  // The prescription is untouched until the question is answered.
  await expect(page.getByText("5×5 · 65 lb")).toBeVisible();

  await page.getByRole("button", { name: "Deload 30%" }).click();

  // Re-prescribed in place, and the badge says why this weight is not the one
  // the stall deload picked.
  await expect(page.getByText("5×5 · 50 lb")).toBeVisible();
  await expect(page.getByText("time off · 72.5 → 50 lb")).toBeVisible();
  // Answered, so it stops asking.
  await expect(page.getByRole("heading", { name: "Welcome back" })).toBeHidden();
});

test("keeps the prescribed weights when the deload is declined", async ({ page }) => {
  await routeLayoffPreview(page);

  await page.goto("/#/programs/1");
  await page.getByRole("button", { name: "Keep my weights" }).click();

  await expect(page.getByRole("heading", { name: "Welcome back" })).toBeHidden();
  await expect(page.getByText("5×5 · 65 lb")).toBeVisible();
  await expect(page.getByText("stalled 3× at 72.5 → 65 lb")).toBeVisible();
});

// The answer has to reach the session, or the prompt is decoration.
test("starts the session with the deload the lifter accepted", async ({ page }) => {
  await routeLayoffPreview(page);
  const posted: unknown[] = [];
  await page.route("**/api/v1/sessions**", (route) => {
    const request = route.request();
    if (request.method() === "POST") {
      posted.push(request.postDataJSON());
      return route.fulfill({ json: sessionDetail() });
    }
    // Start navigates to the created session, so /sessions/1 has to answer with
    // one — the bare /sessions list is the history behind it.
    return /\/sessions\/\d+/.test(request.url())
      ? route.fulfill({ json: sessionDetail() })
      : route.fulfill({ json: emptySessions });
  });

  await page.goto("/#/programs/1");
  await page.getByRole("button", { name: "Deload 30%" }).click();
  await expect(page.getByText("5×5 · 50 lb")).toBeVisible();
  await page.getByRole("button", { name: "Start", exact: true }).click();

  await expect.poll(() => posted).toEqual([{ programDayId: 10, deload: true }]);
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
  // The top set arrives on the list row. The page used to fetch every lift's
  // whole history and take the maximum itself, which is why this test once had
  // a history route per exercise; picking the heaviest set is the server's job
  // now, pinned against the history endpoint by TestListExercisesCarriesTopSet.
  await page.route("**/api/v1/exercises**", (route) =>
    route.fulfill({
      json: [
        { id: 1, name: "Squat", topSet: { weightLb: 135, performedOn: "2026-08-01" } },
        { id: 2, name: "Bench Press", topSet: null },
      ],
    }),
  );
  // Registered last so it wins: the list glob above ends in ** and would
  // otherwise swallow the history URLs too, making the assertion below vacuous.
  const historyCalls: string[] = [];
  await page.route("**/api/v1/exercises/*/history", (route) => {
    historyCalls.push(route.request().url());
    return route.fulfill({ json: [] });
  });

  await page.goto("/#/progress");
  await expect(page.getByRole("heading", { name: "Progress", exact: true })).toBeVisible();
  await expect(page.getByText("Squat")).toBeVisible();
  await expect(page.getByText("135 lb")).toBeVisible();
  // A null topSet is a lift never performed, and reads as such.
  await expect(page.getByText("No sessions yet")).toBeVisible();
  // The N+1 this replaced: rendering the page must cost no history request at
  // all, or the round trips are still being paid one card at a time.
  expect(historyCalls).toEqual([]);
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

// Detecting a release landing under an open tab, and taking it without losing
// the workout. Two things here can only be exercised in a real browser: the
// clock driving the poll interval, and the reload itself — jsdom has no
// navigation, so UpdatePrompt.test.ts stubs it and this is where it's real.

/** Serve /health a version per call, sticking on the last one thereafter. */
async function routeHealth(page: import("@playwright/test").Page, versions: string[]) {
  let call = 0;
  await page.route("**/api/v1/health", (route) => {
    const version = versions[Math.min(call, versions.length - 1)];
    call += 1;
    return route.fulfill({ json: { status: "ok", version, environment: "production" } });
  });
}

/**
 * Deploy a new version under an already-open tab, and wait for the prompt.
 *
 * The clock is frozen only long enough to jump the five-minute poll interval —
 * waiting it out for real is not an option — and then handed straight back with
 * resume(), so the dialog's own open/close behaviour runs on ordinary time
 * rather than on timers this test has to remember to advance.
 */
async function deployUnderTab(page: import("@playwright/test").Page) {
  await routeHealth(page, ["v1.0.0", "v2.0.0"]);
  await page.clock.install();
}

test("offers a new version, and stops asking once it's declined", async ({ page }) => {
  await deployUnderTab(page);

  await page.goto("/");
  await expect(page.getByTestId("version")).toContainText("iron-temple v1.0.0");

  const prompt = page.getByTestId("update-prompt");
  await expect(prompt).toBeHidden();

  // A release lands: the next poll sees a version this page isn't running.
  await page.clock.runFor("06:00");
  await page.clock.resume();

  await expect(prompt).toBeVisible();
  await expect(prompt).toContainText("v2.0.0");
  await expect(prompt).toContainText("every set you've logged is already saved");

  // The header keeps naming the build actually on screen, not the new one.
  await expect(page.getByTestId("version")).toContainText("iron-temple v1.0.0");

  await page.getByRole("button", { name: "Not now" }).click();
  await expect(prompt).toBeHidden();

  // Declining is per-version: later polls returning the same v2.0.0 must not
  // put it back up, or every five minutes becomes an interruption.
  await page.clock.fastForward("06:00");
  await expect(prompt).toBeHidden();
});

test("loads the new version when the update is accepted", async ({ page }) => {
  await deployUnderTab(page);

  await page.goto("/");
  const prompt = page.getByTestId("update-prompt");

  // Let the first /health land before jumping the clock, the same way the
  // declining test above does. routeHealth answers by call count, and
  // startPolling()'s opening poll is call #1 — jump the interval while that is
  // still in the air and the tick hits poll()'s inFlight guard, which DROPS it
  // rather than queueing it. v2.0.0 would then never be asked for, the prompt
  // would never appear, and the failure would read as a mysterious timeout on
  // an assertion that has nothing to do with the cause.
  await expect(page.getByTestId("version")).toContainText("iron-temple v1.0.0");

  await page.clock.runFor("06:00");
  await page.clock.resume();
  await expect(prompt).toBeVisible();

  await page.getByRole("button", { name: "Load it" }).click();

  // Reloaded onto the new build — and re-baselined on it, so it does not
  // immediately offer the update it has just taken.
  await expect(page.getByTestId("version")).toContainText("iron-temple v2.0.0");
  await expect(prompt).toBeHidden();
});

// The promise the dialog makes. A reload mid-workout is only safe to offer if
// the session comes back as it was — and the rest countdown is the one piece of
// it that never reaches the server, so it's the piece that has to be proven.
//
// Driven with a plain reload rather than the dialog: this is about what survives
// the navigation, and the tests above already cover how one gets triggered. No
// fake clock either — the exact arithmetic is restStorage.test.ts's job, and
// real time keeps this test honest about a real page load.
test("carries a running rest timer through a reload", async ({ page }) => {
  // One set logged, one not: started (so the timer is on screen) but not over.
  await page.route("**/api/v1/sessions/1", (route) =>
    route.fulfill({
      json: sessionDetail({ actualReps: 5, completed: true, loggedSets: 1 }),
    }),
  );
  await page.route("**/api/v1/exercises/1/history", (route) => route.fulfill({ json: [] }));

  await page.goto("/#/sessions/1");
  const remaining = page.getByTestId("rest-remaining");
  const start = page.getByRole("button", { name: "Start", exact: true });
  await expect(remaining).toHaveText("3:00");

  await start.click();
  // Ticking, so the reload below has a rest actually in progress to preserve.
  await expect(remaining).not.toHaveText("3:00");
  const before = seconds(await remaining.textContent());

  await page.reload();

  // Picked up where it left off: still running, still counting down from where
  // it was — not restarted at a stopped 3:00, which is what a reload used to
  // cost. The exact arithmetic is restStorage.test.ts's; this is the guarantee.
  await expect(start).toBeDisabled();
  const after = seconds(await remaining.textContent());
  expect(after).toBeGreaterThan(0);
  expect(after).toBeLessThanOrEqual(before);
});

/** "2:57" → 177. The inverse of the countdown's own formatting. */
function seconds(display: string | null): number {
  const [minutes, secs] = (display ?? "").split(":").map(Number);
  return (minutes || 0) * 60 + (secs || 0);
}

// The Racked recap. Every figure here is computed by the API, so these assert
// that the page renders the report faithfully — not that the statistics are
// right, which is internal/racked's own test suite.
//
// Typed against the generated contract, unlike the smaller fixtures above, and
// deliberately so: the report carries twenty-odd required fields and the page
// reads them without guarding, so a field added to the spec and forgotten here
// blanks the page at runtime. Untyped, that surfaced only in this suite — the
// one gate a sandbox cannot run — and every Racked test failed at once with a
// missing heading rather than pointing at the missing field. `pnpm check` now
// names it instead. See tsconfig.json, where e2e joined the type-check for this.
const rackedMarch: RackedReport = {
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
  split: {
    main: { volumeLb: 84000, sets: 180, reps: 900, lifts: 2, share: 1 },
    assistance: { volumeLb: 0, sets: 0, reps: 0, lifts: 0, share: 0 },
  },
  muscles: [
    { group: "legs", volumeLb: 50000, sets: 90, reps: 450, lifts: 1, share: 0.6, trained: true },
    { group: "chest", volumeLb: 34000, sets: 90, reps: 450, lifts: 1, share: 0.4, trained: true },
    { group: "back", volumeLb: 0, sets: 0, reps: 0, lifts: 0, share: 0, trained: false },
    { group: "shoulders", volumeLb: 0, sets: 0, reps: 0, lifts: 0, share: 0, trained: false },
    { group: "arms", volumeLb: 0, sets: 0, reps: 0, lifts: 0, share: 0, trained: false },
    { group: "core", volumeLb: 0, sets: 0, reps: 0, lifts: 0, share: 0, trained: false },
    { group: "other", volumeLb: 0, sets: 0, reps: 0, lifts: 0, share: 0, trained: false },
  ],
  lifts: [
    {
      exerciseId: 1,
      exerciseName: "Squat",
      volumeLb: 50000,
      sets: 90,
      reps: 450,
      share: 0.6,
      isAssistance: false,
    },
    {
      exerciseId: 2,
      exerciseName: "Bench Press",
      volumeLb: 34000,
      sets: 90,
      reps: 450,
      share: 0.4,
      isAssistance: false,
    },
  ],
  series: [
    {
      exerciseId: 1,
      exerciseName: "Squat",
      isAssistance: false,
      points: [
        { performedOn: "2026-03-02", topWeightLb: 200, e1rmLb: 233 },
        { performedOn: "2026-03-16", topWeightLb: 220, e1rmLb: 256 },
      ],
    },
    {
      exerciseId: 2,
      exerciseName: "Bench Press",
      isAssistance: false,
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
  bodyweight: null,
  days: [
    { date: "2026-03-02", volumeLb: 7000, sessions: 1 },
    { date: "2026-03-16", volumeLb: 7500, sessions: 1 },
  ],
  weekdays: [0, 42000, 0, 30000, 0, 12000, 0],
  bestWeekday: 1,
  // Cast for the same reason the vitest fixtures cast it: the contract types
  // this as a 24-tuple, and Array.from produces a plain array.
  hours: Array.from({ length: 24 }, (_, h) =>
    h === 6 ? 9 : h === 18 ? 3 : 0,
  ) as RackedReport["hours"],
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

  // Scoped to the lift breakdown, not the page. The muscle card below states
  // the same figure whenever one movement is all of its group's volume — which
  // for the squat and legs is the normal case, not a quirk of this fixture.
  const lifts = page.getByTestId("stat-lifts");
  await expect(lifts.getByRole("heading", { name: "Where the weight went" })).toBeVisible();
  await expect(lifts.getByText("50,000 lb · 60%")).toBeVisible();

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

// A month with assistance work and a run of weigh-ins — the two sections
// rackedMarch has nothing to show for, since most lifters have neither.
const rackedWithAssistanceAndBodyweight: RackedReport = {
  ...rackedMarch,
  split: {
    main: { volumeLb: 76000, sets: 160, reps: 800, lifts: 2, share: 0.905 },
    assistance: { volumeLb: 8000, sets: 20, reps: 100, lifts: 1, share: 0.095 },
  },
  lifts: [
    ...rackedMarch.lifts,
    {
      exerciseId: 3,
      exerciseName: "Barbell Curl",
      volumeLb: 8000,
      sets: 20,
      reps: 100,
      share: 0.095,
      isAssistance: true,
    },
  ],
  // Inherited muscles would say arms went untrained in a month that includes a
  // curl. The slices divide the same total the split does, so they move with it.
  muscles: [
    { group: "legs", volumeLb: 50000, sets: 90, reps: 450, lifts: 1, share: 0.595, trained: true },
    { group: "chest", volumeLb: 26000, sets: 70, reps: 350, lifts: 1, share: 0.31, trained: true },
    { group: "arms", volumeLb: 8000, sets: 20, reps: 100, lifts: 1, share: 0.095, trained: true },
    { group: "back", volumeLb: 0, sets: 0, reps: 0, lifts: 0, share: 0, trained: false },
    { group: "shoulders", volumeLb: 0, sets: 0, reps: 0, lifts: 0, share: 0, trained: false },
    { group: "core", volumeLb: 0, sets: 0, reps: 0, lifts: 0, share: 0, trained: false },
    { group: "other", volumeLb: 0, sets: 0, reps: 0, lifts: 0, share: 0, trained: false },
  ],
  bodyweight: {
    points: [
      { performedOn: "2026-03-02", weightLb: 184 },
      { performedOn: "2026-03-09", weightLb: 182.5 },
      { performedOn: "2026-03-16", weightLb: 181.4 },
    ],
    startLb: 184,
    endLb: 181.4,
    lowLb: 181.4,
    highLb: 184,
    changeLb: -2.6,
    changePct: -0.0141,
  },
};

// The split divides the headline rather than qualifying it, so both halves have
// to read against the same total the card above them prints.
test("Racked splits the volume into main work and assistance", async ({ page }) => {
  await page.route("**/api/v1/racked**", (route) =>
    route.fulfill({ json: rackedWithAssistanceAndBodyweight }),
  );
  await page.goto("/#/racked");

  const split = page.getByTestId("work-split");
  await expect(split).toBeVisible();
  await expect(split).toContainText("91%");
  await expect(split).toContainText("10%");
  await expect(split).toContainText("across 1 movement");

  // The assistance lift keeps its place in the volume ranking and is tagged
  // there, rather than being moved into a list of its own.
  await expect(page.getByText("Barbell Curl")).toBeVisible();
  await expect(page.getByText("assistance", { exact: true })).toBeVisible();
});

// The muscle card is the one section whose most useful rows are the empty ones,
// so what this checks is mostly that they are still there.
test("Racked accounts for every muscle group, trained or not", async ({ page }) => {
  await page.route("**/api/v1/racked**", (route) =>
    route.fulfill({ json: rackedWithAssistanceAndBodyweight }),
  );
  await page.goto("/#/racked");

  const card = page.getByTestId("stat-muscles");
  await expect(card).toBeVisible();
  await expect(card).toContainText("Legs");
  await expect(card).toContainText("Arms");
  // Kept and labelled rather than dropped or drawn as a very short bar.
  await expect(card).toContainText("Core");
  await expect(card).toContainText("not trained");
  // And named in prose, since a column of empty tracks is the easiest thing on
  // the page to skim past.
  await expect(card).toContainText("Nothing logged for back, shoulders, core and other");
});

// The bodyweight chart is plain SVG, so this is the only suite that lays it out
// for real — jsdom renders the markup without ever computing a path.
test("Racked charts bodyweight and its change", async ({ page }) => {
  await page.route("**/api/v1/racked**", (route) =>
    route.fulfill({ json: rackedWithAssistanceAndBodyweight }),
  );
  await page.goto("/#/racked");

  const card = page.getByTestId("stat-bodyweight");
  await expect(card).toBeVisible();
  await expect(card).toContainText("181.4");
  await expect(card).toContainText("−2.6 lb");
  await expect(card).toContainText("Range 181.4–184 lb");
  await expect(card.getByRole("img", { name: /Bodyweight across 3 weigh-ins/ })).toBeVisible();
});

// Recording a bodyweight is optional on every session, so most periods hold
// none — and the section is absent rather than empty.
test("Racked omits bodyweight and the split when there is nothing to show", async ({ page }) => {
  await page.route("**/api/v1/racked**", (route) => route.fulfill({ json: rackedMarch }));
  await page.goto("/#/racked");

  await expect(page.getByRole("heading", { name: "Where the weight went" })).toBeVisible();
  await expect(page.getByTestId("stat-bodyweight")).toHaveCount(0);
  await expect(page.getByTestId("work-split")).toHaveCount(0);
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

test("Racked switches to the week", async ({ page }) => {
  const asked: string[] = [];
  // Same shape as the year test above, and for the same reason: answer the
  // period actually asked for, or the assertion passes before the click.
  await page.route("**/api/v1/racked**", (route) => {
    const period = new URL(route.request().url()).searchParams.get("period") ?? "month";
    asked.push(period);
    route.fulfill({
      json:
        period === "week"
          ? {
              ...rackedMarch,
              period: {
                kind: "week",
                start: "2026-03-16",
                end: "2026-03-22",
                label: "March 16–22 2026",
                inProgress: false,
              },
            }
          : rackedMarch,
    });
  });
  await page.goto("/#/racked");
  await expect(page.getByText("March 2026")).toBeVisible();

  await page.getByRole("radio", { name: "This week" }).click();
  await expect(page.getByText("March 16–22 2026")).toBeVisible();
  expect(asked).toContain("week");

  // A week has no rhythm for the heatmap to draw, and its columns would be the
  // wrong seven days anyway — see the note in Racked.svelte.
  await expect(page.getByRole("heading", { name: "Every training day" })).toHaveCount(0);
  await expect(page.getByText("Days trained")).toBeVisible();
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
