import { describe, it, expect } from "vitest";

import { asciiFixture } from "./formAscii";
import { exerciseDemo, isDeliberatelyUndrawn } from "./exerciseDemo";
import { SEEDED_EXERCISES } from "./seededExercises";

describe("exerciseDemo", () => {
  it("matches the whole name, not a substring", () => {
    // The distinction this table exists to make: a goblet squat holds the load
    // in front of the chest and a back squat does not, so one must not silently
    // borrow the other's figure just because "squat" appears in both.
    expect(exerciseDemo("Squat")).toBeDefined();
    expect(exerciseDemo("Squat Thrust That Does Not Exist")).toBeUndefined();
  });

  it("ignores case and surrounding space", () => {
    expect(exerciseDemo("  romanian deadlift  ")).toBe(exerciseDemo("Romanian Deadlift"));
  });

  // The open set: exercises.created_by_user_id is non-NULL for a lifter's own
  // movements, which no table can enumerate. Returning undefined is the normal
  // path for those, not an error case.
  it("returns undefined for a movement it has never heard of", () => {
    expect(exerciseDemo("Copenhagen Plank")).toBeUndefined();
    expect(exerciseDemo("")).toBeUndefined();
  });
});

// Every seeded movement either has a figure or is on the record as having been
// refused one. What this cannot catch is a migration adding a 59th exercise —
// see the note at the top of exerciseDemo.ts.
describe("coverage of the seeded catalogue", () => {
  const undrawn = SEEDED_EXERCISES.filter(
    (name) => !exerciseDemo(name) && !isDeliberatelyUndrawn(name),
  );

  it("leaves no seeded exercise silently undrawn", () => {
    expect(undrawn, `no figure and no deny-list entry: ${undrawn.join(", ")}`).toEqual([]);
  });
});

// The fixtures. These are the review surface — there is no browser in this
// devcontainer, so a committed rasterisation is the only way a figure gets
// looked at before it ships. A pose regression lands in the diff as a visibly
// wrong little person.
//
// Written with toMatchFileSnapshot so each exercise owns a readable file rather
// than sharing one unreadable blob. Note these do NOT self-write under CI: a
// new figure's fixture has to be generated locally and committed, or the gate
// fails on a missing snapshot.
describe("figures", () => {
  const drawn = SEEDED_EXERCISES.map((name) => [name, exerciseDemo(name)] as const).filter(
    (pair): pair is readonly [string, NonNullable<ReturnType<typeof exerciseDemo>>] =>
      pair[1] !== undefined,
  );

  it.each(drawn)("draws %s", async (name, demo) => {
    const frames = demo.frames();
    const { skeleton } = demo;
    const art = asciiFixture(name, frames[0], frames[Math.floor(frames.length * 0.45)], skeleton.view, {
      scenery: skeleton.scenery,
      headRadius: skeleton.headRadius,
    });
    const slug = name.toLowerCase().replace(/[^a-z0-9]+/g, "-");
    await expect(art).toMatchFileSnapshot(`./__figures__/${slug}.txt`);
  });
});
