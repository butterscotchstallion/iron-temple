import type { Program, ProgramDay, ProgramDayExercise } from "./api";

/**
 * The pure logic behind the program editor.
 *
 * Kept out of the components because it is all answerable without a DOM, and
 * because the reorder arithmetic is the kind that is easy to get subtly wrong
 * and hard to see wrong on screen — a list that renders in the order you
 * expected tells you nothing about the ids that were sent.
 */

/** Anything with a stable id, which is all `move` needs to know. */
type Identified = { id: number };

/**
 * The list reordered by moving one item one place in `direction`.
 *
 * Returns the SAME array when the move is impossible — the first item moving up,
 * the last moving down, or an id that is not in the list. Callers check identity
 * to decide whether to send anything, so a no-op costs no request; it also means
 * a disabled button that somehow fires is harmless rather than a 400.
 */
export function move<T extends Identified>(
  items: T[],
  id: number,
  direction: "up" | "down",
): T[] {
  const from = items.findIndex((item) => item.id === id);
  if (from === -1) return items;
  const to = direction === "up" ? from - 1 : from + 1;
  if (to < 0 || to >= items.length) return items;

  const next = [...items];
  [next[from], next[to]] = [next[to], next[from]];
  return next;
}

/** The ids of a list, in order — the body every reorder endpoint takes. */
export function idsInOrder(items: Identified[]): number[] {
  return items.map((item) => item.id);
}

/**
 * Whether a name is acceptable, matching the API's own bounds so the two refuse
 * the same things rather than the client waving through what the server rejects.
 */
export function validName(name: string): boolean {
  const trimmed = name.trim();
  return trimmed.length > 0 && trimmed.length <= 80;
}

/**
 * The exercise ids already prescribed on a day, to keep the picker from offering
 * a lift that would only earn a 409 — one entry per lift is the rule this table
 * has had since the schema's first migration.
 *
 * Assistance is deliberately NOT excluded. A lifter who has curls as an accessory
 * and decides they belong in the program is doing something reasonable, and the
 * server copes: prescribe() drops an assistance row naming a lift the day
 * prescribes, so the lift appears once.
 */
export function prescribedExerciseIds(day: ProgramDay): number[] {
  return day.exercises.map((e) => e.exerciseId);
}

/**
 * How a prescription reads on one line: "5 × 5 · 95 lb", or "5 × 5" at zero.
 *
 * Zero means bodyweight, and "0 lb" is a worse way of saying that than saying
 * nothing — the same rule the rest of the app applies to banded and bodyweight
 * work.
 */
export function prescriptionSummary(lift: ProgramDayExercise): string {
  const block = `${lift.sets} × ${lift.reps}`;
  return lift.startingWeightLb > 0
    ? `${block} · starts at ${lift.startingWeightLb} lb`
    : block;
}

/**
 * Whether this program can be edited at all, which is exactly "is it mine".
 *
 * A seeded program has no owner, so this is false for everybody — the same
 * sentence the API enforces, restated here so the editor's route guard and its
 * controls cannot disagree with it. The server is still the boundary; this only
 * decides what to draw.
 */
export function canEdit(program: Pick<Program, "isMine">): boolean {
  return program.isMine;
}
