/**
 * The movement patterns, as poses.
 *
 * An ARCHETYPE is one pattern — squat, hinge, press — and the exercises that
 * share it differ only in parameters: which implement is drawn, how steep the
 * bench is, where the hands anchor. That reuse is the point. A stick figure
 * doing a barbell curl and one doing a dumbbell curl differ in the thing held,
 * and nothing else worth drawing; pretending otherwise would mean forty
 * hand-tuned pose tables that nobody can eyeball.
 *
 * Angles are absolute, in degrees, y-UP: 0° points forward, 90° straight up.
 * Read them as you would on graph paper. The lifter faces +x.
 *
 * Every number here was checked by rasterising it — see __figures__/ for what
 * each one actually draws.
 */

import {
  defineArchetype,
  ik,
  polar,
  sampled,
  type Frame,
  type Point,
  type Skeleton,
} from "./formKinematics";

// ---------------------------------------------------------------------------
// Proportions
// ---------------------------------------------------------------------------

/**
 * One body, shared by every side-view skeleton. Roughly human: shank ≈ thigh,
 * and the arm a shade under the torso so the hands reach the hip crease and no
 * further. A standing figure stands about 95 units tall.
 */
export const SEG = {
  shank: 24,
  thigh: 24,
  torso: 28,
  neck: 10,
  headR: 6.5,
  upperArm: 14,
  foreArm: 13,
} as const;

/** The planted foot, shared by every standing movement so the figure never drifts. */
const ANKLE: Point = { x: 0, y: 5 };
const HEEL: Point = { x: -5, y: 0 };
const TOE: Point = { x: 13, y: 0 };
const FOOT: Point[] = [HEEL, ANKLE, TOE];

const floor = (minX: number, maxX: number): Point[][] => [
  [
    { x: minX, y: 0 },
    { x: maxX, y: 0 },
  ],
];

// ---------------------------------------------------------------------------
// Skeletons — a body and the fixed window it is drawn in
// ---------------------------------------------------------------------------

/**
 * Standing lifts: tall and narrow. Wide enough for the hips to travel back into
 * a full hinge and for the hands to hang in front of the shins.
 */
export const STANDING: Skeleton = {
  // Tall enough for an overhead lockout, which is what sets the ceiling: the
  // hand finishes a head's height above the head.
  view: { minX: -38, maxX: 44, minY: -6, maxY: 118 },
  scenery: floor(-38, 44),
  headRadius: SEG.headR,
};

/** Seated machine work — a seat back behind the lifter, legs out in front. */
export const SEATED: Skeleton = {
  view: { minX: -30, maxX: 58, minY: -6, maxY: 92 },
  scenery: [
    ...floor(-30, 58),
    [
      { x: -14, y: 26 },
      { x: 10, y: 26 },
    ],
    [
      { x: -14, y: 26 },
      { x: -18, y: 54 },
    ],
    [
      { x: -2, y: 26 },
      { x: -2, y: 0 },
    ],
  ],
  headRadius: SEG.headR,
};

/**
 * Face-down on the floor: push-ups and planks. Very low and very wide, because
 * a body held in one line is exactly as long as it is tall standing up.
 */
export const FLOOR: Skeleton = {
  view: { minX: -64, maxX: 54, minY: -8, maxY: 44 },
  scenery: floor(-64, 54),
  headRadius: SEG.headR,
};

/**
 * Kneeling core work. A separate skeleton from FLOOR despite both touching the
 * ground: a kneeling lifter is half standing, and sharing a box with the plank
 * would draw them at a quarter the height.
 */
export const KNEELING: Skeleton = {
  view: { minX: -36, maxX: 64, minY: -6, maxY: 82 },
  scenery: floor(-36, 64),
  headRadius: SEG.headR,
};

/** Hanging from a bar overhead, or pulling one down to the chest. */
export const HANGING: Skeleton = {
  // No floor line: a lifter hanging from a bar is not standing on anything, and
  // drawing ground under them would say they were.
  view: { minX: -56, maxX: 60, minY: 14, maxY: 136 },
  scenery: [
    [
      { x: -26, y: 126 },
      { x: 26, y: 126 },
    ],
  ],
  headRadius: SEG.headR,
};

/**
 * Seen from the front, for the movements that happen in the frontal plane.
 *
 * A separate skeleton rather than a parameterised one, because the body is
 * genuinely different: two arms and two legs where the side view has one of
 * each, and no planted-ankle trick since both feet are on the floor. A lateral
 * raise drawn from the side is an arm that appears to grow.
 */
export const FRONT: Skeleton = {
  view: { minX: -46, maxX: 46, minY: -6, maxY: 102 },
  scenery: floor(-46, 46),
  headRadius: SEG.headR,
};

/**
 * Lifts performed lying on a bench: short and wide. The pad sits just under the
 * shoulder so the lifter lies ON it rather than floating above it.
 */
export const SUPINE: Skeleton = {
  view: { minX: -26, maxX: 70, minY: -6, maxY: 58 },
  scenery: [
    ...floor(-26, 70),
    [
      { x: -19, y: 18 },
      { x: 9, y: 18 },
    ],
    [
      { x: -7, y: 18 },
      { x: -7, y: 0 },
    ],
  ],
  headRadius: SEG.headR,
};

// ---------------------------------------------------------------------------
// Builders
// ---------------------------------------------------------------------------

/** The bar on the upper back, offset perpendicular to the torso so it tips with the lifter. */
function barOnBack(shoulder: Point, torsoDeg: number, offset: number): Point {
  const t = (torsoDeg * Math.PI) / 180;
  return { x: shoulder.x - offset * Math.sin(t), y: shoulder.y + offset * Math.cos(t) };
}

type StandingPose = {
  shank: number;
  thigh: number;
  torso: number;
  head: number;
  upperArm: number;
  foreArm: number;
};

/**
 * A standing lift whose arms are carried BY the torso — a squat, where the arms
 * do not move relative to the trunk. Their angles are offsets from the torso.
 */
function squatBody(p: StandingPose & { barOffset: number }): Frame {
  const knee = polar(ANKLE, p.shank, SEG.shank);
  const hip = polar(knee, p.thigh, SEG.thigh);
  const shoulder = polar(hip, p.torso, SEG.torso);
  const elbow = polar(shoulder, p.torso + p.upperArm, SEG.upperArm);
  const hand = polar(elbow, p.torso + p.foreArm, SEG.foreArm);
  return {
    chains: [FOOT, [ANKLE, knee, hip], [hip, shoulder], [shoulder, elbow, hand]],
    head: polar(shoulder, p.head, SEG.neck),
    bar: barOnBack(shoulder, p.torso, p.barOffset),
  };
}

/**
 * A standing lift whose arms HANG — a hinge, where the bar tracks down the
 * thigh because gravity, not the torso, decides where the arms point. Their
 * angles are absolute and ignore the trunk entirely, which is the whole
 * difference between this and squatBody.
 */
function hangingBody(p: StandingPose): Frame {
  const knee = polar(ANKLE, p.shank, SEG.shank);
  const hip = polar(knee, p.thigh, SEG.thigh);
  const shoulder = polar(hip, p.torso, SEG.torso);
  const elbow = polar(shoulder, p.upperArm, SEG.upperArm);
  const hand = polar(elbow, p.foreArm, SEG.foreArm);
  return {
    chains: [FOOT, [ANKLE, knee, hip], [hip, shoulder], [shoulder, elbow, hand]],
    head: polar(shoulder, p.head, SEG.neck),
    bar: hand,
  };
}

/**
 * The hip thrust: rooted at the SHOULDER, because the shoulder is what is
 * pinned. One angle drives the torso and the knee falls out of IK against a
 * foot that must not slide — the one closed chain in the catalogue, and the
 * reason ik() exists at all.
 */
function hipThrustBody(p: { torso: number; head: number }): Frame {
  const shoulder: Point = { x: 0, y: 22 };
  const hip = polar(shoulder, p.torso, SEG.torso);
  const ankle: Point = { x: 48, y: 5 };
  const leg = ik(hip, ankle, SEG.thigh, SEG.shank, 1);
  const knee = leg.joint;
  // The bar rests across the hip CREASE, not the hip joint — a little up the
  // body from it. That is where hands grip it, and it is also the only place
  // they can: the arm is marginally shorter than the torso, so a bar drawn at
  // the joint itself sat just out of reach and the IK quietly straightened the
  // arm at it. The clamp test is what surfaced that.
  const bar = barOnBack(polar(shoulder, p.torso, SEG.torso * 0.86), p.torso, -5);
  const arm = ik(shoulder, bar, SEG.upperArm, SEG.foreArm, -1);
  return {
    chains: [
      [shoulder, hip, knee],
      [knee, ankle],
      [ankle, { x: ankle.x + 11, y: 0 }],
      [shoulder, arm.joint, arm.end],
    ],
    head: polar(shoulder, p.head, SEG.neck),
    bar,
    clamped: leg.clamped || arm.clamped,
  };
}

type UprightPose = { torso: number; head: number; upperArm: number; foreArm: number };

/**
 * A standing lift where only the arms work — curls, presses, pushdowns, pulls.
 *
 * One builder covers a third of the catalogue because from the side these
 * movements genuinely differ in nothing but where the arms point. The legs are
 * straight and the torso is upright by construction; what varies is two angles.
 */
function uprightBody(p: UprightPose): Frame {
  const knee = polar(ANKLE, 90, SEG.shank);
  const hip = polar(knee, 90, SEG.thigh);
  const shoulder = polar(hip, p.torso, SEG.torso);
  const elbow = polar(shoulder, p.upperArm, SEG.upperArm);
  const hand = polar(elbow, p.foreArm, SEG.foreArm);
  return {
    chains: [FOOT, [ANKLE, knee, hip], [hip, shoulder], [shoulder, elbow, hand]],
    head: polar(shoulder, p.head, SEG.neck),
    bar: hand,
  };
}

/** A split stance: the rear leg gets its own chain from the same hip. */
function splitBody(p: {
  shank: number;
  thigh: number;
  torso: number;
  head: number;
  rearThigh: number;
  rearShank: number;
}): Frame {
  const knee = polar(ANKLE, p.shank, SEG.shank);
  const hip = polar(knee, p.thigh, SEG.thigh);
  const shoulder = polar(hip, p.torso, SEG.torso);
  const rearKnee = polar(hip, p.rearThigh, SEG.thigh);
  const rearAnkle = polar(rearKnee, p.rearShank, SEG.shank);
  return {
    chains: [
      FOOT,
      [ANKLE, knee, hip],
      [hip, rearKnee, rearAnkle],
      [hip, shoulder],
      [
        shoulder,
        polar(shoulder, 270, SEG.upperArm),
        polar(polar(shoulder, 270, SEG.upperArm), 270, SEG.foreArm),
      ],
    ],
    head: polar(shoulder, p.head, SEG.neck),
  };
}

/** Rotate a point about a pivot. Used where a rigid part pivots rather than bends. */
function rotateAbout(p: Point, pivot: Point, deg: number): Point {
  const r = (deg * Math.PI) / 180;
  const dx = p.x - pivot.x;
  const dy = p.y - pivot.y;
  return {
    x: pivot.x + dx * Math.cos(r) - dy * Math.sin(r),
    y: pivot.y + dx * Math.sin(r) + dy * Math.cos(r),
  };
}

/**
 * Calf raise: the foot pivots about the toe and carries the body up with it.
 *
 * The foot ROTATES rather than deforming. Lifting the heel by translating it
 * was the obvious first attempt and it stretched the foot a little further on
 * every frame — which the segment-length test refused, correctly.
 */
function calfBody(p: { pitch: number; head: number }): Frame {
  const ankle = rotateAbout(ANKLE, TOE, p.pitch);
  const heel = rotateAbout(HEEL, TOE, p.pitch);
  const knee = polar(ankle, 90, SEG.shank);
  const hip = polar(knee, 90, SEG.thigh);
  const shoulder = polar(hip, 90, SEG.torso);
  const elbow = polar(shoulder, 272, SEG.upperArm);
  return {
    chains: [
      [heel, ankle, TOE],
      [ankle, knee, hip],
      [hip, shoulder],
      [shoulder, elbow, polar(elbow, 272, SEG.foreArm)],
    ],
    head: polar(shoulder, p.head, SEG.neck),
  };
}

/** The lifter's body on the bench: shoulder pinned to the pad, feet on the floor. */
function benchFrame(incline: number) {
  const shoulder: Point = { x: 0, y: 22 };
  const hip = polar(shoulder, -incline, SEG.torso);
  // Swept with the incline: on a flat bench the thigh drops toward the floor,
  // and on an incline the hips are already high enough that the same angle
  // would bury the knee underground.
  const knee = polar(hip, -40 + incline, SEG.thigh);
  const ankle: Point = { x: knee.x + 4, y: 5 };
  return { shoulder, hip, knee, ankle };
}

/**
 * A press on the bench, driven by where the BAR is rather than by arm angles.
 *
 * The bar path is the whole point of a press, and angles cannot state it: an
 * arm swung from the shoulder traces an arc, so the first version of this
 * pressed the bar diagonally up from somewhere around the lifter's knees.
 * Giving IK the hand position and letting the elbow fall out makes the bar
 * travel straight up and down, which is what a press actually looks like and
 * what the lifter is being shown.
 */
function benchPressBody(p: { barX: number; barY: number; incline: number }): Frame {
  const { shoulder, hip, knee, ankle } = benchFrame(p.incline);
  const bar: Point = { x: p.barX, y: p.barY };
  // bend -1 puts the elbow on the FEET side of the shoulder and below it, which
  // is the only way a shoulder bends. The mirror solution is geometrically just
  // as valid and anatomically impossible: it folds the upper arm up and back
  // over the lifter's head while the hand stays out in front. Both keep every
  // segment its proper length, so neither the length test nor the clamp test
  // has anything to say about it — see the elbow test in formKinematics.test.ts.
  const arm = ik(shoulder, bar, SEG.upperArm, SEG.foreArm, -1);
  return {
    chains: [
      [shoulder, hip, knee],
      [knee, ankle],
      [ankle, { x: ankle.x + 11, y: 0 }],
      [shoulder, arm.joint, arm.end],
    ],
    head: polar(shoulder, 165, SEG.neck),
    bar: arm.end,
    clamped: arm.clamped,
  };
}

/** Lying on the bench with the elbow held still — the skull crusher's shape. */
function benchBody(p: { upperArm: number; foreArm: number; incline: number; head: number }): Frame {
  const { shoulder, hip, knee, ankle } = benchFrame(p.incline);
  const elbow = polar(shoulder, p.upperArm, SEG.upperArm);
  const hand = polar(elbow, p.foreArm, SEG.foreArm);
  return {
    chains: [
      [shoulder, hip, knee],
      [knee, ankle],
      [ankle, { x: ankle.x + 11, y: 0 }],
      [shoulder, elbow, hand],
    ],
    head: polar(shoulder, 165, SEG.neck),
    bar: hand,
  };
}

/**
 * Face-down on the floor, held rigid from the shoulder: push-ups and planks.
 *
 * Built UP from the planted hand rather than down from the shoulder, because
 * the hand is what cannot move. Each segment chains off the last — computing
 * the shoulder straight from the hand across a combined arm length is what a
 * first attempt does, and it lets the elbow drift off the arm entirely.
 */
function floorBody(p: { upperArm: number; foreArm: number; body: number; head: number }): Frame {
  const hand: Point = { x: 38, y: 2 };
  const elbow = polar(hand, p.foreArm, SEG.foreArm);
  const shoulder = polar(elbow, p.upperArm, SEG.upperArm);
  const hip = polar(shoulder, p.body, SEG.torso);
  const knee = polar(hip, p.body, SEG.thigh);
  const ankle = polar(knee, p.body - 8, SEG.shank);
  return {
    chains: [[hand, elbow, shoulder], [shoulder, hip, knee, ankle]],
    head: polar(shoulder, p.head, SEG.neck),
  };
}

/** Hanging from the bar, or pulling it down. */
function hangBody(p: {
  upperArm: number;
  foreArm: number;
  torso: number;
  thigh: number;
  shank: number;
  head: number;
}): Frame {
  const hand: Point = { x: 0, y: 126 };
  const elbow = polar(hand, p.foreArm, SEG.foreArm);
  const shoulder = polar(elbow, p.upperArm, SEG.upperArm);
  const hip = polar(shoulder, p.torso, SEG.torso);
  const knee = polar(hip, p.thigh, SEG.thigh);
  const ankle = polar(knee, p.shank, SEG.shank);
  return {
    chains: [
      [hand, elbow, shoulder],
      [shoulder, hip, knee, ankle],
    ],
    head: polar(shoulder, p.head, SEG.neck),
    bar: hand,
  };
}

/** Seated on a machine: back against the pad, legs working in front. */
function seatedBody(p: {
  thigh: number;
  shank: number;
  torso: number;
  upperArm: number;
  foreArm: number;
  head: number;
}): Frame {
  const hip: Point = { x: 0, y: 28 };
  const knee = polar(hip, p.thigh, SEG.thigh);
  const ankle = polar(knee, p.shank, SEG.shank);
  const shoulder = polar(hip, p.torso, SEG.torso);
  const elbow = polar(shoulder, p.upperArm, SEG.upperArm);
  const hand = polar(elbow, p.foreArm, SEG.foreArm);
  return {
    chains: [
      [hip, knee, ankle],
      [hip, shoulder],
      [shoulder, elbow, hand],
    ],
    head: polar(shoulder, p.head, SEG.neck),
    bar: hand,
  };
}

/** Kneeling: shin flat on the floor, the work happening above the hip. */
function kneelBody(p: { torso: number; upperArm: number; foreArm: number; head: number }): Frame {
  const knee: Point = { x: 0, y: 3 };
  const ankle: Point = { x: -SEG.shank, y: 3 };
  const hip = polar(knee, 90, SEG.thigh);
  const shoulder = polar(hip, p.torso, SEG.torso);
  const elbow = polar(shoulder, p.upperArm, SEG.upperArm);
  const hand = polar(elbow, p.foreArm, SEG.foreArm);
  return {
    chains: [
      [ankle, knee],
      [knee, hip],
      [hip, shoulder],
      [shoulder, elbow, hand],
    ],
    head: polar(shoulder, p.head, SEG.neck),
    bar: hand,
  };
}

/**
 * Seen from the front. Both limbs are drawn, mirrored about the midline, which
 * is the entire reason this view exists — the movements that use it are the
 * ones whose travel is sideways and therefore invisible from the side.
 */
function frontBody(p: {
  arm: number;
  foreArm: number;
  legR: number;
  legL: number;
  head: number;
}): Frame {
  const hipW = 9;
  const shoulderW = 13;
  const midHip: Point = { x: 0, y: 53 };
  const neckBase: Point = { x: 0, y: 81 };
  const limbs: Point[][] = [];
  for (const side of [-1, 1] as const) {
    // Arm angles are written for the RIGHT arm and mirrored as (180 - deg),
    // which is what makes "straight down" (270) stay down on both sides while
    // "straight out" (0) becomes 180 on the left. Reflecting about the vertical
    // instead sends both arms inward across the chest, and a lateral raise
    // drawn that way is a figure hugging itself.
    const armDeg = side === 1 ? p.arm : 180 - p.arm;
    const foreDeg = side === 1 ? p.foreArm : 180 - p.foreArm;
    const shoulder: Point = { x: side * shoulderW, y: 81 };
    const elbow = polar(shoulder, armDeg, SEG.upperArm);
    const hand = polar(elbow, foreDeg, SEG.foreArm);
    limbs.push([shoulder, elbow, hand]);
    // Legs are per-side rather than mirrored, because the two band movements
    // differ in exactly this: a lateral walk widens the whole stance, and an
    // abduction drives ONE leg out. Mirroring both made them the same picture.
    const legDeg = 270 + side * (side === 1 ? p.legR : p.legL);
    const hip: Point = { x: side * hipW, y: 53 };
    const knee = polar(hip, legDeg, SEG.thigh);
    const ankle = polar(knee, legDeg, SEG.shank);
    limbs.push([hip, knee, ankle]);
  }
  return {
    chains: [
      [midHip, neckBase],
      [
        { x: -shoulderW, y: 81 },
        { x: shoulderW, y: 81 },
      ],
      [
        { x: -hipW, y: 53 },
        { x: hipW, y: 53 },
      ],
      ...limbs,
    ],
    head: polar(neckBase, p.head, SEG.neck),
  };
}

// ---------------------------------------------------------------------------
// Archetypes
// ---------------------------------------------------------------------------

/**
 * Squat. The shin travels forward, the thigh passes horizontal so the hip
 * finishes below the knee, and the torso leans just enough to keep the bar over
 * midfoot — which is the check worth making on the fixture: the bar should fall
 * almost dead vertical.
 */
const squat = defineArchetype({
  skeleton: STANDING,
  build: (p: StandingPose & { barOffset: number }) => squatBody(p),
  start: { shank: 90, thigh: 90, torso: 90, head: 90, upperArm: 160, foreArm: 10, barOffset: 3.5 },
  finish: { shank: 68, thigh: 186, torso: 62, head: 76, upperArm: 160, foreArm: 10, barOffset: 3.5 },
});

/**
 * Front squat, as the same movement with the bar racked in front. A front squat
 * is genuinely more upright than a back squat — the load in front of the body
 * is what demands it — so the torso is not simply copied.
 */
const frontSquat = defineArchetype({
  skeleton: STANDING,
  build: (p: StandingPose & { barOffset: number }) => squatBody(p),
  start: { shank: 90, thigh: 90, torso: 90, head: 90, upperArm: 200, foreArm: 70, barOffset: -4 },
  finish: { shank: 66, thigh: 184, torso: 74, head: 84, upperArm: 200, foreArm: 70, barOffset: -4 },
});

/**
 * Romanian deadlift. Soft knee, hips travelling back, torso hinging to roughly
 * 25° above horizontal. The arms stay vertical throughout — they are a plumb
 * line, not a lever, and the fixture is wrong if they ever swing.
 */
const romanianDeadlift = defineArchetype({
  skeleton: STANDING,
  build: hangingBody,
  start: { shank: 90, thigh: 90, torso: 90, head: 90, upperArm: 270, foreArm: 270 },
  finish: { shank: 96, thigh: 142, torso: 25, head: 40, upperArm: 270, foreArm: 270 },
});

/**
 * Deadlift from the floor. Unlike the RDL this starts at the bottom — the bar
 * begins on the ground — so the "top" pose is the pull's finish and the
 * "bottom" is the setup. The knee bends far more than a hinge: this is a squat
 * and a hinge at once.
 */
const deadlift = defineArchetype({
  skeleton: STANDING,
  build: hangingBody,
  start: { shank: 90, thigh: 90, torso: 90, head: 90, upperArm: 270, foreArm: 270 },
  finish: { shank: 76, thigh: 158, torso: 38, head: 54, upperArm: 270, foreArm: 270 },
  still: "finish",
});

/**
 * Barbell hip thrust. Lockout is shoulder, hip and knee on one line over a
 * vertical shin; the bottom drops the hips toward the floor with the feet
 * planted. The head rests ON the pad — at 165° the skull's underside lands at
 * the bench top, where pointing it down the way a standing lift does would bury
 * it in the bench.
 */
const hipThrust = defineArchetype({
  skeleton: SUPINE,
  build: hipThrustBody,
  start: { torso: 8, head: 165 },
  finish: { torso: -34, head: 165 },
  still: "start",
});

/** Goblet squat: the load is held at the chest, which keeps the torso upright. */
const gobletSquat = defineArchetype({
  skeleton: STANDING,
  build: (p: StandingPose & { barOffset: number }) => squatBody(p),
  start: { shank: 90, thigh: 90, torso: 90, head: 90, upperArm: 215, foreArm: 55, barOffset: -6 },
  finish: { shank: 65, thigh: 183, torso: 76, head: 86, upperArm: 215, foreArm: 55, barOffset: -6 },
});

/** Split squat: the rear knee travels to the floor while the front shin stays near vertical. */
const splitSquat = defineArchetype({
  skeleton: STANDING,
  build: splitBody,
  start: { shank: 92, thigh: 90, torso: 90, head: 90, rearThigh: 258, rearShank: 292 },
  finish: { shank: 86, thigh: 150, torso: 84, head: 84, rearThigh: 268, rearShank: 348 },
});

/** Calf raise: a small movement, but the heel leaving the floor is unmistakable. */
const calfRaise = defineArchetype({
  skeleton: STANDING,
  build: calfBody,
  start: { pitch: 0, head: 90 },
  finish: { pitch: -26, head: 90 },
  still: "finish",
});

/** Curl: the elbow stays at the ribs and only the forearm travels. */
const curl = defineArchetype({
  skeleton: STANDING,
  build: uprightBody,
  start: { torso: 90, head: 90, upperArm: 270, foreArm: 272 },
  finish: { torso: 90, head: 90, upperArm: 265, foreArm: 55 },
  still: "finish",
});

/** Preacher curl: the same movement with the upper arm pinned forward on a pad. */
const preacherCurl = defineArchetype({
  skeleton: STANDING,
  build: uprightBody,
  start: { torso: 78, head: 96, upperArm: 310, foreArm: 312 },
  finish: { torso: 78, head: 96, upperArm: 310, foreArm: 40 },
  still: "finish",
});

/** Overhead press: from the rack position at the shoulder to a locked-out arm. */
const overheadPress = defineArchetype({
  skeleton: STANDING,
  build: uprightBody,
  start: { torso: 90, head: 98, upperArm: 235, foreArm: 75 },
  finish: { torso: 90, head: 104, upperArm: 80, foreArm: 84 },
  still: "finish",
});

/** Triceps pushdown: elbow pinned at the side, forearm sweeping down. */
const pushdown = defineArchetype({
  skeleton: STANDING,
  build: uprightBody,
  start: { torso: 86, head: 94, upperArm: 268, foreArm: 40 },
  finish: { torso: 86, head: 94, upperArm: 268, foreArm: 272 },
  still: "finish",
});

/** Overhead triceps extension: the elbow stays up and the forearm drops behind the head. */
const overheadExtension = defineArchetype({
  skeleton: STANDING,
  build: uprightBody,
  start: { torso: 90, head: 90, upperArm: 95, foreArm: 200 },
  finish: { torso: 90, head: 90, upperArm: 95, foreArm: 92 },
  still: "start",
});

/** Upright row: the elbows lead, rising in front of the body above the hands. */
const uprightRow = defineArchetype({
  skeleton: STANDING,
  build: uprightBody,
  start: { torso: 90, head: 90, upperArm: 270, foreArm: 272 },
  finish: { torso: 90, head: 90, upperArm: 20, foreArm: 200 },
  still: "finish",
});

/** Face pull: hands finish beside the head with the elbows high and wide. */
const facePull = defineArchetype({
  skeleton: STANDING,
  build: uprightBody,
  start: { torso: 90, head: 90, upperArm: 8, foreArm: 8 },
  finish: { torso: 90, head: 90, upperArm: 30, foreArm: 150 },
  still: "finish",
});

/** Bent-over row: the torso holds its angle and the bar travels to the ribs. */
const row = defineArchetype({
  skeleton: STANDING,
  build: hangingBody,
  start: { shank: 92, thigh: 145, torso: 30, head: 45, upperArm: 270, foreArm: 270 },
  finish: { shank: 92, thigh: 145, torso: 30, head: 45, upperArm: 200, foreArm: 320 },
  still: "finish",
});

/** Banded kickback: the working leg drives straight back from a braced trunk. */
const kickback = defineArchetype({
  skeleton: STANDING,
  build: splitBody,
  start: { shank: 90, thigh: 90, torso: 70, head: 60, rearThigh: 265, rearShank: 275 },
  finish: { shank: 90, thigh: 90, torso: 70, head: 60, rearThigh: 240, rearShank: 250 },
  still: "finish",
});

/** Back extension: hinged over a pad, the trunk rising to a straight line. */
const backExtension = defineArchetype({
  skeleton: STANDING,
  build: hangingBody,
  start: { shank: 90, thigh: 90, torso: 30, head: 40, upperArm: 30, foreArm: 90 },
  finish: { shank: 90, thigh: 90, torso: 86, head: 86, upperArm: 30, foreArm: 90 },
  still: "start",
});

/** Bench press: flat, the bar travelling from the chest to a locked-out arm. */
const benchPress = defineArchetype({
  skeleton: SUPINE,
  build: benchPressBody,
  // Starts locked out and lowers to the chest, because that is the order a rep
  // happens in: the bar is already overhead when it is unracked. barX holds
  // still so the bar goes straight up and down.
  start: { barX: 8, barY: 47, incline: 0 },
  finish: { barX: 8, barY: 29, incline: 0 },
  still: "finish",
});

/** Incline press: the same movement with the bench tilted up under the lifter. */
const inclinePress = defineArchetype({
  skeleton: SUPINE,
  build: benchPressBody,
  start: { barX: 14, barY: 44, incline: 28 },
  finish: { barX: 14, barY: 28, incline: 28 },
  still: "finish",
});

/** Skull crusher: the upper arm holds still and the forearm folds past the head. */
const skullCrusher = defineArchetype({
  skeleton: SUPINE,
  build: benchBody,
  start: { upperArm: 80, foreArm: 175, incline: 0, head: 165 },
  finish: { upperArm: 80, foreArm: 88, incline: 0, head: 165 },
  still: "start",
});

/** Banded glute bridge: the hip thrust done from the floor, so a shorter range. */
const gluteBridge = defineArchetype({
  skeleton: SUPINE,
  build: hipThrustBody,
  start: { torso: -2, head: 178 },
  finish: { torso: -34, head: 178 },
  still: "start",
});

/** Push-up: the body holds one rigid line while the elbows bend. */
const pushUp = defineArchetype({
  skeleton: FLOOR,
  build: floorBody,
  start: { upperArm: 105, foreArm: 105, body: 188, head: 12 },
  finish: { upperArm: 150, foreArm: 105, body: 188, head: 8 },
  still: "finish",
});

/**
 * Plank. Start and finish are the same pose, which is the correct
 * demonstration of a static hold — and the emitter's "nothing changes, so emit
 * no animation" rule makes it a still image for free.
 */
const plank = defineArchetype({
  skeleton: FLOOR,
  build: floorBody,
  start: { upperArm: 95, foreArm: 180, body: 190, head: 10 },
  finish: { upperArm: 95, foreArm: 180, body: 190, head: 10 },
});

/** Dip: the torso stays upright between the bars and drops straight down. */
const dip = defineArchetype({
  skeleton: HANGING,
  build: hangBody,
  start: { upperArm: 272, foreArm: 272, torso: 272, thigh: 300, shank: 340, head: 88 },
  finish: { upperArm: 232, foreArm: 300, torso: 272, thigh: 300, shank: 340, head: 88 },
  still: "finish",
});

/** Vertical pull: hanging at full stretch, then chin to the bar. */
const verticalPull = defineArchetype({
  skeleton: HANGING,
  build: hangBody,
  start: { upperArm: 272, foreArm: 272, torso: 272, thigh: 272, shank: 272, head: 86 },
  finish: { upperArm: 228, foreArm: 292, torso: 272, thigh: 285, shank: 320, head: 86 },
  still: "finish",
});

/** Lat pulldown: the same pull with the lifter seated and the bar travelling instead. */
const latPulldown = defineArchetype({
  skeleton: HANGING,
  build: hangBody,
  start: { upperArm: 268, foreArm: 268, torso: 274, thigh: 340, shank: 290, head: 84 },
  finish: { upperArm: 225, foreArm: 290, torso: 274, thigh: 340, shank: 290, head: 84 },
  still: "finish",
});

/** Hanging leg raise: the trunk hangs still and the legs come up in front. */
const hangingLegRaise = defineArchetype({
  skeleton: HANGING,
  build: hangBody,
  start: { upperArm: 272, foreArm: 272, torso: 272, thigh: 272, shank: 272, head: 86 },
  finish: { upperArm: 272, foreArm: 272, torso: 272, thigh: 2, shank: 2, head: 86 },
  still: "finish",
});

/** Leg press: the knees fold toward the chest and drive back out. */
const legPress = defineArchetype({
  skeleton: SEATED,
  build: seatedBody,
  start: { thigh: 8, shank: 6, torso: 108, upperArm: 340, foreArm: 340, head: 100 },
  finish: { thigh: 40, shank: 300, torso: 108, upperArm: 340, foreArm: 340, head: 100 },
  still: "finish",
});

/** Leg extension: the thigh is fixed and the shin swings up to straight. */
const legExtension = defineArchetype({
  skeleton: SEATED,
  build: seatedBody,
  start: { thigh: 0, shank: 280, torso: 92, upperArm: 300, foreArm: 300, head: 90 },
  finish: { thigh: 0, shank: 2, torso: 92, upperArm: 300, foreArm: 300, head: 90 },
  still: "finish",
});

/** Leg curl: seated, the shin sweeping back under the seat. */
const legCurl = defineArchetype({
  skeleton: SEATED,
  build: seatedBody,
  start: { thigh: 0, shank: 350, torso: 92, upperArm: 300, foreArm: 300, head: 90 },
  finish: { thigh: 0, shank: 265, torso: 92, upperArm: 300, foreArm: 300, head: 90 },
  still: "finish",
});

/** Seated cable row: the torso stays tall and the handle travels to the waist. */
const seatedRow = defineArchetype({
  skeleton: SEATED,
  build: seatedBody,
  start: { thigh: 4, shank: 320, torso: 78, upperArm: 8, foreArm: 8, head: 96 },
  finish: { thigh: 4, shank: 320, torso: 95, upperArm: 200, foreArm: 330, head: 84 },
  still: "finish",
});

/** Machine chest press: seated, pressing straight out in front. */
const machinePress = defineArchetype({
  skeleton: SEATED,
  build: seatedBody,
  start: { thigh: 4, shank: 300, torso: 96, upperArm: 190, foreArm: 330, head: 88 },
  finish: { thigh: 4, shank: 300, torso: 96, upperArm: 6, foreArm: 4, head: 88 },
  still: "start",
});

/** Lateral raise: the arms travel out to the sides, which only the front view shows. */
const lateralRaise = defineArchetype({
  skeleton: FRONT,
  build: frontBody,
  // 362 rather than 2, and the difference is the whole movement: angles
  // interpolate numerically, so sweeping 272 down to 2 would raise the arm
  // through straight-up and lower it back down. 272 to 362 takes the arc a
  // shoulder actually travels.
  start: { arm: 272, foreArm: 272, legR: 3, legL: 3, head: 90 },
  finish: { arm: 362, foreArm: 362, legR: 3, legL: 3, head: 90 },
  still: "finish",
});

/** Cable fly: the arms sweep in toward each other across the front of the chest. */
const cableFly = defineArchetype({
  skeleton: FRONT,
  build: frontBody,
  start: { arm: 5, foreArm: 5, legR: 3, legL: 3, head: 90 },
  finish: { arm: 22, foreArm: 148, legR: 3, legL: 3, head: 90 },
  still: "start",
});

/** Banded lateral walk: a half-squat, the stance widening against the band. */
const bandWalk = defineArchetype({
  skeleton: FRONT,
  build: frontBody,
  start: { arm: 250, foreArm: 250, legR: 5, legL: 5, head: 90 },
  finish: { arm: 250, foreArm: 250, legR: 24, legL: 24, head: 90 },
  still: "finish",
});

/** Banded hip abduction: standing tall, one leg driving out against the band. */
const hipAbduction = defineArchetype({
  skeleton: FRONT,
  build: frontBody,
  start: { arm: 265, foreArm: 265, legR: 2, legL: 2, head: 90 },
  finish: { arm: 265, foreArm: 265, legR: 30, legL: 2, head: 90 },
  still: "finish",
});

/** Cable crunch: kneeling, the trunk folding down over a fixed hip. */
const cableCrunch = defineArchetype({
  skeleton: KNEELING,
  build: kneelBody,
  start: { torso: 84, upperArm: 120, foreArm: 60, head: 80 },
  finish: { torso: 36, upperArm: 120, foreArm: 60, head: 20 },
  still: "finish",
});

/** Ab wheel rollout: kneeling, the arms reaching away until the body is one line. */
const abWheel = defineArchetype({
  skeleton: KNEELING,
  build: kneelBody,
  start: { torso: 70, upperArm: 280, foreArm: 300, head: 60 },
  finish: { torso: 22, upperArm: 8, foreArm: 4, head: 14 },
  still: "finish",
});

/**
 * Every archetype, by key, with its pose type sealed by sampled(). That is what
 * lets this be one collection despite each entry having its own pose shape —
 * see SampledArchetype.
 */
export const ARCHETYPES = {
  squat: sampled(squat),
  frontSquat: sampled(frontSquat),
  gobletSquat: sampled(gobletSquat),
  splitSquat: sampled(splitSquat),
  calfRaise: sampled(calfRaise),
  romanianDeadlift: sampled(romanianDeadlift),
  deadlift: sampled(deadlift),
  hipThrust: sampled(hipThrust),
  gluteBridge: sampled(gluteBridge),
  kickback: sampled(kickback),
  backExtension: sampled(backExtension),
  curl: sampled(curl),
  preacherCurl: sampled(preacherCurl),
  overheadPress: sampled(overheadPress),
  pushdown: sampled(pushdown),
  overheadExtension: sampled(overheadExtension),
  uprightRow: sampled(uprightRow),
  facePull: sampled(facePull),
  row: sampled(row),
  benchPress: sampled(benchPress),
  inclinePress: sampled(inclinePress),
  skullCrusher: sampled(skullCrusher),
  pushUp: sampled(pushUp),
  plank: sampled(plank),
  dip: sampled(dip),
  verticalPull: sampled(verticalPull),
  latPulldown: sampled(latPulldown),
  hangingLegRaise: sampled(hangingLegRaise),
  legPress: sampled(legPress),
  legExtension: sampled(legExtension),
  legCurl: sampled(legCurl),
  seatedRow: sampled(seatedRow),
  machinePress: sampled(machinePress),
  lateralRaise: sampled(lateralRaise),
  cableFly: sampled(cableFly),
  bandWalk: sampled(bandWalk),
  hipAbduction: sampled(hipAbduction),
  cableCrunch: sampled(cableCrunch),
  abWheel: sampled(abWheel),
};

export type ArchetypeName = keyof typeof ARCHETYPES;
