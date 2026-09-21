import { describe, it, expect } from "vitest";

import { ARCHETYPES } from "./formArchetypes";
import { FRAMES, ik, lerpPose, phase, pointsOf, polar, type Point } from "./formKinematics";

const dist = (a: Point, b: Point) => Math.hypot(b.x - a.x, b.y - a.y);
const entries = Object.entries(ARCHETYPES);

describe("polar", () => {
  it("measures 90° as straight up in a y-up world", () => {
    const p = polar({ x: 0, y: 0 }, 90, 10);
    expect(p.x).toBeCloseTo(0);
    expect(p.y).toBeCloseTo(10);
  });
});

describe("ik", () => {
  it("places the joint so both segments keep their length", () => {
    const a = { x: 0, y: 0 };
    const b = { x: 20, y: 0 };
    const { joint, clamped } = ik(a, b, 15, 15, 1);
    expect(clamped).toBe(false);
    expect(dist(a, joint)).toBeCloseTo(15);
    expect(dist(joint, b)).toBeCloseTo(15);
  });

  it("reports a clamp when the target is out of reach", () => {
    const { clamped, end } = ik({ x: 0, y: 0 }, { x: 100, y: 0 }, 15, 15, 1);
    expect(clamped).toBe(true);
    // Straightened at the target rather than thrown: a limb pointing at the bar
    // reads correctly on screen where an exception reads as a blank card.
    expect(end.x).toBeCloseTo(30);
  });
});

describe("phase", () => {
  it("starts and ends at the top so the loop is seamless", () => {
    expect(phase(0)).toBe(0);
    expect(phase(0.999)).toBe(0);
  });

  it("reaches the bottom and holds there", () => {
    expect(phase(0.45)).toBe(1);
  });
});

describe("lerpPose", () => {
  it("interpolates every key of the pose", () => {
    expect(lerpPose({ a: 0, b: 10 }, { a: 100, b: 20 }, 0.5)).toEqual({ a: 50, b: 15 });
  });
});

// These are the tests that actually catch a bad pose. The fixtures in
// __figures__/ show what a movement looks like; these prove things about it
// that no amount of squinting at ASCII would.
describe.each(entries)("%s", (name, archetype) => {
  const frames = archetype.frames();

  it("samples a full loop", () => {
    expect(frames).toHaveLength(FRAMES);
  });

  // The property the whole angle-interpolation design exists to guarantee.
  // Vacuous for a pure forward-kinematics chain — polar() cannot produce a wrong
  // length — but it has real teeth on the IK chains, and it is the tripwire
  // against a future contributor "simplifying" the sampler to interpolate
  // positions, which would quietly shorten every limb mid-rep.
  it("keeps every segment the same length in every frame", () => {
    const first = frames[0].chains.map((chain) =>
      chain.slice(1).map((p, i) => dist(chain[i], p)),
    );
    for (const frame of frames) {
      frame.chains.forEach((chain, ci) => {
        chain.slice(1).forEach((p, si) => {
          expect(dist(chain[si], p)).toBeCloseTo(first[ci][si], 4);
        });
      });
    }
  });

  // A clamp is correct behaviour at runtime and a DATA bug in shipped poses: it
  // means a hand that never reaches its bar. Nothing else would surface it —
  // the figure just draws slightly wrong, forever.
  //
  // Asked of the builder rather than measured off the result, which was this
  // test's first mistake: a clamped chain spans exactly l1 + l2, and so does a
  // legitimately straight arm, so no amount of geometry can separate them.
  it("never asks the IK for something out of reach", () => {
    for (const frame of frames) {
      expect(frame.clamped ?? false).toBe(false);
    }
  });

  // The class of bug the spike's ASCII caught: a head drawn through a bench pad.
  // A joint below the floor is a lifter sunk into the ground.
  it("keeps every joint above the floor", () => {
    for (const frame of frames) {
      for (const p of pointsOf(frame)) {
        expect(p.y).toBeGreaterThanOrEqual(-0.001);
      }
    }
  });

  // A fixed viewBox is only safe if it actually contains the movement; without
  // this the figure would clip mid-rep and only at one end of the range.
  it("stays inside its skeleton's viewBox, head included", () => {
    const { view, headRadius } = archetype.skeleton;
    for (const frame of frames) {
      for (const p of pointsOf(frame)) {
        expect(p.x).toBeGreaterThanOrEqual(view.minX);
        expect(p.x).toBeLessThanOrEqual(view.maxX);
        expect(p.y).toBeGreaterThanOrEqual(view.minY);
        expect(p.y).toBeLessThanOrEqual(view.maxY);
      }
      expect(frame.head.y + headRadius).toBeLessThanOrEqual(view.maxY);
      expect(frame.head.x - headRadius).toBeGreaterThanOrEqual(view.minX);
      expect(frame.head.x + headRadius).toBeLessThanOrEqual(view.maxX);
    }
  });

  it("returns to its starting pose so the loop does not jump", () => {
    const first = pointsOf(frames[0]);
    const last = pointsOf(frames[FRAMES - 1]);
    first.forEach((p, i) => {
      expect(p.x).toBeCloseTo(last[i].x, 4);
      expect(p.y).toBeCloseTo(last[i].y, 4);
    });
  });

  // Either the figure travels, or it is a deliberate hold like the plank where
  // nothing moving IS the demonstration. What would be a bug is the space
  // between: a pose that shifts too little to read as a movement.
  it("either travels a visible distance or holds perfectly still", () => {
    const mid = pointsOf(frames[Math.floor(FRAMES * 0.45)]);
    const travel = Math.max(...pointsOf(frames[0]).map((p, i) => dist(p, mid[i])));
    expect(
      travel === 0 || travel > 4,
      `${name} travels ${travel.toFixed(1)} units — too little to read, too much to be a hold`,
    ).toBe(true);
  });
});
