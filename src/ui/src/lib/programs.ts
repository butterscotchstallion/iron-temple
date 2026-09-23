import type { ProgramSummary } from "./api";

/**
 * The subtitle shown on a program card: the program's description, or a generic
 * fallback when the description is empty (all seeded programs are linear).
 */
export function programSubtitle(
  program: Pick<ProgramSummary, "description">,
): string {
  const description = program.description.trim();
  return description !== "" ? description : "Linear progression";
}

/**
 * Who a program came from, for the line under its subtitle — or "" when there is
 * nobody to name and the line should not be drawn at all.
 *
 * Three cases, and only one of them says anything:
 *
 * - A seeded program (`ownerId` null) belongs to the install rather than to a
 *   person. Attributing it would put a line of near-blank space on eight of the
 *   nine cards a fresh install draws, and "by nobody" is not information.
 * - Your own program is already marked, by the "Yours" badge on the card. Saying
 *   it twice is noise.
 * - Somebody else's shared program is the case worth a line, because two lifters
 *   may each have a "Push Pull Legs" — per-owner name uniqueness allows it
 *   deliberately — and the owner is the only thing that tells the two cards
 *   apart.
 *
 * Falls back to "another lifter" rather than rendering "Shared by " with nothing
 * after it, which is what an account with an empty display name would otherwise
 * produce.
 */
/**
 * The programs a new one can be started from.
 *
 * Two exclusions, for different reasons:
 *
 * - **Ramping programs.** Madcow prescribes percentages of a top set, and the
 *   editor has no way to express or change one — so a copy would be a program
 *   whose ramps its owner could see the effects of and never edit. The API
 *   refuses it with `unsupported_progression`; this keeps it out of the list so
 *   nobody picks it and reads a 409 as a bug.
 * - **Archived programs.** One its owner has retired is not a starting point
 *   they are offering. It still appears in the picker for the owner behind
 *   `includeArchived`, which is a different question — "where was that program
 *   I stopped using" rather than "what should I build on".
 *
 * Somebody else's shared program is deliberately INCLUDED. Being able to take a
 * copy and make it yours is most of the point of sharing one, and it costs the
 * owner nothing: a clone copies rows rather than moving them.
 */
export function cloneSources(programs: ProgramSummary[]): ProgramSummary[] {
  return programs.filter(
    (p) => p.progressionKind === "linear" && p.archivedAt === null,
  );
}

export function programAttribution(
  program: Pick<ProgramSummary, "ownerId" | "ownerName" | "isMine">,
): string {
  if (program.ownerId === null || program.isMine) return "";
  const owner = program.ownerName?.trim();
  return `Shared by ${owner !== undefined && owner !== "" ? owner : "another lifter"}`;
}
