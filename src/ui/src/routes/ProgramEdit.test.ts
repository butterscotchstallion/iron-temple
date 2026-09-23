import { fireEvent, render, screen, waitFor } from "@testing-library/svelte";
import { beforeEach, describe, expect, it, vi } from "vitest";
import ProgramEdit from "./ProgramEdit.svelte";
import { auth } from "../lib/auth.svelte";
import { testProgramSummary, testUser } from "../lib/testFixtures";
import type { Program, ProgramDay, ProgramDayExercise } from "../lib/api";

// The editor, tested for the three things that live in template and handler
// branches where programEdit.test.ts cannot see them:
//
//  - the reorder arrows are disabled at the ends of a list, and the ids sent
//    are the WHOLE new order — a partial list is a 400 that changes nothing, so
//    getting this wrong is a feature that silently never works;
//  - opening a program that is not yours redirects rather than rendering a
//    screen of controls that will all 404;
//  - a failed write shows the server's message, because the reasons an edit is
//    refused (a taken name, a lift already on the day) are things only the
//    server knows and only the lifter can act on.

const getProgram = vi.hoisted(() => vi.fn());
const reorderPrescriptions = vi.hoisted(() => vi.fn());
const addProgramDay = vi.hoisted(() => vi.fn());
const listExercises = vi.hoisted(() => vi.fn());
const push = vi.hoisted(() => vi.fn());

vi.mock("../lib/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../lib/api")>()),
  getProgram,
  reorderPrescriptions,
  addProgramDay,
  listExercises,
}));
vi.mock("svelte-spa-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("svelte-spa-router")>()),
  push,
}));

function lift(over: Partial<ProgramDayExercise> = {}): ProgramDayExercise {
  return {
    id: 1,
    exerciseId: 10,
    exerciseName: "Squat",
    position: 1,
    sets: 5,
    reps: 5,
    startingWeightLb: 95,
    restSeconds: 180,
    ...over,
  };
}

function day(over: Partial<ProgramDay> = {}): ProgramDay {
  return {
    id: 7,
    name: "Workout A",
    position: 1,
    weekday: null,
    exercises: [],
    assistance: [],
    ...over,
  };
}

/** A program of the caller's own, which is the only kind this screen opens. */
function mine(over: Partial<Program> = {}): Program {
  return {
    ...testProgramSummary({
      id: 3,
      name: "Push Pull Legs",
      ownerId: 1,
      isMine: true,
      isShared: false,
    }),
    days: [day()],
    ...over,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  auth.me = testUser();
  auth.loaded = true;
  listExercises.mockResolvedValue({ status: 200, data: [] });
});

/** Renders and waits for the load to settle. */
async function open(program: Program) {
  getProgram.mockResolvedValue({ status: 200, data: program });
  render(ProgramEdit, { props: { params: { id: String(program.id) } } });
  await screen.findByRole("heading", { name: `Edit ${program.name}` });
}

describe("ProgramEdit", () => {
  it("disables the reorder arrows at the ends of the list", async () => {
    await open(
      mine({
        days: [
          day({
            exercises: [
              lift({ id: 1, exerciseName: "Squat" }),
              lift({ id: 2, exerciseName: "Bench Press" }),
              lift({ id: 3, exerciseName: "Barbell Row" }),
            ],
          }),
        ],
      }),
    );

    expect(screen.getByLabelText("Move Squat up")).toBeDisabled();
    expect(screen.getByLabelText("Move Squat down")).toBeEnabled();
    expect(screen.getByLabelText("Move Bench Press up")).toBeEnabled();
    expect(screen.getByLabelText("Move Barbell Row down")).toBeDisabled();
  });

  it("sends the complete new order when a lift moves", async () => {
    reorderPrescriptions.mockResolvedValue({ status: 200, data: mine() });
    await open(
      mine({
        days: [
          day({
            exercises: [
              lift({ id: 1, exerciseName: "Squat" }),
              lift({ id: 2, exerciseName: "Bench Press" }),
              lift({ id: 3, exerciseName: "Barbell Row" }),
            ],
          }),
        ],
      }),
    );

    await fireEvent.click(screen.getByLabelText("Move Barbell Row up"));

    await waitFor(() => expect(reorderPrescriptions).toHaveBeenCalled());
    // Every id, exactly once, in the new order — a partial list is a 400 that
    // changes nothing.
    expect(reorderPrescriptions).toHaveBeenCalledWith(3, 7, { ids: [1, 3, 2] });
  });

  it("redirects rather than opening somebody else's program", async () => {
    // The API refuses every write regardless; this only stops the screen from
    // drawing a wall of controls that all 404.
    await getProgram.mockResolvedValue({
      status: 200,
      data: { ...mine({ isMine: false }) },
    });
    render(ProgramEdit, { props: { params: { id: "3" } } });

    await waitFor(() => expect(push).toHaveBeenCalledWith("/programs/3"));
  });

  it("shows the server's reason when a write is refused", async () => {
    addProgramDay.mockResolvedValue({
      status: 409,
      data: { code: "duplicate_day", message: "this program already has a day with that name" },
    });
    await open(mine());

    await fireEvent.input(screen.getByLabelText("New day"), {
      target: { value: "Workout A" },
    });
    await fireEvent.click(screen.getByRole("button", { name: /Add day/ }));

    expect(
      await screen.findByText("this program already has a day with that name"),
    ).toBeInTheDocument();
  });

  it("offers to bring an archived program back rather than archive it again", async () => {
    await open(mine({ archivedAt: "2026-09-01T00:00:00Z" }));

    expect(
      screen.getByRole("button", { name: "Bring this program back" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Archive this program" }),
    ).not.toBeInTheDocument();
  });
});
