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
