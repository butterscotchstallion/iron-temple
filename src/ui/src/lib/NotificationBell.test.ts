import { render, screen, waitFor, fireEvent } from "@testing-library/svelte";
import { afterAll, afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import NotificationBell from "./NotificationBell.svelte";
import { auth } from "./auth.svelte";
import { notifications, poll } from "./notifications.svelte";
import { achievements, resetAchievements } from "./achievements.svelte";
import {
  testAchievementHolders,
  testLifter,
  testNotification,
  testUser,
} from "./testFixtures";

// The header bell and the panel behind it.
//
// Two things here are worth more than the rest. The badge has to be readable
// without opening anything — it is the only part of this feature most page
// loads ever show — and it has to say "unread" to a screen reader as text, not
// as a coloured dot. And the row's link target depends on WHO owns the session:
// a reply reaches you about somebody else's workout, and sending that to your
// own recap route would 404.

const listNotifications = vi.hoisted(() => vi.fn());
const markNotificationsRead = vi.hoisted(() => vi.fn());
const markNotificationRead = vi.hoisted(() => vi.fn());
const clearNotifications = vi.hoisted(() => vi.fn());
const getNotificationGroupMembers = vi.hoisted(() => vi.fn());
vi.mock("./api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./api")>()),
  listNotifications,
  markNotificationsRead,
  markNotificationRead,
  clearNotifications,
  getNotificationGroupMembers,
}));

const push = vi.hoisted(() => vi.fn());
vi.mock("svelte-spa-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("svelte-spa-router")>()),
  push,
}));

const ME = 1;
const THEM = 2;

/** Answer the list endpoint with these rows and this count. */
function served(items: unknown[], unreadCount = items.length) {
  listNotifications.mockResolvedValue({
    status: 200,
    data: { items, limit: 20, offset: 0, unreadCount },
  });
}

/**
 * Put these rows in the panel, both in the state the bell reads and in the
 * answer the endpoint gives.
 *
 * Both halves are needed and for different reasons. The badge is drawn from
 * module state before anything is opened — App.svelte's poller fills it in the
 * real app, and there is no App here — while opening the panel asks for a fresh
 * page, so a test that seeded only the state would watch that poll replace its
 * rows with the default empty answer.
 */
function seed(items: unknown[], unreadCount = items.length) {
  served(items, unreadCount);
  notifications.items = items as typeof notifications.items;
  notifications.unread = unreadCount;
  notifications.loaded = true;
}

beforeEach(() => {
  vi.clearAllMocks();
  served([]);
  markNotificationsRead.mockResolvedValue({ status: 204, data: undefined });
  markNotificationRead.mockResolvedValue({ status: 204, data: undefined });
  clearNotifications.mockResolvedValue({ status: 204, data: undefined });
  // One member by default — a crown row that folds only itself, which is the
  // common case. Tests that care about cycling supply their own list.
  getNotificationGroupMembers.mockResolvedValue({
    status: 200,
    data: { items: [crownRow()] },
  });
  auth.me = testUser({ id: ME });
  auth.loaded = true;
  // The module's state outlives any one component, which is the point of it —
  // so each test starts from a known panel rather than the previous one's.
  notifications.items = [];
  notifications.unread = 0;
  notifications.loaded = false;
  notifications.failed = false;
});

afterEach(() => {
  notifications.items = [];
  notifications.unread = 0;
});

// This file grew a dialog, so it inherits UpdatePrompt.test.ts's teardown race.
//
// bits-ui's body-scroll-lock resets the document's styles on a TIMER scheduled
// when the dialog unmounts. The last test's cleanup leaves one pending with
// nothing after it, and if vitest tears the jsdom environment down first it fires
// into a world with no `document` — which fails the whole run on an unhandled
// error while every test still reports as passing, because none of them did
// anything wrong.
//
// It is a race, so it turns on machine speed: the sandbox wins it and CI, being
// slower, does not. Waiting here removes it rather than making it rarer.
// Deliberately afterAll and not afterEach — see that file for why unmounting by
// hand between tests trades this for a pile of derived_inert warnings.
afterAll(async () => {
  await new Promise((resolve) => setTimeout(resolve, 100));
});

/** Open the panel and wait for it to draw. */
async function open() {
  await fireEvent.click(screen.getByRole("button", { name: /notifications/i }));
}

/**
 * A crown row: no session, no comment, no emoji, and a board instead.
 *
 * Spelled out rather than defaulted in the fixture because every one of those
 * absences is load-bearing — a crown that arrived carrying a sessionId would
 * take the reader to a recap, which is the bug this shape prevents.
 */
function crownRow(overrides: Record<string, unknown> = {}) {
  return testNotification({
    id: 90,
    kind: "crown",
    emoji: undefined,
    sessionId: undefined,
    sessionOwnerId: undefined,
    programDayName: undefined,
    achievementSlug: "crown-streak",
    actor: testLifter({ id: THEM, displayName: "Grace Hopper" }),
    ...overrides,
  });
}

describe("the badge", () => {
  it("is absent when there is nothing unread", () => {
    render(NotificationBell);
    expect(
      screen.getByRole("button", { name: "Notifications" }),
    ).toBeInTheDocument();
  });

  it("counts the unread in the trigger's accessible name", async () => {
    notifications.unread = 3;
    render(NotificationBell);
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Notifications, 3 unread" }),
      ).toBeInTheDocument(),
    );
  });

  it("stops counting past 99 rather than widening", async () => {
    notifications.unread = 140;
    render(NotificationBell);
    await waitFor(() => expect(screen.getByText("99+")).toBeInTheDocument());
    // The exact number still reaches a screen reader, which has room for it.
    expect(
      screen.getByRole("button", { name: "Notifications, 140 unread" }),
    ).toBeInTheDocument();
  });
});

describe("the panel", () => {
  it("says what happened, to which workout, and when", async () => {
    seed([
      testNotification({
        actor: testLifter({ id: THEM, displayName: "Grace Hopper" }),
        programDayName: "Workout A",
      }),
    ]);
    render(NotificationBell);
    await open();

    await waitFor(() =>
      expect(screen.getByText("Grace Hopper")).toBeInTheDocument(),
    );
    expect(screen.getByText(/applauded your Workout A/)).toBeInTheDocument();
  });

  it("shows a comment's body under the sentence", async () => {
    seed([
      testNotification({
        kind: "comment",
        emoji: undefined,
        commentBody: "strong sets",
      }),
    ]);
    render(NotificationBell);
    await open();

    await waitFor(() =>
      expect(screen.getByText(/strong sets/)).toBeInTheDocument(),
    );
    expect(screen.getByText(/commented on your Workout A/)).toBeInTheDocument();
  });

  it("tells an empty panel apart from one that hasn't loaded", async () => {
    notifications.loaded = true;
    render(NotificationBell);
    await open();

    await waitFor(() =>
      expect(screen.getByText(/Nothing yet/)).toBeInTheDocument(),
    );
  });

  it("offers a message when the last poll failed", async () => {
    // The refresh that opening triggers has to fail too, or it would clear the
    // very state this is asserting.
    listNotifications.mockResolvedValue({ status: 0, data: undefined });
    notifications.failed = true;
    render(NotificationBell);
    await open();

    await waitFor(() =>
      expect(screen.getByRole("alert")).toHaveTextContent("Couldn't load these"),
    );
  });
});

describe("a row that folds several people", () => {
  // The whole point of the grouping: six lifters applauding one session is one
  // row that names some of them, not six rows that each say it once. The API
  // decides what folds; what is under test here is the sentence.

  it("names both people when a row folds two", async () => {
    seed([
      testNotification({
        actor: testLifter({ id: THEM, displayName: "Grace Hopper" }),
        actorCount: 2,
        otherActorNames: ["Ada Lovelace"],
      }),
    ]);
    render(NotificationBell);
    await open();

    await waitFor(() =>
      expect(
        screen.getByText("Grace Hopper and Ada Lovelace"),
      ).toBeInTheDocument(),
    );
    // Still one sentence about one session, not one per person.
    expect(screen.getByText(/applauded your Workout A/)).toBeInTheDocument();
  });

  it("counts the rest past the two it names", async () => {
    seed([
      testNotification({
        actor: testLifter({ id: THEM, displayName: "Grace Hopper" }),
        actorCount: 6,
        // The API sends at most two spare names however many people are in the
        // group, so the remainder is arithmetic rather than a longer list.
        otherActorNames: ["Ada Lovelace", "Katherine Johnson"],
      }),
    ]);
    render(NotificationBell);
    await open();

    await waitFor(() =>
      expect(
        screen.getByText("Grace Hopper, Ada Lovelace and 4 others"),
      ).toBeInTheDocument(),
    );
  });

  it("says one other in the singular", async () => {
    seed([
      testNotification({
        actor: testLifter({ id: THEM, displayName: "Grace Hopper" }),
        actorCount: 3,
        otherActorNames: ["Ada Lovelace", "Katherine Johnson"],
      }),
    ]);
    render(NotificationBell);
    await open();

    await waitFor(() =>
      expect(
        screen.getByText("Grace Hopper, Ada Lovelace and 1 other"),
      ).toBeInTheDocument(),
    );
  });

  // A folded row is still one row of the panel, so it is one unread thing and
  // one tap. The server marks every notification behind it read from this id.
  it("marks the whole fold read as one, from the id it was given", async () => {
    seed(
      [
        testNotification({
          id: 21,
          sessionId: 42,
          sessionOwnerId: ME,
          actorCount: 4,
          otherActorNames: ["Ada Lovelace"],
        }),
      ],
      1,
    );
    render(NotificationBell);
    await open();

    await fireEvent.click(await screen.findByText(/applauded your/i));

    expect(markNotificationRead).toHaveBeenCalledWith(21);
    // One, not four: the badge was counting the group rather than its members.
    await waitFor(() => expect(notifications.unread).toBe(0));
  });
});

describe("where a row goes", () => {
  it("sends applause on your own session to your own recap", async () => {
    seed([testNotification({ sessionId: 7, sessionOwnerId: ME })]);
    render(NotificationBell);
    await open();

    await fireEvent.click(await screen.findByText(/applauded your/));
    await waitFor(() => expect(push).toHaveBeenCalledWith("/sessions/7/recap"));
  });

  it("sends a reply to the OWNER's recap, not the reader's", async () => {
    // The case that makes sessionOwnerId necessary: this notification is about
    // a session belonging to somebody else entirely, reaching a lifter who had
    // also commented on it. The caller's own recap route would 404 here.
    seed([
      testNotification({
        kind: "reply",
        emoji: undefined,
        commentBody: "how heavy?",
        sessionId: 7,
        sessionOwnerId: THEM,
      }),
    ]);
    render(NotificationBell);
    await open();

    await fireEvent.click(await screen.findByText(/also replied on/));
    await waitFor(() =>
      expect(push).toHaveBeenCalledWith("/lifters/2/sessions/7/recap"),
    );
  });

  it("sends a new lifter to their profile", async () => {
    seed([
      testNotification({
        kind: "joined",
        emoji: undefined,
        sessionId: undefined,
        sessionOwnerId: undefined,
        programDayName: undefined,
        actor: testLifter({ id: THEM, displayName: "Grace Hopper" }),
      }),
    ]);
    render(NotificationBell);
    await open();

    await fireEvent.click(await screen.findByText(/joined the gym/));
    await waitFor(() => expect(push).toHaveBeenCalledWith("/lifters/2"));
  });

  // A crown goes NOWHERE, and that is the point of it: the row folds every crown
  // on the install, so no single route could show them all. It opens a dialog
  // instead, and navigating is something the reader chooses from in there.
  it("takes a crown to a dialog rather than a route", async () => {
    seed([crownRow()]);
    render(NotificationBell);
    await open();

    // "a crown" rather than "the crown on …": no catalogue is seeded in this
    // block, and what a row DOES must not depend on whether the board's name has
    // arrived.
    await fireEvent.click(await screen.findByText(/took a crown/));

    await waitFor(() => expect(screen.getByTestId("achievement-dialog")).toBeInTheDocument());
    expect(push).not.toHaveBeenCalled();
  });
});

// The one kind whose sentence depends on a second endpoint having answered. The
// board name lives in the achievements catalogue, so the panel has to read well
// both when it is there and when it is not.
describe("a crown", () => {
  beforeEach(() => {
    achievements.items = [testAchievementHolders()];
    achievements.loaded = true;
  });
  afterEach(() => resetAchievements());

  it("names the board it was won on", async () => {
    seed([crownRow()]);
    render(NotificationBell);
    await open();

    expect(
      await screen.findByText(/took the crown on Top of Week streak/),
    ).toBeInTheDocument();
  });

  // The API withholds the slug when a row folded crowns from several boards,
  // because naming one of them would be a claim the group does not support. The
  // panel has to say the unnamed thing rather than render a gap.
  it("says the unnamed thing when the row spans boards", async () => {
    seed([crownRow({ achievementSlug: undefined, actorCount: 3 })]);
    render(NotificationBell);
    await open();

    expect(await screen.findByText(/took a crown/)).toBeInTheDocument();
  });

  // Same fallback, different cause: the catalogue is a separate request and may
  // not have landed when the panel is opened.
  it("says the unnamed thing before the catalogue has loaded", async () => {
    resetAchievements();
    seed([crownRow()]);
    render(NotificationBell);
    await open();

    expect(await screen.findByText(/took a crown/)).toBeInTheDocument();
  });
});

// The panel's half of the dialog: opening it, filling it, and marking the row
// read for having looked. What the dialog then DRAWS is AchievementDialog's own
// test — this one stops at the seam.
describe("opening a crown", () => {
  beforeEach(() => {
    achievements.items = [testAchievementHolders()];
    achievements.loaded = true;
  });
  afterEach(() => resetAchievements());

  /** Open the panel and click the crown row. */
  async function openCrown() {
    render(NotificationBell);
    await open();
    await fireEvent.click(await screen.findByText(/took the crown/));
  }

  it("asks for the row's members and shows them", async () => {
    seed([crownRow()]);
    await openCrown();

    await waitFor(() => expect(getNotificationGroupMembers).toHaveBeenCalledWith(90));
    expect(await screen.findByText(/Grace Hopper/)).toBeInTheDocument();
  });

  // Opening this IS reading it — the same call following a row makes, and the
  // group it names is everything the row folded.
  it("marks the group read", async () => {
    seed([crownRow()], 1);
    await openCrown();

    await waitFor(() => expect(markNotificationRead).toHaveBeenCalledWith(90));
  });

  // The tap has to be answered immediately. A modal that appears a request later
  // reads as a dead row, so the dialog opens first and fills in.
  it("opens before the members have landed", async () => {
    let release: (v: unknown) => void = () => {};
    getNotificationGroupMembers.mockReturnValue(
      new Promise((resolve) => {
        release = resolve;
      }),
    );
    seed([crownRow()]);
    await openCrown();

    expect(await screen.findByTestId("achievement-dialog")).toBeInTheDocument();

    release({ status: 200, data: { items: [crownRow()] } });
    await waitFor(() => expect(screen.getByText(/Grace Hopper/)).toBeInTheDocument());
  });

  // A failure leaves the dialog standing and saying so. Raising a banner over a
  // panel somebody may be reading mid-set is the wrong trade for an ornament.
  it("survives a members request that fails", async () => {
    getNotificationGroupMembers.mockResolvedValue({ status: 500, data: {} });
    seed([crownRow()]);
    await openCrown();

    expect(await screen.findByTestId("achievement-dialog")).toBeInTheDocument();
    expect(await screen.findByText(/isn't here any more/)).toBeInTheDocument();
  });

  it("goes to the standings from inside the dialog", async () => {
    seed([crownRow()]);
    await openCrown();

    await fireEvent.click(await screen.findByRole("button", { name: "Leaderboard" }));
    await waitFor(() => expect(push).toHaveBeenCalledWith("/leaderboard"));
  });
});

describe("the two buttons", () => {
  it("marks all read without emptying the list", async () => {
    seed([testNotification()], 1);
    render(NotificationBell);
    await open();
    // Let the refresh that opening triggers settle on the unread state first;
    // swapping the answer before it lands would disable the button under us.
    await waitFor(() => expect(listNotifications).toHaveBeenCalled());

    // From here the server reports them stamped, which is what the reconciling
    // poll after the tap will find.
    served([testNotification({ readAt: "2026-03-17T11:00:00Z" })], 0);

    await fireEvent.click(
      await screen.findByRole("button", { name: /mark all read/i }),
    );

    await waitFor(() => expect(markNotificationsRead).toHaveBeenCalled());
    expect(clearNotifications).not.toHaveBeenCalled();
    // The rows stay — that is what makes this different from clearing.
    await waitFor(() => expect(notifications.items).toHaveLength(1));
    expect(notifications.unread).toBe(0);
  });

  it("clears the list", async () => {
    seed([testNotification()], 1);
    render(NotificationBell);
    await open();
    await waitFor(() => expect(listNotifications).toHaveBeenCalled());

    // An emptied panel is what the poll after the tap should find.
    served([], 0);

    await fireEvent.click(
      await screen.findByRole("button", { name: /clear all/i }),
    );

    await waitFor(() => expect(clearNotifications).toHaveBeenCalled());
    expect(markNotificationsRead).not.toHaveBeenCalled();
    await waitFor(() => expect(notifications.items).toHaveLength(0));
  });

  it("disables each when it would do nothing", async () => {
    notifications.loaded = true;
    render(NotificationBell);
    await open();

    // Nothing unread and nothing listed.
    expect(
      await screen.findByRole("button", { name: /mark all read/i }),
    ).toBeDisabled();
    expect(screen.getByRole("button", { name: /clear all/i })).toBeDisabled();
  });
});

describe("polling", () => {
  it("keeps the last good list when a poll fails", async () => {
    notifications.items = [testNotification()];
    notifications.unread = 1;
    notifications.loaded = true;

    listNotifications.mockResolvedValue({ status: 0, data: undefined });
    await poll();

    // A notification nobody could fetch is not a reason to blank the panel:
    // the header is on every screen, including one somebody is mid-set on.
    expect(notifications.items).toHaveLength(1);
    expect(notifications.unread).toBe(1);
    expect(notifications.failed).toBe(true);
  });
});

describe("following a notification", () => {
  // Reading THIS one, which is what following it means. The blunt "mark all
  // read" buries the rows that have not been looked at, so before this the
  // badge could only be cleared by losing track of what was new.
  it("marks the row read and drops the badge by one", async () => {
    seed(
      [
        testNotification({
          id: 11,
          kind: "reaction",
          sessionId: 42,
          sessionOwnerId: ME,
        }),
        testNotification({ id: 12, kind: "joined" }),
      ],
      2,
    );
    render(NotificationBell);
    await open();

    await fireEvent.click(await screen.findByText(/applauded your/i));

    expect(markNotificationRead).toHaveBeenCalledWith(11);
    await waitFor(() => expect(notifications.unread).toBe(1));
  });

  // A row already read costs no request — otherwise opening the same
  // notification twice would take the badge below what is actually unread.
  it("asks nothing for a row that was already read", async () => {
    seed(
      [
        testNotification({
          id: 11,
          kind: "reaction",
          sessionId: 42,
          sessionOwnerId: ME,
          readAt: "2026-03-17T18:00:00Z",
        }),
      ],
      0,
    );
    render(NotificationBell);
    await open();

    await fireEvent.click(await screen.findByText(/applauded your/i));

    expect(markNotificationRead).not.toHaveBeenCalled();
    expect(notifications.unread).toBe(0);
  });

  // The deep link. Without it the lifter lands at the top of a long recap whose
  // conversation is the last card, hunting for a sentence they were just shown.
  it("points a comment at the comment, not just the page", async () => {
    seed([
      testNotification({
        id: 13,
        kind: "comment",
        sessionId: 42,
        sessionOwnerId: ME,
        commentId: 99,
        commentBody: "strong sets",
      }),
    ]);
    render(NotificationBell);
    await open();

    await fireEvent.click(await screen.findByText(/commented on your/i));

    expect(push).toHaveBeenCalledWith("/sessions/42/recap?comment=99");
  });

  // A reply reaches you about somebody else's workout, so it routes to the
  // owner-scoped recap — and carries the anchor there too.
  it("routes a reply to the session's owner with the anchor", async () => {
    seed([
      testNotification({
        id: 14,
        kind: "reply",
        sessionId: 42,
        sessionOwnerId: THEM,
        commentId: 7,
        commentBody: "agreed",
      }),
    ]);
    render(NotificationBell);
    await open();

    await fireEvent.click(await screen.findByText(/also replied/i));

    expect(push).toHaveBeenCalledWith("/lifters/2/sessions/42/recap?comment=7");
  });

  // A reaction has no comment to point at, so the link stays bare rather than
  // carrying a parameter the recap would have to know to ignore.
  it("leaves a reaction's link without an anchor", async () => {
    seed([
      testNotification({
        id: 15,
        kind: "reaction",
        sessionId: 42,
        sessionOwnerId: ME,
      }),
    ]);
    render(NotificationBell);
    await open();

    await fireEvent.click(await screen.findByText(/applauded your/i));

    expect(push).toHaveBeenCalledWith("/sessions/42/recap");
  });

  // A reply about a session with no owner — one predating accounts — has no
  // recap route to build. Such a row used to look identical to a live one and
  // simply do nothing when tapped, which reads as a broken panel.
  it("draws a row that leads nowhere as inert", async () => {
    seed([
      testNotification({
        id: 16,
        kind: "reply",
        sessionId: 42,
        sessionOwnerId: undefined,
        commentBody: "orphaned",
      }),
    ]);
    render(NotificationBell);
    await open();

    const row = await screen.findByText(/also replied/i);
    await fireEvent.click(row);

    expect(push).not.toHaveBeenCalled();
    expect(markNotificationRead).not.toHaveBeenCalled();
  });
});
