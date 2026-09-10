import { fireEvent, render, screen, waitFor } from "@testing-library/svelte";
import { beforeEach, describe, expect, it, vi } from "vitest";
import AssistancePicker from "./AssistancePicker.svelte";
import type { Exercise } from "./api";
import { RECENT_LIMIT } from "./library";

// The picker leads with the accessories a lifter actually trains, so the same
// handful needn't be searched for every time one is added to a day. What
// belongs in that section and in which order is library.test.ts' job; this file
// covers the wiring around it — that the section renders above the catalogue,
// that a lift still appears under its muscle group as well, and that filtering
// takes it away again.

const listExercises = vi.hoisted(() => vi.fn());
vi.mock("./api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./api")>()),
  listExercises,
}));

function exercise(over: Partial<Exercise> & { name: string }): Exercise {
  return {
    id: 1,
    muscleGroup: "other",
    equipment: "other",
    isAccessory: true,
    isCustom: false,
    topSet: null,
    lastPerformedOn: null,
    performedSessions: 0,
    ...over,
  };
}

// Names deliberately share no substring, so matching a rendered row back to the
// lift it came from is unambiguous.
const library: Exercise[] = [
  exercise({ id: 1, name: "Squat", muscleGroup: "legs", isAccessory: false }),
  exercise({ id: 2, name: "Dip", muscleGroup: "chest" }),
  exercise({ id: 3, name: "Hammer", muscleGroup: "arms" }),
  exercise({ id: 4, name: "Plank", muscleGroup: "core" }),
];

/** The library rows in the order they render, ignoring chips and buttons. */
function rowsInOrder(): string[] {
  return screen
    .getAllByRole("button")
    .map((button) => library.find((e) => button.textContent?.includes(e.name))?.name)
    .filter((name): name is string => name !== undefined);
}

function open(exercises: Exercise[], props: { exclude?: number[] } = {}) {
  listExercises.mockResolvedValue({ status: 200, data: exercises });
  return render(AssistancePicker, {
    props: { ...props, onAdd: vi.fn(async () => true), onCancel: vi.fn() },
  });
}

beforeEach(() => {
  listExercises.mockReset();
});

describe("the recent section", () => {
  it("leads with recently trained accessories, most recent first", async () => {
    open([
      library[0],
      { ...library[1], lastPerformedOn: "2026-08-20", performedSessions: 4 },
      { ...library[2], lastPerformedOn: "2026-09-02", performedSessions: 9 },
      library[3],
    ]);

    await waitFor(() => expect(screen.getByText("Recent")).toBeInTheDocument());
    expect(rowsInOrder().slice(0, 2)).toEqual(["Hammer", "Dip"]);
  });

  it("still lists a recent lift under its own muscle group", async () => {
    // Otherwise a movement goes missing from the one place a lifter scrolls to
    // expecting it.
    open([{ ...library[1], lastPerformedOn: "2026-09-02", performedSessions: 4 }]);

    await waitFor(() => expect(screen.getByText("Recent")).toBeInTheDocument());
    expect(screen.getAllByRole("button", { name: /Dip/ })).toHaveLength(2);
  });

  it("is absent when nothing has been trained yet", async () => {
    open(library);

    await waitFor(() =>
      expect(screen.getByRole("button", { name: /Dip/ })).toBeInTheDocument(),
    );
    expect(screen.queryByText("Recent")).not.toBeInTheDocument();
  });

  it("leaves out lifts the program prescribes and lifts already on the day", async () => {
    open(
      [
        { ...library[0], lastPerformedOn: "2026-09-02", performedSessions: 30 },
        { ...library[2], lastPerformedOn: "2026-09-02", performedSessions: 9 },
        { ...library[3], lastPerformedOn: "2026-09-01", performedSessions: 5 },
      ],
      // Hammer is already on this day, so offering it again would only earn a
      // 409 — the exclusion has to reach the recents too, not just the groups.
      { exclude: [3] },
    );

    await waitFor(() => expect(screen.getByText("Recent")).toBeInTheDocument());
    expect(rowsInOrder().slice(0, 1)).toEqual(["Plank"]);
    expect(screen.queryByRole("button", { name: /Hammer/ })).not.toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: /Squat/ })).toHaveLength(1);
  });

  it("goes away once the list is filtered", async () => {
    // Filtering already shortens the list, so the section stops being a
    // shortcut to it and becomes a duplicate in front of the match.
    open([
      { ...library[1], lastPerformedOn: "2026-09-02", performedSessions: 4 },
      { ...library[2], lastPerformedOn: "2026-09-01", performedSessions: 4 },
    ]);

    await waitFor(() => expect(screen.getByText("Recent")).toBeInTheDocument());

    await fireEvent.input(screen.getByPlaceholderText("Search exercises…"), {
      target: { value: "dip" },
    });
    expect(screen.queryByText("Recent")).not.toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: /Dip/ })).toHaveLength(1);

    await fireEvent.input(screen.getByPlaceholderText("Search exercises…"), {
      target: { value: "" },
    });
    await waitFor(() => expect(screen.getByText("Recent")).toBeInTheDocument());
  });

  it("holds no more than the limit", async () => {
    open(
      Array.from({ length: RECENT_LIMIT + 2 }, (_, i) =>
        exercise({
          id: 100 + i,
          name: `Lift ${i}`,
          lastPerformedOn: `2026-09-0${i + 1}`,
          performedSessions: 1,
        }),
      ),
    );

    await waitFor(() => expect(screen.getByText("Recent")).toBeInTheDocument());
    // Every lift renders under "Other" as well, so the recents are the excess
    // over one row each.
    expect(screen.getAllByRole("button", { name: /^Lift/ })).toHaveLength(
      RECENT_LIMIT * 2 + 2,
    );
  });
});
