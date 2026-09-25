import { render, screen, waitFor, fireEvent } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import House from "./House.svelte";
import { auth } from "../lib/auth.svelte";
import { resetHouses } from "../lib/houses.svelte";
import {
  testHouseDetail,
  testHouseJoinRequest,
  testHouseMember,
  testLifter,
  testUser,
} from "../lib/testFixtures";

// One House's page.
//
// What carries this screen is the join button, and what makes it worth testing is
// that it has FOUR states rather than two: a stranger may ask, somebody who has
// asked may withdraw, a member may leave, and a lifter already in another House
// can do none of it and has to be told why. Every one of those is read off
// `viewer` — the server's answer — so a test that computed the state itself would
// be testing its own arithmetic.
//
// The other half is the owner's: the pending-request list is ABSENT for a member
// and EMPTY for an owner with nobody waiting, and those must not render the same.

const getHouse = vi.hoisted(() => vi.fn());
const updateHouse = vi.hoisted(() => vi.fn());
const requestToJoinHouse = vi.hoisted(() => vi.fn());
const withdrawHouseRequest = vi.hoisted(() => vi.fn());
const approveHouseRequest = vi.hoisted(() => vi.fn());
const declineHouseRequest = vi.hoisted(() => vi.fn());
const leaveHouse = vi.hoisted(() => vi.fn());
const listHouses = vi.hoisted(() => vi.fn());
vi.mock("../lib/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../lib/api")>()),
  getHouse,
  updateHouse,
  requestToJoinHouse,
  withdrawHouseRequest,
  approveHouseRequest,
  declineHouseRequest,
  leaveHouse,
  listHouses,
}));

const push = vi.hoisted(() => vi.fn());
vi.mock("svelte-spa-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("svelte-spa-router")>()),
  push,
}));

const pushToast = vi.hoisted(() => vi.fn());
vi.mock("../lib/toast.svelte", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../lib/toast.svelte")>()),
  pushToast,
}));

const ME = 1;
const PROPS = { params: { id: "7" } };

/** The House this page will load. */
function served(overrides = {}) {
  getHouse.mockResolvedValue({ status: 200, data: testHouseDetail(overrides) });
}

beforeEach(() => {
  vi.clearAllMocks();
  served();
  listHouses.mockResolvedValue({ status: 200, data: { items: [], memberships: [] } });
  requestToJoinHouse.mockResolvedValue({ status: 201, data: testHouseJoinRequest() });
  withdrawHouseRequest.mockResolvedValue({ status: 204, data: undefined });
  approveHouseRequest.mockResolvedValue({ status: 204, data: undefined });
  declineHouseRequest.mockResolvedValue({ status: 204, data: undefined });
  leaveHouse.mockResolvedValue({ status: 204, data: undefined });
  auth.me = testUser({ id: ME });
  auth.loaded = true;
});

afterEach(() => resetHouses());

describe("the header", () => {
  it("shows the name, the sigil, the tagline and the description", async () => {
    served({
      name: "House Iron",
      sigil: "IRON",
      tagline: "We lift at dawn",
      description: "A longer account of who we are.",
    });
    render(House, { props: PROPS });

    expect(await screen.findByText("House Iron")).toBeInTheDocument();
    expect(screen.getByText("IRON")).toBeInTheDocument();
    expect(screen.getByText("We lift at dawn")).toBeInTheDocument();
    expect(screen.getByText("A longer account of who we are.")).toBeInTheDocument();
  });

  // Both are optional and empty is ordinary, so neither may leave a stray element
  // behind when a House has not set it.
  it("omits the tagline and description when the House has neither", async () => {
    served({ name: "House Iron", tagline: "", description: "" });
    render(House, { props: PROPS });

    expect(await screen.findByText("House Iron")).toBeInTheDocument();
    // A regex, because the count shares its paragraph with the founding date.
    expect(screen.getByText(/1 member/)).toBeInTheDocument();
  });

  it("counts members in the plural once there are two", async () => {
    served({
      memberCount: 2,
      members: [
        testHouseMember({ isOwner: true }),
        testHouseMember({ lifter: testLifter({ id: 3, displayName: "Alan" }) }),
      ],
    });
    render(House, { props: PROPS });

    expect(await screen.findByText(/2 members/)).toBeInTheDocument();
  });
});

describe("the join button", () => {
  it("offers to join for a lifter with no House", async () => {
    render(House, { props: PROPS });
    expect(
      await screen.findByRole("button", { name: /request to join/i }),
    ).toBeInTheDocument();
  });

  it("asks, then reloads the House and the site-wide list", async () => {
    render(House, { props: PROPS });
    await fireEvent.click(await screen.findByRole("button", { name: /request to join/i }));

    await waitFor(() => expect(requestToJoinHouse).toHaveBeenCalledWith(7));
    // Both, because joining changes a sigil that is drawn on every other screen.
    await waitFor(() => expect(listHouses).toHaveBeenCalled());
    expect(getHouse).toHaveBeenCalledTimes(2);
  });

  // Asking an EMPTY House claims it outright — the API answers 200 with the
  // House instead of 201 with a request, because there is no owner to queue for.
  // The two status codes are the only thing telling these apart, so a version
  // that toasted "Asked to join" at a lifter who is now the owner would be wrong
  // in the one place the lifter is looking.
  it("says the House is theirs when an empty one is claimed", async () => {
    requestToJoinHouse.mockResolvedValue({
      status: 200,
      data: testHouseDetail({
        name: "House Last Out",
        viewer: { isMember: true, isOwner: true, inAnotherHouse: false },
      }),
    });
    render(House, { props: PROPS });
    await fireEvent.click(await screen.findByRole("button", { name: /request to join/i }));

    await waitFor(() =>
      expect(pushToast).toHaveBeenCalledWith(
        expect.objectContaining({ title: "The House is yours" }),
      ),
    );
    expect(pushToast).toHaveBeenCalledWith(
      expect.objectContaining({ body: expect.stringContaining("House Last Out") }),
    );
    // Still both reads: claiming changes the sigil drawn beside their name.
    await waitFor(() => expect(listHouses).toHaveBeenCalled());
  });

  it("offers to withdraw once a request is outstanding", async () => {
    served({ viewer: { isMember: false, isOwner: false, inAnotherHouse: false, openRequestId: 21 } });
    render(House, { props: PROPS });

    expect(await screen.findByText(/you've asked to join/i)).toBeInTheDocument();
    await fireEvent.click(screen.getByRole("button", { name: /withdraw/i }));
    await waitFor(() => expect(withdrawHouseRequest).toHaveBeenCalledWith(7, 21));
  });

  // Said rather than shown as a disabled button: a lifter who cannot ask should
  // learn which rule stopped them instead of tapping at something inert.
  it("explains itself to a lifter already in another House", async () => {
    served({ viewer: { isMember: false, isOwner: false, inAnotherHouse: true } });
    render(House, { props: PROPS });

    expect(await screen.findByText(/already in a House/i)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /request to join/i })).not.toBeInTheDocument();
  });

  it("offers to leave for a member, and not to join", async () => {
    served({ viewer: { isMember: true, isOwner: false, inAnotherHouse: false } });
    render(House, { props: PROPS });

    expect(await screen.findByRole("button", { name: /leave house/i })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /request to join/i })).not.toBeInTheDocument();
  });

  it("leaves, then sends the lifter back to the list", async () => {
    served({ viewer: { isMember: true, isOwner: false, inAnotherHouse: false } });
    render(House, { props: PROPS });
    await fireEvent.click(await screen.findByRole("button", { name: /leave house/i }));

    await waitFor(() => expect(leaveHouse).toHaveBeenCalled());
    // Not back to the page they just left, which may no longer exist — leaving as
    // the last member deletes the House.
    await waitFor(() => expect(push).toHaveBeenCalledWith("/houses"));
  });
});

describe("the owner's controls", () => {
  const OWNER = { isMember: true, isOwner: true, inAnotherHouse: false };

  it("offers an edit form only to the owner", async () => {
    served({ viewer: OWNER, pendingRequests: [] });
    render(House, { props: PROPS });
    expect(await screen.findByRole("button", { name: /edit/i })).toBeInTheDocument();
  });

  it("hides the edit form from an ordinary member", async () => {
    served({ viewer: { isMember: true, isOwner: false, inAnotherHouse: false } });
    render(House, { props: PROPS });

    await screen.findByRole("button", { name: /leave house/i });
    expect(screen.queryByRole("button", { name: /^edit$/i })).not.toBeInTheDocument();
  });

  it("saves an edit and refreshes the site-wide list", async () => {
    served({ viewer: OWNER, pendingRequests: [] });
    updateHouse.mockResolvedValue({
      status: 200,
      data: testHouseDetail({ viewer: OWNER, tagline: "We lift at dawn", pendingRequests: [] }),
    });
    render(House, { props: PROPS });

    await fireEvent.click(await screen.findByRole("button", { name: /edit/i }));
    await fireEvent.click(await screen.findByRole("button", { name: /^save$/i }));

    await waitFor(() => expect(updateHouse).toHaveBeenCalled());
    // A rename changes the sigil drawn beside every member's name elsewhere.
    await waitFor(() => expect(listHouses).toHaveBeenCalled());
  });

  it("lists who is waiting, and approves one", async () => {
    served({
      viewer: OWNER,
      pendingRequests: [
        // A lifter who is NOT already in the member list: the default member is
        // testLifter, and a requester with the same name would make this assertion
        // pass on the wrong element.
        testHouseJoinRequest({
          id: 21,
          lifter: testLifter({ id: 3, displayName: "Alan Turing" }),
        }),
      ],
    });
    render(House, { props: PROPS });

    expect(await screen.findByText("Alan Turing")).toBeInTheDocument();
    await fireEvent.click(screen.getByRole("button", { name: /approve/i }));
    await waitFor(() => expect(approveHouseRequest).toHaveBeenCalledWith(7, 21));
  });

  it("declines one", async () => {
    served({ viewer: OWNER, pendingRequests: [testHouseJoinRequest({ id: 21 })] });
    render(House, { props: PROPS });

    await fireEvent.click(await screen.findByRole("button", { name: /decline/i }));
    await waitFor(() => expect(declineHouseRequest).toHaveBeenCalledWith(7, 21));
  });

  // An owner with nobody waiting still gets the section, saying so. A member does
  // not get it at all — "nobody has asked" and "you may not know" are different
  // answers and the API is careful to keep them apart.
  it("says nobody is waiting to an owner with an empty queue", async () => {
    served({ viewer: OWNER, pendingRequests: [] });
    render(House, { props: PROPS });
    expect(await screen.findByText(/nobody is waiting/i)).toBeInTheDocument();
  });

  it("draws no request section at all for a member", async () => {
    served({ viewer: { isMember: true, isOwner: false, inAnotherHouse: false } });
    render(House, { props: PROPS });

    await screen.findByRole("button", { name: /leave house/i });
    expect(screen.queryByText(/asking to join/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/nobody is waiting/i)).not.toBeInTheDocument();
  });

  // The one failure worth naming: the lifter joined somewhere else while the
  // request sat in the queue, so there is nothing wrong with the House or the tap.
  it("explains a request that can no longer be approved", async () => {
    served({ viewer: OWNER, pendingRequests: [testHouseJoinRequest({ id: 21 })] });
    approveHouseRequest.mockResolvedValue({ status: 409, data: undefined });
    render(House, { props: PROPS });

    await fireEvent.click(await screen.findByRole("button", { name: /approve/i }));
    await waitFor(() =>
      expect(pushToast).toHaveBeenCalledWith(
        expect.objectContaining({ title: "They've joined another House" }),
      ),
    );
  });
});

describe("when the House is gone", () => {
  // Reachable from a notification about a House whose last member has since left,
  // which is an ordinary path rather than a broken link.
  it("says so rather than offering a retry", async () => {
    getHouse.mockResolvedValue({ status: 404, data: undefined });
    render(House, { props: PROPS });

    expect(await screen.findByText(/doesn't exist/i)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /retry/i })).not.toBeInTheDocument();
  });

  it("offers a retry when the load merely failed", async () => {
    getHouse.mockResolvedValue({ status: 500, data: undefined });
    render(House, { props: PROPS });

    expect(await screen.findByRole("button", { name: /retry/i })).toBeInTheDocument();
  });
});
