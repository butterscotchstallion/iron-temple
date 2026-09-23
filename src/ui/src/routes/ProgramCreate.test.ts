import { fireEvent, render, screen, waitFor } from "@testing-library/svelte";
import { beforeEach, describe, expect, it, vi } from "vitest";
import ProgramCreate from "./ProgramCreate.svelte";
import { auth } from "../lib/auth.svelte";
import { testProgramSummary, testUser } from "../lib/testFixtures";

// The create screen, which is mostly a form and would not normally earn a render
// test. Three things here live entirely in handler and template branches, where
// the lib tests cannot see them:
//
//  - "start from nothing" must send null, not 0. A 0 would be an id the API
//    looks up and 404s on, and the failure would read as "couldn't create".
//  - a failed create must show the SERVER's message, because the two reasons a
//    create is refused (the name is taken, the source prescribes ramps) are
//    things only the server can tell apart and only the lifter can act on.
//  - the ramping program must not be in the dropdown at all.

const createProgram = vi.hoisted(() => vi.fn());
const listPrograms = vi.hoisted(() => vi.fn());
const push = vi.hoisted(() => vi.fn());

vi.mock("../lib/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../lib/api")>()),
  createProgram,
  listPrograms,
}));
vi.mock("svelte-spa-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("svelte-spa-router")>()),
  push,
}));

beforeEach(() => {
  vi.clearAllMocks();
  auth.me = testUser();
  auth.loaded = true;
  listPrograms.mockResolvedValue({
    status: 200,
    data: [
      testProgramSummary({ id: 1, name: "StrongLifts 5x5" }),
      testProgramSummary({ id: 6, name: "Madcow 5x5", progressionKind: "madcow" }),
    ],
  });
});

/** Fills the name field, which is the only required one. */
async function nameIt(value: string) {
  const field = screen.getByLabelText("Name");
  await fireEvent.input(field, { target: { value } });
}

describe("ProgramCreate", () => {
  it("sends null rather than 0 when starting from nothing", async () => {
    createProgram.mockResolvedValue({ status: 201, data: { id: 12 } });
    render(ProgramCreate);

    await nameIt("Push Pull Legs");
    await fireEvent.click(screen.getByRole("button", { name: /Create program/ }));

    await waitFor(() => expect(createProgram).toHaveBeenCalled());
    expect(createProgram).toHaveBeenCalledWith(
      expect.objectContaining({ cloneFromProgramId: null }),
    );
  });

  it("navigates to the new program on success", async () => {
    createProgram.mockResolvedValue({ status: 201, data: { id: 12 } });
    render(ProgramCreate);

    await nameIt("Push Pull Legs");
    await fireEvent.click(screen.getByRole("button", { name: /Create program/ }));

    await waitFor(() => expect(push).toHaveBeenCalledWith("/programs/12"));
  });

  it("shows the server's reason when the create is refused", async () => {
    createProgram.mockResolvedValue({
      status: 409,
      data: { code: "duplicate_name", message: "you already have a program with that name" },
    });
    render(ProgramCreate);

    await nameIt("StrongLifts 5x5");
    await fireEvent.click(screen.getByRole("button", { name: /Create program/ }));

    expect(
      await screen.findByText("you already have a program with that name"),
    ).toBeInTheDocument();
    expect(push).not.toHaveBeenCalled();
  });

  it("does not offer a ramping program as a starting point", async () => {
    render(ProgramCreate);

    // Waiting on the one that SHOULD be there, so the absence below is a real
    // absence rather than a list that had not loaded yet.
    await screen.findByRole("option", { name: "StrongLifts 5x5" });
    expect(
      screen.queryByRole("option", { name: "Madcow 5x5" }),
    ).not.toBeInTheDocument();
  });
});
