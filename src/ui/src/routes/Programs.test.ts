import { render, screen } from "@testing-library/svelte";
import { beforeEach, describe, expect, it, vi } from "vitest";
import Programs from "./Programs.svelte";
import { auth } from "../lib/auth.svelte";
import { testProgramSummary, testUser } from "../lib/testFixtures";

// A render test for the picker, which had none.
//
// It earns its place on the one thing the lib tests cannot see: which card gets
// the "Yours" badge. programAttribution is pure and tested next to itself, but
// the badge is a template branch on `program.isMine`, and getting it wrong is
// the kind of mistake that reads fine in review — an Edit affordance offered on
// a program the API will refuse to let you edit, or withheld from one that is
// actually yours.

const listPrograms = vi.hoisted(() => vi.fn());
const listSessions = vi.hoisted(() => vi.fn());
vi.mock("../lib/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../lib/api")>()),
  listPrograms,
  listSessions,
}));

beforeEach(() => {
  vi.clearAllMocks();
  auth.me = testUser();
  auth.loaded = true;
  // No sessions, so nothing is highlighted as the current program and the cards
  // differ only in what this file is asserting about.
  listSessions.mockResolvedValue({ status: 200, data: { items: [], total: 0 } });
});

/** Waits for the in-flight load to resolve and the skeletons to be replaced. */
async function shown(name: string) {
  return screen.findByRole("heading", { name, level: 2 });
}

describe("Programs", () => {
  it("badges the caller's own program and not the seeded ones", async () => {
    listPrograms.mockResolvedValue({
      status: 200,
      data: [
        testProgramSummary({ id: 1, name: "StrongLifts 5x5" }),
        testProgramSummary({
          id: 2,
          name: "Push Pull Legs",
          ownerId: 1,
          ownerName: "Ada Lovelace",
          isMine: true,
          isShared: false,
        }),
      ],
    });

    render(Programs);
    await shown("Push Pull Legs");

    // One badge, not two: the seeded program belongs to the install, which is
    // nobody, so it is not "yours" for any value of you.
    expect(screen.getAllByText("Yours")).toHaveLength(1);
  });

  it("attributes another lifter's shared program", async () => {
    listPrograms.mockResolvedValue({
      status: 200,
      data: [
        testProgramSummary({
          id: 3,
          name: "Grace's Deadlift Block",
          ownerId: 2,
          ownerName: "Grace Hopper",
          isMine: false,
          isShared: true,
        }),
      ],
    });

    render(Programs);
    await shown("Grace's Deadlift Block");

    expect(screen.getByText("Shared by Grace Hopper")).toBeInTheDocument();
    expect(screen.queryByText("Yours")).not.toBeInTheDocument();
  });

  it("attributes nobody on a seeded program", async () => {
    listPrograms.mockResolvedValue({
      status: 200,
      data: [testProgramSummary({ id: 1, name: "StrongLifts 5x5" })],
    });

    render(Programs);
    await shown("StrongLifts 5x5");

    expect(screen.queryByText(/^Shared by/)).not.toBeInTheDocument();
  });
});
