/**
 * The maths behind the animated form demonstrations.
 *
 * A movement is described as JOINT ANGLES, and forward kinematics turns those
 * into points. Frames are sampled by interpolating the ANGLES, never the
 * positions — that is the whole trick, and it is not a stylistic preference. A
 * chord is shorter than the arc it spans, so interpolating a knee's POSITION
 * between "straight" and "bent" would pass it through points closer to the hip
 * than the femur is long, and the lifter's thigh would visibly telescope on the
 * way down into a squat. Angles keep every segment rigid by construction.
 *
 * This module is deliberately free of proportions, anchors and viewBoxes: it
 * knows how to rotate a limb and how to interpolate a pose, and nothing about
 * what a body looks like. That lives in formArchetypes.ts, which is what lets
 * the sagittal and frontal figures be two explicit skeletons rather than one
 * over-general one.
 */

const DEG = Math.PI / 180;

export type Point = { x: number; y: number };

/**
 * A single rendered moment: chains of points to stroke, plus the head and (when
 * the movement is loaded) the implement.
 *
 * `chains` rather than named joints, because the two skeletons do not agree on
 * what joints exist — the frontal figure has two arms and two legs where the
 * sagittal one has a near arm and a near leg. The emitter just strokes whatever
 * chains it is handed, so a differing chain count between views costs nothing.
 */
export type Frame = {
  chains: Point[][];
  head: Point;
  bar?: Point;
};

/** A pose: joint angles in degrees, keyed by whatever the skeleton calls them. */
export type Pose = Record<string, number>;

/** A fixed window onto the world, in world units. */
export type Viewport = {
  minX: number;
  maxX: number;
  minY: number;
  maxY: number;
};

/**
 * A body and the window it is drawn in.
 *
 * The viewport is FIXED per skeleton, never fitted to a pose's own extents.
 * Fitting is the obvious thing to do and it is wrong twice over: within one
 * movement it would rescale between the top and the bottom of the rep, so a
 * squat would read as a lifter who shrinks rather than one who descends; and
 * across the catalogue it would draw every exercise's body at its own arbitrary
 * size, so the figure would jump between exercise pages.
 *
 * There is more than one skeleton for the side view because a standing lifter
 * and a lifter lying on a bench have opposite aspect ratios — a single box wide
 * enough for the hip thrust would draw a squat as a thumbnail in a field of
 * whitespace. Grouping by stance keeps the body one size within each group,
 * which is the property that actually matters.
 */
export type Skeleton = {
  view: Viewport;
  /** Static chains — floor, bench — drawn faintly beneath the figure. */
  scenery: Point[][];
  headRadius: number;
};

/**
 * Angles are absolute and measured the usual mathematical way — 0° is forward
 * (+x), 90° is straight up — in a y-UP world. The single flip to SVG's y-down
 * happens once, at emit time, so every number in an archetype reads the way it
 * would on graph paper.
 */
export function polar(p: Point, deg: number, len: number): Point {
  return { x: p.x + len * Math.cos(deg * DEG), y: p.y + len * Math.sin(deg * DEG) };
}

export type IkResult = {
  /** The middle joint — knee, or elbow. */
  joint: Point;
  /** Where the chain actually ends, which is the target unless it was out of reach. */
  end: Point;
  /** True when the target could not be reached and the chain was straightened at it. */
  clamped: boolean;
};

/**
 * Place the middle joint of a chain whose two ends are both pinned.
 *
 * Needed wherever the chain is CLOSED and angles cannot drive it: the hip
 * thrust, where the foot stays planted on the floor while the shoulders stay on
 * the bench, so the knee is whatever those two constraints leave it. `bend`
 * picks between the two mirror solutions.
 *
 * An unreachable target is clamped to full extension rather than throwing. That
 * is the right behaviour at runtime — a straight limb pointing at the bar reads
 * correctly and an exception does not — but it is a DATA bug if it ever fires
 * for a shipped pose, because it silently means a hand that never reaches its
 * implement. `clamped` is reported so the tests can refuse it.
 */
export function ik(a: Point, b: Point, l1: number, l2: number, bend: 1 | -1): IkResult {
  let dx = b.x - a.x;
  let dy = b.y - a.y;
  let d = Math.hypot(dx, dy);
  const max = l1 + l2;
  const clamped = d > max;
  if (clamped) {
    const s = max / d;
    dx *= s;
    dy *= s;
    d = max;
  }
  const ux = dx / d;
  const uy = dy / d;
  const t = (d * d + l1 * l1 - l2 * l2) / (2 * d);
  const h = Math.sqrt(Math.max(0, l1 * l1 - t * t));
  return {
    joint: { x: a.x + ux * t - uy * h * bend, y: a.y + uy * t + ux * h * bend },
    end: { x: a.x + dx, y: a.y + dy },
    clamped,
  };
}

/** Frames sampled per rep. Enough that linear SMIL interpolation between them is invisible. */
export const FRAMES = 40;

const smoothstep = (t: number) => t * t * (3 - 2 * t);

/**
 * Where in the rep a given moment of the loop sits, 0 at the start pose and 1
 * at the finish.
 *
 * One rep: travel out, a beat at the far end, travel back, a beat at the start. The pauses
 * are what stop the loop reading as a pendulum — without them the figure never
 * settles and the eye cannot find the start of the movement.
 */
export function phase(t: number): number {
  if (t < 0.42) return smoothstep(t / 0.42);
  if (t < 0.5) return 1;
  if (t < 0.92) return 1 - smoothstep((t - 0.5) / 0.42);
  return 0;
}

/**
 * An archetype: one movement pattern, as the two poses it travels between.
 *
 * `start` and `finish` rather than top and bottom, because the catalogue does
 * not agree on which way a rep goes: a squat starts at the top and a press
 * starts at the bottom. Naming them by position would be wrong for half the
 * movements here.
 *
 * `still` names which end of the range is the informative one, and so which
 * single pose gets drawn when the lifter has asked for less motion. The finish
 * is usually where technique goes wrong, but not always — a squat is judged at
 * the bottom and a lateral raise at the top — so it is per-archetype.
 */
export type Archetype<P extends Pose> = {
  skeleton: Skeleton;
  build: (pose: P) => Frame;
  start: P;
  finish: P;
  still?: "start" | "finish";
};

/**
 * Declare an archetype.
 *
 * The single generic `P` across `start` and `finish` is the point of this
 * helper, and it is load-bearing rather than decorative: the sampler
 * interpolates over the keys of `start`, so an angle present there and missing
 * from `finish` would interpolate to NaN and that limb would silently vanish
 * mid-rep. Binding both poses to one type makes it unrepresentable instead of a
 * thing a test has to catch.
 */
export function defineArchetype<P extends Pose>(a: Archetype<P>): Archetype<P> {
  return a;
}

/** Interpolate between two poses. `u` is 0 at `start` and 1 at `finish`. */
export function lerpPose<P extends Pose>(start: P, finish: P, u: number): P {
  const out = {} as Record<string, number>;
  for (const k of Object.keys(start)) out[k] = start[k] + (finish[k] - start[k]) * u;
  return out as P;
}

/** Build one full loop of the movement, eased, as FRAMES frames. */
export function sampleFrames<P extends Pose>(a: Archetype<P>): Frame[] {
  const frames: Frame[] = [];
  for (let i = 0; i < FRAMES; i++) {
    frames.push(a.build(lerpPose(a.start, a.finish, phase(i / FRAMES))));
  }
  return frames;
}

/** The pose drawn when motion is unwanted — the end of the range that teaches. */
export function stillFrame<P extends Pose>(a: Archetype<P>): Frame {
  return a.build(a.still === "start" ? a.start : a.finish);
}

/**
 * An archetype with its pose type sealed away.
 *
 * Each archetype has its own pose shape, so a collection of them is
 * heterogeneous and cannot be typed as `Archetype<Pose>` without a cast that
 * lies (a builder taking a squat's pose is not a builder taking any pose).
 * Closing over the pose at definition time sidesteps that entirely: what
 * escapes is frames, which every archetype agrees about.
 *
 * Both are computed on first use rather than at module load, because the two
 * callers want different things — the library draws 58 still poses and never a
 * single loop.
 */
export type SampledArchetype = {
  skeleton: Skeleton;
  frames: () => Frame[];
  still: () => Frame;
};

export function sampled<P extends Pose>(a: Archetype<P>): SampledArchetype {
  let loop: Frame[] | undefined;
  let still: Frame | undefined;
  return {
    skeleton: a.skeleton,
    frames: () => (loop ??= sampleFrames(a)),
    still: () => (still ??= stillFrame(a)),
  };
}

/**
 * One decimal is plenty at this scale, and it is worth doing for two reasons:
 * it cuts the length of the SMIL `values` strings substantially, and it keeps
 * the ASCII test fixtures stable against floating-point noise that would
 * otherwise rewrite them on an unrelated change.
 */
export const r1 = (n: number): number => Math.round(n * 10) / 10;

/** Every point in a frame, for bounds checks and tests. */
export function pointsOf(f: Frame): Point[] {
  return [...f.chains.flat(), f.head, ...(f.bar ? [f.bar] : [])];
}
