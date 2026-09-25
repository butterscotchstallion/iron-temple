import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/svelte";
import FollowButton from "./FollowButton.svelte";
import { testLifter } from "./testFixtures";

// The Follow control, shared by the roster and the profile.
//
// Two things here are worth more than the rest. It has to be a genuine two-state
// toggle to a screen reader — `aria-pressed` and a label that says which direction
// the press goes — because "Follow" and "Following" look obviously different and
// sound identical if the only difference is a colour. And it must not be pressable
// twice while the first press is in flight, which `disabled` alone does not
// guarantee.

const followLifter = vi.hoisted(() => vi.fn());
const unfollowLifter = vi.hoisted(() => vi.fn());
vi.mock("./api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./api")>()),
  followLifter,
  unfollowLifter,
}));

const GRACE = testLifter({ id: 2, displayName: "Grace Hopper" });

function show(following = false, onChange = vi.fn()) {
  render(FollowButton, { props: { lifter: GRACE, following, onChange } });
  return onChange;
}

beforeEach(() => {
  followLifter.mockReset().mockResolvedValue({ status: 204, data: undefined });
  unfollowLifter.mockReset().mockResolvedValue({ status: 204, data: undefined });
});

describe("what it says", () => {
  it("offers to follow somebody it does not follow", () => {
    show(false);
    const button = screen.getByRole("button", { name: "Follow Grace Hopper" });
    expect(button).toHaveAttribute("aria-pressed", "false");
    expect(button).toHaveTextContent("Follow");
  });

  it("offers to unfollow somebody it does", () => {
    show(true);
    const button = screen.getByRole("button", { name: "Unfollow Grace Hopper" });
    expect(button).toHaveAttribute("aria-pressed", "true");
    expect(button).toHaveTextContent("Following");
  });

  // An account created through the admin area has no display name until its owner
  // sets one, and a label reading "Follow " is worse than a username.
  it("falls back to the username in its label", () => {
    render(FollowButton, {
      props: {
        lifter: testLifter({ id: 3, displayName: "", username: "ada" }),
        following: false,
        onChange: vi.fn(),
      },
    });
    expect(screen.getByRole("button", { name: "Follow ada" })).toBeInTheDocument();
  });
});

describe("pressing it", () => {
  it("follows, and reports the new state", async () => {
    const onChange = show(false);

    await fireEvent.click(screen.getByRole("button", { name: /^Follow/ }));

    await waitFor(() => expect(followLifter).toHaveBeenCalledWith(2));
    expect(onChange).toHaveBeenCalledWith(true);
    expect(unfollowLifter).not.toHaveBeenCalled();
  });

  it("unfollows, and reports the new state", async () => {
    const onChange = show(true);

    await fireEvent.click(screen.getByRole("button", { name: /^Unfollow/ }));

    await waitFor(() => expect(unfollowLifter).toHaveBeenCalledWith(2));
    expect(onChange).toHaveBeenCalledWith(false);
    expect(followLifter).not.toHaveBeenCalled();
  });

  // It does NOT refetch. SessionSocial's reaction toggle deliberately does, because
  // the count beside it belongs to the server; a follow is a fact about the caller
  // that nobody else can change, so the state after a 204 is known exactly.
  it("asks the server once", async () => {
    show(false);
    await fireEvent.click(screen.getByRole("button", { name: /^Follow/ }));
    await waitFor(() => expect(followLifter).toHaveBeenCalledTimes(1));
  });
});

describe("while it is saving", () => {
  /** A request that will not resolve until released. */
  function pending() {
    let release: (v: unknown) => void = () => {};
    followLifter.mockReturnValue(
      new Promise((resolve) => {
        release = resolve;
      }),
    );
    return () => release({ status: 204, data: undefined });
  }

  it("says so, to a screen reader as well as by going dim", async () => {
    const release = pending();
    show(false);

    await fireEvent.click(screen.getByRole("button", { name: /^Follow/ }));

    const button = await screen.findByRole("button", { name: "Saving" });
    expect(button).toHaveAttribute("aria-busy", "true");
    expect(button).toBeDisabled();

    release();
  });

  // A re-entrancy guard rather than only `disabled`, which is a property of the
  // rendered element and says nothing about a handler already in flight.
  it("cannot be pressed twice", async () => {
    const release = pending();
    const onChange = show(false);

    const button = screen.getByRole("button", { name: /^Follow/ });
    await fireEvent.click(button);
    await fireEvent.click(button);
    await fireEvent.click(button);

    expect(followLifter).toHaveBeenCalledTimes(1);
    release();
    await waitFor(() => expect(onChange).toHaveBeenCalledTimes(1));
  });
});

describe("when it fails", () => {
  it("says so and does not claim the state changed", async () => {
    followLifter.mockResolvedValue({ status: 500, data: {} });
    const onChange = show(false);

    await fireEvent.click(screen.getByRole("button", { name: /^Follow/ }));

    expect(await screen.findByText("Couldn't save that.")).toBeInTheDocument();
    expect(onChange).not.toHaveBeenCalled();
    // Still offering to follow, because the follow did not happen.
    expect(screen.getByRole("button", { name: /^Follow/ })).toBeInTheDocument();
  });

  it("becomes pressable again", async () => {
    followLifter.mockResolvedValue({ status: 500, data: {} });
    show(false);

    await fireEvent.click(screen.getByRole("button", { name: /^Follow/ }));
    await screen.findByText("Couldn't save that.");

    followLifter.mockResolvedValue({ status: 204, data: undefined });
    await fireEvent.click(screen.getByRole("button", { name: /^Follow/ }));

    await waitFor(() => expect(followLifter).toHaveBeenCalledTimes(2));
  });
});
