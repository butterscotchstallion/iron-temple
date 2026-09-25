import { render, screen, waitFor, fireEvent } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import Houses from "./Houses.svelte";
import { auth } from "../lib/auth.svelte";
import { resetHouses } from "../lib/houses.svelte";
import {
  testHouse,
  testHouseDetail,
  testHouseMembership,
  testUser,
} from "../lib/testFixtures";

// The list of Houses, and the way into founding one.
//
// This screen reads the same module every sigil reads rather than fetching its own
// list, so the thing worth proving is that it can still tell the three states of
// that module apart: nothing loaded, loaded and empty, and loaded with Houses in
// it. A failed first load and an install where nobody has founded one are both
// "no Houses" to a naive renderer, and they need different words.

const listHouses = vi.hoisted(() => vi.fn());
const createHouse = vi.hoisted(() => vi.fn());
vi.mock("../lib/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../lib/api")>()),
  listHouses,
  createHouse,
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

function served(items: unknown[] = [], memberships: unknown[] = []) {
  listHouses.mockResolvedValue({ status: 200, data: { items, memberships } });
}

/** Fill in the form's two required fields. */
async function fillForm(name: string, sigil: string) {
  const fields = screen.getAllByRole("textbox");
  await fireEvent.input(fields[0], { target: { value: name } });
  await fireEvent.input(fields[1], { target: { value: sigil } });
}

beforeEach(() => {
  vi.clearAllMocks();
  served();
  createHouse.mockResolvedValue({ status: 201, data: testHouseDetail({ id: 7 }) });
  auth.me = testUser({ id: ME });
  auth.loaded = true;
  resetHouses();
});

afterEach(() => resetHouses());

describe("the list", () => {
  it("says nobody has founded one on an install with no Houses", async () => {
    render(Houses);
    expect(await screen.findByText(/nobody has founded a House yet/i)).toBeInTheDocument();
  });

  it("offers a retry when the list could not be loaded", async () => {
    // Which is the state that must not read as "there are none": loadHouses is
    // silent on failure, so the only signal is that the module never loaded.
    listHouses.mockResolvedValue({ status: 500, data: undefined });
    render(Houses);

    expect(await screen.findByRole("button", { name: /retry/i })).toBeInTheDocument();
    expect(screen.queryByText(/nobody has founded a House yet/i)).not.toBeInTheDocument();
  });

  it("lists each House with its sigil", async () => {
    served([testHouse({ id: 7, name: "House Iron", sigil: "IRON", memberCount: 3 })]);
    render(Houses);

    expect(await screen.findByText("House Iron")).toBeInTheDocument();
    expect(screen.getByText("IRON")).toBeInTheDocument();
    expect(screen.getByText("3 members")).toBeInTheDocument();
  });

  it("prefers a House's tagline over its member count", async () => {
    served([testHouse({ id: 7, name: "House Iron", tagline: "We lift at dawn" })]);
    render(Houses);

    expect(await screen.findByText("We lift at dawn")).toBeInTheDocument();
    expect(screen.queryByText("1 member")).not.toBeInTheDocument();
  });

  it("names the caller's own House before the list", async () => {
    served(
      [testHouse({ id: 7, name: "House Iron" })],
      [testHouseMembership({ userId: ME, houseId: 7 })],
    );
    render(Houses);

    expect(await screen.findByText(/your house/i)).toBeInTheDocument();
  });
});

describe("founding one", () => {
  it("is offered to a lifter with no House", async () => {
    render(Houses);
    expect(await screen.findByRole("button", { name: /found one/i })).toBeInTheDocument();
  });

  // One House at a time: a lifter who is in one cannot found another, so the
  // button is not there to be pressed into a 409.
  it("is not offered to a lifter who is already in one", async () => {
    served(
      [testHouse({ id: 7 })],
      [testHouseMembership({ userId: ME, houseId: 7 })],
    );
    render(Houses);

    await screen.findByText(/your house/i);
    expect(screen.queryByRole("button", { name: /found one/i })).not.toBeInTheDocument();
  });

  it("founds it, then opens the new House", async () => {
    render(Houses);
    await fireEvent.click(await screen.findByRole("button", { name: /found one/i }));
    await fillForm("House Iron", "IRON");
    await fireEvent.click(screen.getByRole("button", { name: /found it/i }));

    await waitFor(() =>
      expect(createHouse).toHaveBeenCalledWith(
        expect.objectContaining({ name: "House Iron", sigil: "IRON" }),
      ),
    );
    await waitFor(() => expect(push).toHaveBeenCalledWith("/houses/7"));
  });

  // Shown in the form rather than as a toast, because it names a field the lifter
  // is looking at and a toast would outlive the fix.
  it("reports a taken name in the form", async () => {
    createHouse.mockResolvedValue({ status: 409, data: undefined });
    render(Houses);

    await fireEvent.click(await screen.findByRole("button", { name: /found one/i }));
    await fillForm("House Iron", "IRON");
    await fireEvent.click(screen.getByRole("button", { name: /found it/i }));

    expect(
      await screen.findByText(/already has that name or sigil/i),
    ).toBeInTheDocument();
    expect(push).not.toHaveBeenCalled();
  });
});
