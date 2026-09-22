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

beforeEach(() => {
  vi.clearAllMocks();
  listSessionReactions.mockResolvedValue({ status: 200, data: [] });
  listSessionComments.mockResolvedValue({ status: 200, data: [] });
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
    await waitFor(() => expect(listSessionReactions).toHaveBeenCalledTimes(1));

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

    await waitFor(() => expect(listSessionReactions).toHaveBeenCalled());
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
    listSessionComments.mockResolvedValue({ status: 200, data: [comment()] });
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
    listSessionComments.mockResolvedValue({
      status: 200,
      data: [
        comment({ id: 1, author: testLifter({ id: ME }), body: "mine" }),
        comment({ id: 2, author: testLifter({ id: THEM }), body: "theirs" }),
      ],
    });
    render(SessionSocial, props);

    await waitFor(() => {
      expect(screen.getByText("mine")).toBeInTheDocument();
    });
    expect(screen.getAllByLabelText("Remove this comment")).toHaveLength(1);
  });

  // The install's owner may remove any. This is the only moderation the app has.
  it("offers remove on every comment to the install's owner", async () => {
    auth.me = testUser({ id: ME, isAdmin: true });
    listSessionComments.mockResolvedValue({
      status: 200,
      data: [
        comment({ id: 1, author: testLifter({ id: THEM }), body: "theirs" }),
        comment({ id: 2, author: testLifter({ id: 3 }), body: "somebody else's" }),
      ],
    });
    render(SessionSocial, props);

    await waitFor(() => {
      expect(screen.getByText("theirs")).toBeInTheDocument();
    });
    expect(screen.getAllByLabelText("Remove this comment")).toHaveLength(2);
  });

  it("drops a removed comment from the list", async () => {
    listSessionComments.mockResolvedValue({
      status: 200,
      data: [comment({ id: 7, author: testLifter({ id: ME }), body: "regrettable" })],
    });
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
    listSessionComments.mockResolvedValue({
      status: 200,
      data: [comment({ id: 7, author: testLifter({ id: ME }), body: "still here" })],
    });
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
