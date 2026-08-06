/** Standard Olympic bar weight in pounds. */
export const BAR_LB = 45;

/** Available plates (lb), largest first — 45 down to 2.5. */
export const PLATES_LB = [45, 35, 25, 10, 5, 2.5];

/**
 * The plates to load on ONE side of the bar for a target weight, largest first
 * (largest sits nearest the collar). Greedy over the available plates; anything
 * below the bar weight, or not representable, yields an empty list.
 *
 * All plate sizes are multiples of 0.5 lb (exact in floating point), so the
 * arithmetic is exact for the weights this app produces (bar + multiples of 5).
 */
export function platesPerSide(
  weightLb: number,
  bar = BAR_LB,
  plates = PLATES_LB,
): number[] {
  let perSide = (weightLb - bar) / 2;
  const result: number[] = [];
  if (perSide <= 0) return result;
  for (const plate of plates) {
    while (perSide + 1e-9 >= plate) {
      result.push(plate);
      perSide -= plate;
    }
  }
  return result;
}
