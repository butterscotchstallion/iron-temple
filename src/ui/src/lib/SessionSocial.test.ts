import { render, screen, waitFor, fireEvent } from "@testing-library/svelte";
import { beforeEach, describe, expect, it, vi } from "vitest";
import SessionSocial from "./SessionSocial.svelte";
import { auth } from "./auth.svelte";
import { testLifter, testUser } from "./testFixtures";
import type { SessionComment, SessionReaction } from "./api";

// Applause and conversation under a recap.
//
// The branch worth most here is the own-session one: the API refuses a reaction on
// your own workout, so the buttons must not be offered — a control that can only
// ever 403 is worse than no control — while the counts still show, because seeing
// who applauded your session is the point of having it.

const listSessionReactions = vi.hoisted(() => vi.fn());
const listSessionComments = vi.hoisted(() => vi.fn());
const addSessionReaction = vi.hoisted(() => vi.fn());
const removeSessionReaction = vi.hoisted(() => vi.fn());
const addSessionComment = vi.hoisted(() => vi.fn());
const deleteSessionComment = vi.hoisted(() => vi.fn());
vi.mock("./api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./api")>()),
  listSessionReactions,
  listSessionComments,
  addSessionReaction,
  removeSessionReaction,
  addSessionComment,
  deleteSessionComment,
}));

// The live socket, faked at the module boundary: this card's job is to
// subscribe and to refetch what it is told to, and both are observable here
// without a WebSocket anywhere in sight.
const stopWatching = vi.hoisted(() => vi.fn());
const watchSession = vi.hoisted(() => vi.fn());
vi.mock("./live.svelte", () => ({ watchSession }));

/** Deliver an event to whatever the card subscribed with. */
function emitSessionEvent(kind: "reaction" | "comment" | "resync") {
  const handler = watchSession.mock.calls.at(-1)?.[1] as
    | ((event: string) => void)
    | undefined;
  handler?.(kind);
}

const ME = 1;
const THEM = 2;

function reaction(over: Partial<SessionReaction> = {}): SessionReaction {
  return { emoji: "💪", count: 1, mine: false, ...over };
}

function comment(over: Partial<SessionComment> = {}): SessionComment {
  return {
    id: 10,
    sessionId: 42,
    author: testLifter({ id: THEM, displayName: "Grace Hopper" }),
    body: "strong session",
    createdAt: "2026-03-17T18:00:00Z",
    ...over,
  };
}

/**
 * The comments endpoint answers with a PAGE plus the thread's total, not a bare
 * array — `total` is what lets the card offer "show N earlier comments".
 *
 * `total` defaults to what was passed, which is the ordinary case: everything
 * fits on one page and there is nothing above it. A test about paging passes a
 * larger one on purpose.
 */
function commentsPage(items: unknown[], total = items.length) {
  return { status: 200, data: { items, total, limit: 20, offset: 0 } };
}

beforeEach(() => {
  vi.clearAllMocks();
  watchSession.mockReturnValue(stopWatching);
  listSessionReactions.mockResolvedValue({ status: 200, data: [] });
  listSessionComments.mockResolvedValue(commentsPage([]));
  addSessionReaction.mockResolvedValue({ status: 204, data: undefined });
  removeSessionReaction.mockResolvedValue({ status: 204, data: undefined });
  deleteSessionComment.mockResolvedValue({ status: 204, data: undefined });
  auth.me = testUser({ id: ME, isAdmin: false });
  auth.loaded = true;
});

describe("SessionSocial on somebody else's session", () => {
  const props = { sessionId: 42, ownerId: THEM };

  it("offers every emoji the contract lists", async () => {
    render(SessionSocial, props);

    await waitFor(() => {
      expect(screen.getByLabelText("React with 💪")).toBeInTheDocument();
    });
    for (const emoji of ["💪", "🔥", "👏", "🎉"]) {
      expect(screen.getByLabelText(`React with ${emoji}`)).toBeInTheDocument();
    }
  });

  it("applauds, then refetches rather than guessing the new count", async () => {
    render(SessionSocial, props);
    // Waiting for the button rather than for the call: the card holds its
    // skeleton until both requests have ANSWERED, and "was called" is a promise
    // that has not necessarily settled.
    await waitFor(() => {
      expect(screen.getByLabelText("React with 💪")).toBeInTheDocument();
    });
    expect(listSessionReactions).toHaveBeenCalledTimes(1);

    listSessionReactions.mockResolvedValue({
      status: 200,
      data: [reaction({ count: 1, mine: true })],
    });
    await fireEvent.click(screen.getByLabelText("React with 💪"));

    await waitFor(() => {
      expect(addSessionReaction).toHaveBeenCalledWith(42, { emoji: "💪" });
    });
    // The server owns the count — other lifters may have reacted since this
    // loaded, so the number comes from a refetch and not from arithmetic here.
    expect(listSessionReactions).toHaveBeenCalledTimes(2);
    await waitFor(() => {
      expect(screen.getByLabelText("React with 💪")).toHaveAttribute("aria-pressed", "true");
    });
  });

  it("withdraws applause it already gave", async () => {
    listSessionReactions.mockResolvedValue({
      status: 200,
      data: [reaction({ mine: true })],
    });
    render(SessionSocial, props);

    await waitFor(() => {
      expect(screen.getByLabelText("React with 💪")).toHaveAttribute("aria-pressed", "true");
    });
    await fireEvent.click(screen.getByLabelText("React with 💪"));

    await waitFor(() => {
      expect(removeSessionReaction).toHaveBeenCalledWith(42, { emoji: "💪" });
    });
    expect(addSessionReaction).not.toHaveBeenCalled();
  });

  it("reports a failed reaction rather than showing one that did not save", async () => {
    addSessionReaction.mockResolvedValue({ status: 500, data: undefined });
    render(SessionSocial, props);

    await waitFor(() => {
      expect(screen.getByLabelText("React with 🔥")).toBeInTheDocument();
    });
    await fireEvent.click(screen.getByLabelText("React with 🔥"));

    await waitFor(() => {
      expect(screen.getByText("Couldn't save that.")).toBeInTheDocument();
    });
    expect(screen.getByLabelText("React with 🔥")).toHaveAttribute("aria-pressed", "false");
  });
});

describe("SessionSocial on your own session", () => {
  const props = { sessionId: 42, ownerId: ME };

  // The API refuses this, so the buttons are not offered.
  it("does not offer reaction buttons", async () => {
    listSessionReactions.mockResolvedValue({ status: 200, data: [reaction({ count: 3 })] });
    render(SessionSocial, props);

    await waitFor(() => {
      expect(screen.getByText("3")).toBeInTheDocument();
    });
    expect(screen.queryByLabelText("React with 💪")).not.toBeInTheDocument();
  });

  it("still shows the applause it drew", async () => {
    listSessionReactions.mockResolvedValue({
      status: 200,
      data: [reaction({ emoji: "🎉", count: 2 })],
    });
    render(SessionSocial, props);

    await waitFor(() => {
      expect(screen.getByText("2")).toBeInTheDocument();
    });
  });

  it("says so when nobody has reacted", async () => {
    render(SessionSocial, props);
    await waitFor(() => {
      expect(screen.getByText("No reactions yet.")).toBeInTheDocument();
    });
  });

  // Comments are allowed on your own session, unlike reactions: you may need to
  // answer somebody.
  it("still lets you comment", async () => {
    addSessionComment.mockResolvedValue({ status: 201, data: comment({ id: 11 }) });
    render(SessionSocial, props);

    await waitFor(() => expect(listSessionComments).toHaveBeenCalled());
    await fireEvent.input(screen.getByPlaceholderText("Say something"), {
      target: { value: "felt easier than it looked" },
    });
    await fireEvent.click(screen.getByText("Post"));

    await waitFor(() => {
      expect(addSessionComment).toHaveBeenCalledWith(42, {
        body: "felt easier than it looked",
      });
    });
  });
});

describe("SessionSocial comments", () => {
  const props = { sessionId: 42, ownerId: THEM };

  it("lists them with their authors", async () => {
    listSessionComments.mockResolvedValue(commentsPage([comment()]));
    render(SessionSocial, props);

    await waitFor(() => {
      expect(screen.getByText("strong session")).toBeInTheDocument();
    });
    expect(screen.getByText("Grace Hopper")).toBeInTheDocument();
  });

  it("trims the body and appends the posted row", async () => {
    addSessionComment.mockResolvedValue({
      status: 201,
      data: comment({ id: 12, body: "nice work" }),
    });
    render(SessionSocial, props);

    await waitFor(() => expect(listSessionComments).toHaveBeenCalled());
    await fireEvent.input(screen.getByPlaceholderText("Say something"), {
      target: { value: "  nice work  " },
    });
    await fireEvent.click(screen.getByText("Post"));

    await waitFor(() => {
      expect(addSessionComment).toHaveBeenCalledWith(42, { body: "nice work" });
    });
    // Appended from the response rather than refetched: the list is oldest-first,
    // so a new comment belongs at the end.
    await waitFor(() => {
      expect(screen.getByText("nice work")).toBeInTheDocument();
    });
    expect(listSessionComments).toHaveBeenCalledTimes(1);
  });

  it("will not post a blank comment", async () => {
    render(SessionSocial, props);

    await waitFor(() => expect(listSessionComments).toHaveBeenCalled());
    await fireEvent.input(screen.getByPlaceholderText("Say something"), {
      target: { value: "   " },
    });
    expect(screen.getByText("Post")).toBeDisabled();

    await fireEvent.click(screen.getByText("Post"));
    expect(addSessionComment).not.toHaveBeenCalled();
  });

  // The server names the reason — too long, blank — which is more use than a
  // generic failure.
  it("shows the server's reason when a post is refused", async () => {
    addSessionComment.mockResolvedValue({
      status: 400,
      data: { code: "bad_request", message: "comment must be at most 256 characters" },
    });
    render(SessionSocial, props);

    await waitFor(() => expect(listSessionComments).toHaveBeenCalled());
    await fireEvent.input(screen.getByPlaceholderText("Say something"), {
      target: { value: "too long" },
    });
    await fireEvent.click(screen.getByText("Post"));

    await waitFor(() => {
      expect(
        screen.getByText("comment must be at most 256 characters"),
      ).toBeInTheDocument();
    });
  });

  it("offers a remove button on your own comment only", async () => {
    listSessionComments.mockResolvedValue(commentsPage([
        comment({ id: 1, author: testLifter({ id: ME }), body: "mine" }),
        comment({ id: 2, author: testLifter({ id: THEM }), body: "theirs" }),
      ]));
    render(SessionSocial, props);

    await waitFor(() => {
      expect(screen.getByText("mine")).toBeInTheDocument();
    });
    expect(screen.getAllByLabelText("Remove this comment")).toHaveLength(1);
  });

  // The install's owner may remove any. This is the only moderation the app has.
  it("offers remove on every comment to the install's owner", async () => {
    auth.me = testUser({ id: ME, isAdmin: true });
    listSessionComments.mockResolvedValue(commentsPage([
        comment({ id: 1, author: testLifter({ id: THEM }), body: "theirs" }),
        comment({ id: 2, author: testLifter({ id: 3 }), body: "somebody else's" }),
      ]));
    render(SessionSocial, props);

    await waitFor(() => {
      expect(screen.getByText("theirs")).toBeInTheDocument();
    });
    expect(screen.getAllByLabelText("Remove this comment")).toHaveLength(2);
  });

  it("drops a removed comment from the list", async () => {
    listSessionComments.mockResolvedValue(commentsPage([comment({ id: 7, author: testLifter({ id: ME }), body: "regrettable" })]));
    render(SessionSocial, props);

    await waitFor(() => {
      expect(screen.getByText("regrettable")).toBeInTheDocument();
    });
    await fireEvent.click(screen.getByLabelText("Remove this comment"));

    await waitFor(() => {
      expect(screen.queryByText("regrettable")).not.toBeInTheDocument();
    });
    expect(deleteSessionComment).toHaveBeenCalledWith(42, 7);
  });

  it("keeps the comment when removing it fails", async () => {
    deleteSessionComment.mockResolvedValue({ status: 500, data: undefined });
    listSessionComments.mockResolvedValue(commentsPage([comment({ id: 7, author: testLifter({ id: ME }), body: "still here" })]));
    render(SessionSocial, props);

    await waitFor(() => {
      expect(screen.getByText("still here")).toBeInTheDocument();
    });
    await fireEvent.click(screen.getByLabelText("Remove this comment"));

    await waitFor(() => {
      expect(screen.getByText("Couldn't remove that.")).toBeInTheDocument();
    });
    expect(screen.getByText("still here")).toBeInTheDocument();
  });
});

describe("SessionSocial while it loads", () => {
  const props = { sessionId: 42, ownerId: THEM };

  // The card used to draw immediately with both lists empty and then grow as
  // each landed, pushing the comment box down the page a beat after the lifter
  // had reached for it.
  it("holds the card's height until both requests have answered", async () => {
    let answerReactions!: (result: unknown) => void;
    listSessionReactions.mockReturnValue(
      new Promise((resolve) => (answerReactions = resolve)),
    );
    render(SessionSocial, props);

    // The comments call has already resolved; the card waits for the other one.
    await waitFor(() => {
      expect(screen.getByRole("status")).toHaveTextContent(
        "Loading applause and comments…",
      );
    });
    expect(screen.queryByLabelText("React with 💪")).toBeNull();

    answerReactions({ status: 200, data: [] });
    await waitFor(() => {
      expect(screen.getByLabelText("React with 💪")).toBeInTheDocument();
    });
  });

  // A shimmer that never resolves would be a worse lie than a zero.
  it("gives up the placeholder when the requests fail", async () => {
    listSessionReactions.mockResolvedValue({ status: 500, data: undefined });
    listSessionComments.mockResolvedValue({ status: 500, data: undefined });
    render(SessionSocial, props);

    await waitFor(() => {
      expect(screen.getByLabelText("React with 💪")).toBeInTheDocument();
    });
  });

  // Posting needs nothing either request returns, so withholding the box would
  // only mean it moved when they landed.
  it("leaves the comment box usable throughout", () => {
    listSessionReactions.mockReturnValue(new Promise(() => {}));
    listSessionComments.mockReturnValue(new Promise(() => {}));
    render(SessionSocial, props);

    expect(screen.getByPlaceholderText("Say something")).toBeInTheDocument();
  });
});

describe("SessionSocial while a write is in flight", () => {
  const props = { sessionId: 42, ownerId: THEM };

  // Applauding costs two round trips — the write, then the refetch that owns the
  // count — so on anything slower than a LAN the button used to look dead, and
  // the second tap withdrew the first.
  it("holds the reaction buttons until the refetched count lands", async () => {
    let finishWrite!: (result: unknown) => void;
    addSessionReaction.mockReturnValue(new Promise((resolve) => (finishWrite = resolve)));
    render(SessionSocial, props);

    await waitFor(() => {
      expect(screen.getByLabelText("React with 💪")).toBeInTheDocument();
    });
    await fireEvent.click(screen.getByLabelText("React with 💪"));

    await waitFor(() => {
      expect(screen.getByLabelText("Saving 💪")).toHaveAttribute("aria-busy", "true");
    });
    // Every button, not just the pressed one: they all read from a count that is
    // about to be replaced wholesale.
    expect(screen.getByLabelText("React with 🔥")).toBeDisabled();

    finishWrite({ status: 204, data: undefined });
    await waitFor(() => {
      expect(screen.getByLabelText("React with 💪")).toBeEnabled();
    });
    expect(addSessionReaction).toHaveBeenCalledTimes(1);
  });

  it("releases the buttons when the write fails", async () => {
    addSessionReaction.mockResolvedValue({ status: 500, data: undefined });
    render(SessionSocial, props);

    await waitFor(() => {
      expect(screen.getByLabelText("React with 💪")).toBeInTheDocument();
    });
    await fireEvent.click(screen.getByLabelText("React with 💪"));

    await waitFor(() => {
      expect(screen.getByText("Couldn't save that.")).toBeInTheDocument();
    });
    expect(screen.getByLabelText("React with 💪")).toBeEnabled();
  });

  it("does not send a second delete for a comment already going", async () => {
    let finishDelete!: (result: unknown) => void;
    deleteSessionComment.mockReturnValue(
      new Promise((resolve) => (finishDelete = resolve)),
    );
    listSessionComments.mockResolvedValue(commentsPage([comment({ id: 7, author: testLifter({ id: ME }), body: "regrettable" })]));
    render(SessionSocial, props);

    await waitFor(() => {
      expect(screen.getByText("regrettable")).toBeInTheDocument();
    });
    const trash = screen.getByLabelText("Remove this comment");
    await fireEvent.click(trash);
    await waitFor(() => expect(trash).toHaveAttribute("aria-busy", "true"));
    await fireEvent.click(trash);

    finishDelete({ status: 204, data: undefined });
    await waitFor(() => {
      expect(screen.queryByText("regrettable")).not.toBeInTheDocument();
    });
    expect(deleteSessionComment).toHaveBeenCalledTimes(1);
  });
});

describe("SessionSocial paging a long conversation", () => {
  const props = { sessionId: 42, ownerId: THEM };

  // The endpoint pages from the NEWEST end, so the first page is the tail of
  // the thread — the part somebody opening a session wants, and the part a
  // notification points at. The control walks upwards into the older part.
  it("offers the earlier comments when there are more than a page", async () => {
    listSessionComments.mockResolvedValue(
      commentsPage([comment({ id: 20, body: "most recent" })], 4),
    );
    render(SessionSocial, props);

    await waitFor(() => {
      expect(screen.getByText("most recent")).toBeInTheDocument();
    });
    expect(screen.getByRole("button", { name: /3 earlier comments/i })).toBeInTheDocument();
  });

  it("says it in the singular for exactly one", async () => {
    listSessionComments.mockResolvedValue(
      commentsPage([comment({ id: 20, body: "most recent" })], 2),
    );
    render(SessionSocial, props);

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: /1 earlier comment$/i }),
      ).toBeInTheDocument();
    });
  });

  it("stays quiet when the whole thread is already on screen", async () => {
    listSessionComments.mockResolvedValue(commentsPage([comment({ body: "all of it" })]));
    render(SessionSocial, props);

    await waitFor(() => {
      expect(screen.getByText("all of it")).toBeInTheDocument();
    });
    expect(screen.queryByRole("button", { name: /earlier comment/i })).toBeNull();
  });

  // Prepended, not appended: what comes back is OLDER than what is on screen,
  // and the list reads downwards.
  it("puts the earlier page above the one already held", async () => {
    listSessionComments.mockResolvedValue(
      commentsPage([comment({ id: 20, body: "newest" })], 2),
    );
    render(SessionSocial, props);
    await waitFor(() => expect(screen.getByText("newest")).toBeInTheDocument());

    listSessionComments.mockResolvedValue(
      commentsPage([comment({ id: 10, body: "older" })], 2),
    );
    await fireEvent.click(screen.getByRole("button", { name: /1 earlier comment/i }));

    await waitFor(() => expect(screen.getByText("older")).toBeInTheDocument());
    // Offset is how many are already held, which is how far back from the
    // newest the next page starts.
    expect(listSessionComments).toHaveBeenLastCalledWith(42, { limit: 20, offset: 1 });

    const bodies = screen
      .getAllByText(/newest|older/)
      .map((node) => node.textContent);
    expect(bodies).toEqual(["older", "newest"]);
  });
});

describe("SessionSocial arriving from a notification", () => {
  const props = { sessionId: 42, ownerId: THEM };

  beforeEach(() => {
    // jsdom implements no layout, so this is not there to be called for real —
    // only to be observed.
    Element.prototype.scrollIntoView = vi.fn();
  });

  it("brings the comment it was sent to into view", async () => {
    listSessionComments.mockResolvedValue(
      commentsPage([
        comment({ id: 1, body: "first" }),
        comment({ id: 2, body: "the one" }),
      ]),
    );
    render(SessionSocial, { ...props, highlightCommentId: 2 });

    await waitFor(() => expect(screen.getByText("the one")).toBeInTheDocument());
    await waitFor(() => expect(Element.prototype.scrollIntoView).toHaveBeenCalled());
  });

  // Above the first page, so the card has to walk back through the thread
  // before it can point at anything.
  it("pages back to find a comment older than the first page", async () => {
    listSessionComments.mockResolvedValueOnce(
      commentsPage([comment({ id: 9, body: "recent" })], 2),
    );
    listSessionComments.mockResolvedValueOnce(
      commentsPage([comment({ id: 3, body: "buried" })], 2),
    );
    render(SessionSocial, { ...props, highlightCommentId: 3 });

    await waitFor(() => expect(screen.getByText("buried")).toBeInTheDocument());
    expect(listSessionComments).toHaveBeenCalledTimes(2);
  });

  // A comment deleted since the notification was raised simply is not there.
  // The lifter still gets the conversation rather than a spinner or an error.
  it("gives up quietly when the comment is gone", async () => {
    listSessionComments.mockResolvedValue(
      commentsPage([comment({ id: 1, body: "still here" })]),
    );
    render(SessionSocial, { ...props, highlightCommentId: 404 });

    await waitFor(() => expect(screen.getByText("still here")).toBeInTheDocument());
    expect(Element.prototype.scrollIntoView).not.toHaveBeenCalled();
  });

  // The ordinary visit, which is most of them.
  it("scrolls nowhere when no comment was named", async () => {
    listSessionComments.mockResolvedValue(
      commentsPage([comment({ id: 1, body: "ordinary" })]),
    );
    render(SessionSocial, props);

    await waitFor(() => expect(screen.getByText("ordinary")).toBeInTheDocument());
    expect(Element.prototype.scrollIntoView).not.toHaveBeenCalled();
  });
});

describe("SessionSocial while somebody else is looking at the same session", () => {
  const props = { sessionId: 42, ownerId: THEM };

  it("watches its own session and gives the subscription back", async () => {
    const { unmount } = render(SessionSocial, props);
    await waitFor(() => expect(watchSession).toHaveBeenCalledWith(42, expect.any(Function)));

    unmount();
    expect(stopWatching).toHaveBeenCalled();
  });

  // Split by kind so a run of applause does not refetch the conversation.
  it("refetches only the reactions when applause arrives", async () => {
    render(SessionSocial, props);
    await waitFor(() => expect(listSessionComments).toHaveBeenCalled());
    listSessionReactions.mockClear();
    listSessionComments.mockClear();

    emitSessionEvent("reaction");

    await waitFor(() => expect(listSessionReactions).toHaveBeenCalledTimes(1));
    expect(listSessionComments).not.toHaveBeenCalled();
  });

  it("refetches only the comments when one arrives", async () => {
    render(SessionSocial, props);
    await waitFor(() => expect(listSessionComments).toHaveBeenCalled());
    listSessionReactions.mockClear();
    listSessionComments.mockClear();

    emitSessionEvent("comment");

    await waitFor(() => expect(listSessionComments).toHaveBeenCalledTimes(1));
    expect(listSessionReactions).not.toHaveBeenCalled();
  });

  // A gap in the connection is a gap in both lists.
  it("refetches both on a resync", async () => {
    render(SessionSocial, props);
    await waitFor(() => expect(listSessionComments).toHaveBeenCalled());
    listSessionReactions.mockClear();
    listSessionComments.mockClear();

    emitSessionEvent("resync");

    await waitFor(() => expect(listSessionReactions).toHaveBeenCalledTimes(1));
    expect(listSessionComments).toHaveBeenCalledTimes(1);
  });

  // A live update must not throw away the earlier pages somebody walked back
  // through, so the refetch asks for as much as is held rather than one page.
  it("keeps the earlier comments it had already paged in", async () => {
    listSessionComments.mockResolvedValue(
      commentsPage([comment({ id: 20, body: "newest" })], 2),
    );
    render(SessionSocial, props);
    await waitFor(() => expect(screen.getByText("newest")).toBeInTheDocument());

    listSessionComments.mockResolvedValue(
      commentsPage([comment({ id: 10, body: "older" })], 2),
    );
    await fireEvent.click(screen.getByRole("button", { name: /1 earlier comment/i }));
    await waitFor(() => expect(screen.getByText("older")).toBeInTheDocument());

    listSessionComments.mockClear();
    listSessionComments.mockResolvedValue(
      commentsPage(
        [comment({ id: 10, body: "older" }), comment({ id: 20, body: "newest" })],
        2,
      ),
    );
    emitSessionEvent("comment");

    await waitFor(() =>
      expect(listSessionComments).toHaveBeenCalledWith(42, { limit: 20 }),
    );
    expect(screen.getByText("older")).toBeInTheDocument();
  });
});
