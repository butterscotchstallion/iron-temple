/**
 * Loading the bar with the plates that actually exist.
 *
 * The bar weight and the plate inventory used to be the two constants below,
 * compiled into the bundle. Both were wrong for this gym in different ways: the
 * bar was assumed to be 45 lb when the real one is 80, and the plate set was
 * treated as unlimited, so the loader would happily call for a fourth pair of
 * 45s that nobody owns. Both now come off the profile (`User.barWeightLb` and
 * `User.plates`) and these are only the fallback for a client that has not
 * loaded one yet.
 */

/** Bar weight in pounds, when the profile has not been loaded. */
export const DEFAULT_BAR_LB = 45;

/** The standard rack, when the profile has not been loaded. */
export const DEFAULT_PLATES: PlateInventory = [
  { plateLb: 45, pairs: 2 },
  { plateLb: 35, pairs: 2 },
  { plateLb: 25, pairs: 2 },
  { plateLb: 10, pairs: 2 },
  { plateLb: 5, pairs: 2 },
  { plateLb: 2.5, pairs: 2 },
];

/**
 * What the lifter owns. `pairs`, not a count: plates load symmetrically, so
 * three 45s are one usable pair, and the API stores it in the unit it is used
 * in. Order is not assumed — every function here sorts before loading.
 */
export type PlateInventory = { plateLb: number; pairs: number }[];

/** A loaded bar: the plates on ONE side, and the weight that actually lands. */
export type Loadout = {
  /** Plates for one side, heaviest first (heaviest sits nearest the collar). */
  plates: number[];
  /** What the bar weighs so loaded — the target only when it was reachable. */
  weightLb: number;
  /** True when `weightLb` is not the weight that was asked for. */
  rounded: boolean;
};

// Everything works in half-pounds as integers. Plate denominations are all
// multiples of 0.5, so this makes the arithmetic exact rather than
// nearly-exact, and lets the search below index an array instead of comparing
// floats with an epsilon.
const UNIT = 2;

// The heaviest one side will ever be asked to carry. A cap is needed because
// the search allocates a slot per reachable half-pound; 500 lb a side is a
// 1,000 lb bar, which is past every record and far past this app's purpose.
const MAX_PER_SIDE_LB = 500;

/**
 * The plates to load on ONE side of the bar for a target weight, heaviest
 * first. Bounded by what the lifter owns, so this never asks for a plate that
 * is not in the rack.
 *
 * When the target cannot be built exactly, the closest achievable weight is
 * used instead — see `loadBar`, which returns what that weight was.
 */
export function platesPerSide(
  weightLb: number,
  bar = DEFAULT_BAR_LB,
  plates: PlateInventory = DEFAULT_PLATES,
): number[] {
  return loadBar(weightLb, bar, plates).plates;
}

/**
 * Load the bar as close to `weightLb` as the rack allows.
 *
 * Greedy first, because for a standard rack it is both optimal and instant. It
 * is only tried as a fast path though: greedy genuinely fails once plates run
 * out — with one pair of 45s and two of 25s, a 50 lb side greedily takes the 45
 * and then cannot make the last 5, while 25+25 is exact. When greedy misses,
 * the exhaustive search below finds the best available.
 */
export function loadBar(
  weightLb: number,
  bar = DEFAULT_BAR_LB,
  plates: PlateInventory = DEFAULT_PLATES,
): Loadout {
  const targetPerSide = (weightLb - bar) / 2;
  if (targetPerSide <= 0 || !Number.isFinite(targetPerSide)) {
    // At or below the bar there is nothing to load, and the bar is exactly what
    // it weighs — not a rounding of anything.
    return { plates: [], weightLb: Math.min(weightLb, bar), rounded: false };
  }

  const rack = usableRack(plates);
  const target = Math.round(targetPerSide * UNIT);

  const greedy = greedyLoad(target, rack);
  if (greedy.total === target) {
    return { plates: toLb(greedy.picked), weightLb, rounded: false };
  }

  const best = closestLoad(target, rack);
  const achieved = bar + (best.total / UNIT) * 2;
  return {
    plates: toLb(best.picked),
    weightLb: achieved,
    rounded: achieved !== weightLb,
  };
}

/**
 * Human-readable loadout for ONE side, e.g. "25 + 5 / side", or "bar only" when
 * there is nothing to load. A rounded weight says so, because silently loading
 * something other than the number on the screen is how a lifter ends up doing
 * the wrong set.
 */
export function plateLabel(
  weightLb: number,
  bar = DEFAULT_BAR_LB,
  plates: PlateInventory = DEFAULT_PLATES,
): string {
  const { plates: side, weightLb: actual, rounded } = loadBar(weightLb, bar, plates);
  const base = side.length ? `${side.join(" + ")} / side` : "bar only";
  return rounded ? `${base} · ${actual} lb` : base;
}

// ---------------------------------------------------------------------------
// Internals
// ---------------------------------------------------------------------------

type Rack = { unit: number; pairs: number }[];

/** Denominations in half-pounds, heaviest first, with nonsense dropped. */
function usableRack(plates: PlateInventory): Rack {
  return plates
    .filter((p) => p.plateLb > 0 && p.pairs > 0)
    .map((p) => ({ unit: Math.round(p.plateLb * UNIT), pairs: Math.floor(p.pairs) }))
    .filter((p) => p.unit > 0)
    .sort((a, b) => b.unit - a.unit);
}

function toLb(units: number[]): number[] {
  return units.map((u) => u / UNIT);
}

function greedyLoad(target: number, rack: Rack): { picked: number[]; total: number } {
  const picked: number[] = [];
  let total = 0;
  for (const { unit, pairs } of rack) {
    const want = Math.floor((target - total) / unit);
    for (let i = 0; i < Math.min(want, pairs); i++) {
      picked.push(unit);
      total += unit;
    }
  }
  return { picked, total };
}

/**
 * The achievable per-side weight closest to `target`, never above it.
 *
 * A bounded-knapsack reachability scan: `from[w]` records the denomination that
 * first reached w, so a loadout can be walked back out without storing one per
 * cell. Bounded by MAX_PER_SIDE_LB, and each denomination is expanded pair by
 * pair — the counts are small (a rack is a handful of plates, a few pairs each),
 * so the straightforward form is fast enough and stays readable.
 *
 * Never above the target deliberately: rounding a working set UP is a heavier
 * set than the program called for, which is the one direction that can hurt.
 */
function closestLoad(target: number, rack: Rack): { picked: number[]; total: number } {
  const cap = Math.min(target, MAX_PER_SIDE_LB * UNIT);
  const from = new Int32Array(cap + 1).fill(-1);
  const reached = new Uint8Array(cap + 1);
  reached[0] = 1;

  for (const { unit, pairs } of rack) {
    for (let n = 0; n < pairs; n++) {
      // Descending, so a plate placed in this pass is not reused by it — that
      // is what keeps the pair count honest.
      for (let w = cap - unit; w >= 0; w--) {
        if (reached[w] && !reached[w + unit]) {
          reached[w + unit] = 1;
          from[w + unit] = unit;
        }
      }
    }
  }

  let total = cap;
  while (total > 0 && !reached[total]) total--;

  const picked: number[] = [];
  for (let w = total; w > 0; ) {
    const unit = from[w];
    if (unit <= 0) break; // unreachable in practice; refuse to loop forever
    picked.push(unit);
    w -= unit;
  }
  picked.sort((a, b) => b - a);
  return { picked, total };
}
