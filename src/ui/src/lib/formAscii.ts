/**
 * Rasterise a figure to ASCII.
 *
 * This exists because there is no browser in the devcontainer, so there is
 * otherwise no way to look at a stick figure before shipping it. The test suite
 * writes one fixture per exercise into __figures__/, which makes the committed
 * fixtures the review surface: a pose regression arrives in a diff as a visibly
 * wrong little person rather than as a thousand changed decimals.
 *
 * It earned its place on the spike, where it caught a lifter's head being drawn
 * straight through a bench pad — a bug the pose angles looked entirely fine
 * about.
 *
 * Kept out of the component's import graph on purpose: nothing in the running
 * app rasterises anything, and this should never reach a bundle.
 */

import type { Frame, Point, Viewport } from "./formKinematics";

const DEG = Math.PI / 180;

export type AsciiOptions = {
  /** Extra static chains — floor, bench — drawn faintly under the figure. */
  scenery?: Point[][];
  headRadius: number;
  width?: number;
  height?: number;
};

/**
 * Draw one frame at a FIXED viewport.
 *
 * Fixed rather than fitted to the frame's own extents: a viewport computed per
 * pose would silently rescale between the top and the bottom of a rep, so a
 * squat would render as a lifter who shrinks and grows rather than one who
 * descends. The same mistake at the SVG level would make every exercise page
 * draw the body at a different size.
 */
export function asciiFrame(
  frame: Frame,
  view: Viewport,
  { scenery = [], headRadius, width = 56, height = 30 }: AsciiOptions,
): string {
  const grid: string[][] = Array.from({ length: height }, () => Array(width).fill(" "));

  const scale = Math.min(
    (width - 1) / (view.maxX - view.minX),
    (height - 1) / (view.maxY - view.minY),
  );
  const offsetX = (width - (view.maxX - view.minX) * scale) / 2;
  // World is y-up, the grid is y-down; this is the same single flip the SVG
  // emitter performs.
  const map = (p: Point): [number, number] => [
    Math.round((p.x - view.minX) * scale + offsetX),
    Math.round((view.maxY - p.y) * scale),
  ];

  const plot = (x: number, y: number, ch: string) => {
    if (x >= 0 && x < width && y >= 0 && y < height) grid[y][x] = ch;
  };

  const stroke = (a: Point, b: Point, ch: string) => {
    const [x0, y0] = map(a);
    const [x1, y1] = map(b);
    const steps = Math.max(Math.abs(x1 - x0), Math.abs(y1 - y0), 1);
    for (let i = 0; i <= steps; i++) {
      plot(Math.round(x0 + ((x1 - x0) * i) / steps), Math.round(y0 + ((y1 - y0) * i) / steps), ch);
    }
  };

  for (const chain of scenery) {
    for (let i = 0; i < chain.length - 1; i++) stroke(chain[i], chain[i + 1], ".");
  }
  for (const chain of frame.chains) {
    for (let i = 0; i < chain.length - 1; i++) stroke(chain[i], chain[i + 1], "*");
  }

  const [hx, hy] = map(frame.head);
  const hr = Math.round(headRadius * scale);
  for (let a = 0; a < 360; a += 10) {
    // Squashed vertically because a terminal cell is about twice as tall as it
    // is wide, and a mathematically round head renders as an oval.
    plot(Math.round(hx + hr * Math.cos(a * DEG)), Math.round(hy + hr * Math.sin(a * DEG) * 0.55), "o");
  }

  if (frame.bar) {
    const [bx, by] = map(frame.bar);
    plot(bx - 1, by, "#");
    plot(bx, by, "#");
    plot(bx + 1, by, "#");
  }

  return grid.map((row) => row.join("").replace(/\s+$/, "")).join("\n");
}

/**
 * The committed fixture for one exercise: its two informative poses, labelled.
 *
 * The two ends of the range only. All forty frames would be forty rasterisations nobody
 * reads; the frames in between are the property tests' job, which is the right
 * division of labour between a test that proves something and a fixture that
 * shows something.
 */
export function asciiFixture(
  name: string,
  start: Frame,
  finish: Frame,
  view: Viewport,
  options: AsciiOptions,
): string {
  return [
    name,
    "=".repeat(name.length),
    "",
    "(A terminal cell is about twice as tall as it is wide, so these are",
    "stretched vertically against what the SVG draws. Judge shapes and travel,",
    "not proportions.)",
    "",
    "start",
    "-----",
    asciiFrame(start, view, options),
    "",
    "finish",
    "------",
    asciiFrame(finish, view, options),
    "",
  ].join("\n");
}
