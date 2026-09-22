import { render, screen, waitFor, fireEvent } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import NotificationBell from "./NotificationBell.svelte";
import { auth } from "./auth.svelte";
import { notifications, poll } from "./notifications.svelte";
import { testLifter, testNotification, testUser } from "./testFixtures";

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
const clearNotifications = vi.hoisted(() => vi.fn());
vi.mock("./api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./api")>()),
  listNotifications,
  markNotificationsRead,
  clearNotifications,
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
  clearNotifications.mockResolvedValue({ status: 204, data: undefined });
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

/** Open the panel and wait for it to draw. */
async function open() {
  await fireEvent.click(screen.getByRole("button", { name: /notifications/i }));
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
