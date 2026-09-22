/**
 * Fixtures the unit tests share.
 *
 * Seven test files were each building the same signed-in lifter by hand, which
 * is fine until a required field is added to User: then seven files fail to
 * compile and get seven separate hand-applied fixes, and any that a test happens
 * not to typecheck against goes quietly stale instead.
 *
 * Only the identity lives here. Anything a test is actually asserting about —
 * the bar and rack ExerciseCard's warm-up arithmetic assumes, the gym a profile
 * test signs in with — stays in that file as an override, where its reasoning
 * can be read next to the assertions that depend on it.
 */

import type { Lifter, RackedReport, User } from "./api";

/**
 * A signed-in lifter. An admin, because the first account to register claims the
 * install, so the ordinary case in this app IS the admin — and with no avatar,
 * so components take the initials path unless a test says otherwise.
 */
export function testUser(overrides: Partial<User> = {}): User {
  return {
    id: 1,
    username: "ada",
    displayName: "Ada Lovelace",
    avatarColor: "",
    isAdmin: true,
    // The ordinary case: an account whose owner chose their own password. Only
    // accounts the admin area created start out owing a change, and the one
    // screen that cares overrides this.
    mustChangePassword: false,
    hasAvatar: false,
    ...overrides,
  };
}

/**
 * Somebody else on this install, as the roster describes them.
 *
 * Note what is missing next to `testUser`: no `isAdmin`, no
 * `mustChangePassword`, and no gym. That is the `Lifter` schema rather than an
 * incomplete fixture — those fields are what one lifter may NOT know about
 * another, so a test that could set them here would be testing a shape the API
 * cannot send.
 *
 * `lastTrainedOn` is deliberately absent by default, which is the state of a
 * freshly created account and the one a renderer is most likely to read a
 * property off nothing for.
 */
export function testLifter(overrides: Partial<Lifter> = {}): Lifter {
  return {
    id: 2,
    username: "grace",
    displayName: "Grace Hopper",
    avatarColor: "",
    hasAvatar: false,
    ...overrides,
  };
}

/**
 * A Racked recap with every optional section absent — the shape of a quiet
 * month, and the base every populated variant is spread over.
 *
 * Here rather than in one test file for exactly the reason given at the top of
 * this one, only more so: `RackedReport` has some two dozen required fields, so
 * a second file hand-building one is the stated failure mode with the odds
 * stacked. Both the Racked page and a lifter's profile render this type.
 *
 * Every optional section is null and every count is zero, so a test that wants a
 * populated one says which parts it cares about and nothing else — and a
 * renderer that reads a property off a missing section fails in the test that
 * did not mention it.
 */
export function testRackedReport(overrides: Partial<RackedReport> = {}): RackedReport {
  const muscleGroups = ["chest", "back", "legs", "shoulders", "arms", "core", "other"];
  return {
    period: {
      kind: "month",
      start: "2026-03-01",
      end: "2026-03-31",
      label: "March 2026",
      inProgress: false,
    },
    totals: { volumeLb: 0, sessions: 0, sets: 0, reps: 0 },
    change: null,
    comparison: { count: 0, label: "", unitLb: 0 },
    split: {
      main: { volumeLb: 0, sets: 0, reps: 0, lifts: 0, share: 0 },
      assistance: { volumeLb: 0, sets: 0, reps: 0, lifts: 0, share: 0 },
    },
    // The whole taxonomy, untrained. The API sends every group precisely so the
    // empty ones are visible — see the RackedReport schema — so a fixture that
    // sent an empty array would be a shape the server never produces.
    muscles: muscleGroups.map((group) => ({
      group,
      volumeLb: 0,
      sets: 0,
      reps: 0,
      lifts: 0,
      share: 0,
      trained: false,
    })) as RackedReport["muscles"],
    lifts: [],
    series: [],
    mostImproved: null,
    bodyweight: null,
    days: [],
    weekdays: [0, 0, 0, 0, 0, 0, 0],
    bestWeekday: -1,
    hours: Array.from({ length: 24 }, () => 0) as RackedReport["hours"],
    peakHour: -1,
    hourLabel: "",
    streak: { longestWeeks: 0, currentWeeks: 0 },
    attendance: {
      basis: "none",
      expected: 0,
      actual: 0,
      rate: 0,
      sessionsPerWeek: 0,
      weekdays: [],
    },
    prs: [],
    milestones: [],
    heaviestSet: null,
    fastestSession: null,
    deloads: [],
    archetype: { name: "", description: "" },
    ...overrides,
  };
}
