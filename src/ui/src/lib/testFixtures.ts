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

import type {
  FeedEntry,
  LeaderboardBoard,
  LeaderboardEntry,
  Lifter,
  Notification,
  ProgramSummary,
  RackedReport,
  SessionRecap,
  User,
} from "./api";

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
 * A program card, defaulting to a seeded one: no owner, shared with the whole
 * install, live, and therefore nobody's to edit.
 *
 * That default is the app's ordinary case — eight of the nine programs a fresh
 * install can see are seeded — and it is also the safe one to forget to
 * override. A test that meant "somebody's private program" and got a seeded one
 * asserts against a visible, uneditable card and fails loudly; the other way
 * round, a test that meant "seeded" and got an owned one would quietly render an
 * Edit link nobody checked for.
 *
 * `isMine` is a field rather than something derived from `ownerId` here for the
 * same reason the API sends it: ownership is the server's decision, and a
 * fixture that recomputed it would be testing the recomputation.
 */
export function testProgramSummary(
  overrides: Partial<ProgramSummary> = {},
): ProgramSummary {
  return {
    id: 1,
    name: "StrongLifts 5x5",
    description: "",
    progressionKind: "linear",
    ownerId: null,
    isMine: false,
    isShared: true,
    archivedAt: null,
    ...overrides,
  };
}

/**
 * One notification, defaulting to applause on the signed-in lifter's session.
 *
 * `sessionOwnerId` defaults to 1 — testUser's id — because that is the ordinary
 * case: almost every notification is about your own training. A test covering
 * the `reply` kind, the one that reaches somebody about a session that was
 * never theirs, overrides it, and that override is the thing under test.
 */
export function testNotification(overrides: Partial<Notification> = {}): Notification {
  return {
    id: 1,
    kind: "reaction",
    actor: testLifter(),
    sessionId: 7,
    sessionOwnerId: 1,
    programDayName: "Workout A",
    emoji: "👏",
    createdAt: "2026-03-17T10:00:00Z",
    ...overrides,
  };
}

/**
 * One leaderboard board, defaulting to a sessions-a-week board with no entries.
 *
 * Entries are supplied per test rather than defaulted, because what a board says
 * about who is on it IS the thing under test — a fixture that arrived populated
 * would hide the empty and single-lifter cases, which are the two a renderer is
 * most likely to get wrong.
 */
export function testBoard(overrides: Partial<LeaderboardBoard> = {}): LeaderboardBoard {
  return {
    metric: "sessionsPerWeek",
    label: "Sessions a week",
    unit: "per_week",
    note: "How often each lifter trained.",
    entries: [],
    ...overrides,
  };
}

/** One lifter's place on a board. */
export function testBoardEntry(
  overrides: Partial<LeaderboardEntry> = {},
): LeaderboardEntry {
  return {
    rank: 1,
    lifter: testLifter(),
    value: 3,
    ...overrides,
  };
}

/**
 * A session recap with every optional field absent — a first workout, which has
 * no pace, no previous session and no lift with a delta.
 *
 * Shared for the reason `testRackedReport` is: two routes render this type now —
 * your own recap and another lifter's — and it has too many required fields for
 * each to keep its own hand-built copy honest.
 *
 * `earned` is null here. It is the one field a reader of somebody else's session
 * is not shown at all (see LifterSessionRecap.svelte), so leaving it absent by
 * default means a test has to ask for it before it can assert anything about it.
 */
export function testSessionRecap(overrides: Partial<SessionRecap> = {}): SessionRecap {
  return {
    session: {
      sessionId: 42,
      programId: 1,
      programName: "StrongLifts 5x5",
      programDayId: 7,
      programDayName: "Workout A",
      performedOn: "2026-09-13",
      startedAt: "2026-09-13T18:00:00Z",
      finishedAt: null,
      isOver: false,
    },
    durationSeconds: null,
    pace: null,
    volume: {
      totalLb: 0,
      previousLb: null,
      deltaPct: null,
      comparison: { count: 0, label: "", unitLb: 0 },
      setsLogged: 0,
      setsPrescribed: 5,
      repsLogged: 0,
      repsTargeted: 25,
      setsBonus: 0,
    },
    progress: {
      previousSessionId: null,
      previousPerformedOn: null,
      weightDeltaPct: null,
      liftsCompared: 0,
      liftsNew: 0,
    },
    muscles: [],
    split: {
      main: { volumeLb: 0, sets: 0, reps: 0, lifts: 0, share: 0 },
      assistance: { volumeLb: 0, sets: 0, reps: 0, lifts: 0, share: 0 },
    },
    bodyweightLb: null,
    earned: null,
    lifts: [],
    prs: [],
    milestones: [],
    streak: { sessions: 0, weeks: 0 },
    ...overrides,
  };
}

/**
 * One session in the feed, performed by somebody else.
 *
 * `lifter` defaults to `testLifter()`, so a test that only cares about the
 * session need not build one — and a test that cares whose it is overrides the
 * whole nested object rather than reaching into it.
 */
export function testFeedEntry(overrides: Partial<FeedEntry> = {}): FeedEntry {
  return {
    id: 90,
    lifter: testLifter(),
    programId: 1,
    programName: "StrongLifts 5x5",
    programDayId: 1,
    programDayName: "Workout A",
    performedOn: "2026-03-17",
    setCount: 15,
    completedSetCount: 15,
    volumeLb: 9_000,
    isOver: true,
    // No applause by default, which is the state of most sessions and the one a
    // renderer is most likely to get wrong by drawing an empty badge.
    reactionCount: 0,
    commentCount: 0,
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
