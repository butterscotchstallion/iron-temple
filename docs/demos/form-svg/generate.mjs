#!/usr/bin/env node
// Generates animated SVG form demonstrations for a handful of lifts.
//
// A figure is defined as JOINT ANGLES, not joint positions. Forward kinematics
// turns the angles into points, and frames are sampled by interpolating the
// ANGLES — which is the whole trick. Interpolating positions instead would
// shorten a limb halfway through the rep (the chord of an arc is shorter than
// the arc), so a squat would look like the lifter's femur telescopes. Angles
// keep every segment rigid by construction.
//
// Output is SMIL (`<animate>` on `points`), not CSS. CSS can only animate a
// path's `d` property, which Firefox does not support; SMIL animation of
// `points` works in every current browser. The cost is that SMIL ignores
// prefers-reduced-motion, so the consumer has to call svg.pauseAnimations() —
// see index.html and the note at the bottom of this file.

import { mkdirSync, writeFileSync } from "node:fs";
import { dirname } from "node:path";
import { fileURLToPath } from "node:url";

const OUT = dirname(fileURLToPath(import.meta.url));
const D = Math.PI / 180;

// Segment lengths, in a unit where a standing figure is ~95 tall. Roughly
// human proportion: shank ≈ thigh, arm span (shoulder→hand) a shade under the
// torso so the hands reach the hip crease and no further.
const SEG = {
  shank: 24,
  thigh: 24,
  torso: 28,
  neck: 10,
  headR: 6.5,
  upperArm: 14,
  foreArm: 13,
};

// The planted foot. Every standing lift shares it, so the figure never drifts.
const ANKLE = { x: 0, y: 5 };
const HEEL = { x: -5, y: 0 };
const TOE = { x: 13, y: 0 };

const pt = (x, y) => ({ x, y });
const polar = (p, deg, len) =>
  pt(p.x + len * Math.cos(deg * D), p.y + len * Math.sin(deg * D));

// Two-link IK: place the middle joint of a chain whose ends are pinned. Used
// where the chain is CLOSED and angles can't drive it directly — the hip
// thrust, where the foot stays planted on the floor while the hips travel.
// `bend` picks which of the two mirror solutions to take.
//
// Out of reach is clamped to full extension rather than failing: a straight
// arm pointing at the bar reads correctly, an exception does not.
function ik(a, b, l1, l2, bend) {
  let dx = b.x - a.x;
  let dy = b.y - a.y;
  let d = Math.hypot(dx, dy);
  const max = l1 + l2 - 1e-6;
  if (d > max) {
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
    joint: pt(a.x + ux * t - uy * h * bend, a.y + uy * t + ux * h * bend),
    end: pt(a.x + dx, a.y + dy),
  };
}

// ---------------------------------------------------------------------------
// Builders: pose (angles) -> joints. Angles are absolute, measured the usual
// mathematical way — 0° is forward (+x), 90° is straight up — in a y-up world
// that gets flipped once, at emit time.
// ---------------------------------------------------------------------------

// Back squat. The arms don't move relative to the torso, so their angles are
// given as offsets FROM the torso and carried along by it.
function squat(p) {
  const knee = polar(ANKLE, p.shank, SEG.shank);
  const hip = polar(knee, p.thigh, SEG.thigh);
  const shoulder = polar(hip, p.torso, SEG.torso);
  const head = polar(shoulder, p.head, SEG.neck);
  const elbow = polar(shoulder, p.torso + p.upperArm, SEG.upperArm);
  const hand = polar(elbow, p.torso + p.foreArm, SEG.foreArm);
  // The bar rides on the upper back: a fixed offset perpendicular to the
  // torso, so it tips with the lifter instead of floating.
  const t = p.torso * D;
  const bar = pt(
    shoulder.x - 3.5 * Math.sin(t),
    shoulder.y + 3.5 * Math.cos(t),
  );
  return {
    chains: [
      [HEEL, ANKLE, TOE],
      [ANKLE, knee, hip],
      [hip, shoulder],
      [shoulder, elbow, hand],
    ],
    head,
    bar,
  };
}

// Romanian deadlift. The arms hang from the shoulder under gravity, so unlike
// the squat their angles are ABSOLUTE and ignore the torso entirely — that is
// what makes the bar track down the thigh as the lifter hinges.
function rdl(p) {
  const knee = polar(ANKLE, p.shank, SEG.shank);
  const hip = polar(knee, p.thigh, SEG.thigh);
  const shoulder = polar(hip, p.torso, SEG.torso);
  const head = polar(shoulder, p.head, SEG.neck);
  const elbow = polar(shoulder, p.upperArm, SEG.upperArm);
  const hand = polar(elbow, p.foreArm, SEG.foreArm);
  return {
    chains: [
      [HEEL, ANKLE, TOE],
      [ANKLE, knee, hip],
      [hip, shoulder],
      [shoulder, elbow, hand],
    ],
    head,
    bar: hand,
  };
}

// Barbell hip thrust. Rooted at the SHOULDER rather than the foot, because the
// shoulder is what's pinned — it stays on the bench while everything else
// moves. One angle drives the torso; the knee then falls out of IK against a
// foot that must not slide.
function hipThrust(p) {
  const shoulder = pt(0, 22);
  const hip = polar(shoulder, p.torso, SEG.torso);
  const ankle = pt(48, 5);
  const knee = ik(hip, ankle, SEG.thigh, SEG.shank, 1).joint;
  const head = polar(shoulder, p.head, SEG.neck);
  const bar = pt(hip.x, hip.y + 5);
  const arm = ik(shoulder, bar, SEG.upperArm, SEG.foreArm, -1);
  return {
    chains: [
      [shoulder, hip, knee],
      [knee, ankle],
      [ankle, pt(ankle.x + 11, 0)],
      [shoulder, arm.joint, arm.end],
    ],
    head,
    bar,
  };
}

// ---------------------------------------------------------------------------
// The lifts
// ---------------------------------------------------------------------------

const LIFTS = [
  {
    slug: "squat",
    name: "Squat",
    build: squat,
    // Shin travels forward, thigh past horizontal (hip below knee = below
    // parallel), torso leans to keep the bar over midfoot. Worth checking in
    // the ASCII: the bar should fall almost straight down.
    top: { shank: 90, thigh: 90, torso: 90, head: 90, upperArm: 160, foreArm: 10 },
    bottom: { shank: 68, thigh: 186, torso: 62, head: 76, upperArm: 160, foreArm: 10 },
    ground: true,
  },
  {
    slug: "romanian-deadlift",
    name: "Romanian Deadlift",
    build: rdl,
    // Soft knee, hips travel back, torso hinges to ~25° above horizontal. The
    // arms stay vertical throughout — they are a plumb line, not a lever.
    top: { shank: 90, thigh: 90, torso: 90, head: 90, upperArm: 270, foreArm: 270 },
    bottom: { shank: 96, thigh: 142, torso: 25, head: 40, upperArm: 270, foreArm: 270 },
    ground: true,
  },
  {
    slug: "hip-thrust",
    name: "Barbell Hip Thrust",
    build: hipThrust,
    // Lockout is shoulder, hip and knee on one line; the bottom drops the hips
    // toward the floor with the feet planted.
    // head at 165° rests the skull ON the pad (its underside lands at y≈18.1,
    // the bench top) with the chin tucked toward the hips. Pointing it down
    // the way a standing lift does buries it in the bench.
    top: { torso: 8, head: 165 },
    bottom: { torso: -34, head: 165 },
    ground: true,
    bench: true,
  },
];

// ---------------------------------------------------------------------------
// Frame sampling
// ---------------------------------------------------------------------------

const FRAMES = 40;
const smoothstep = (t) => t * t * (3 - 2 * t);

// One rep: lower, a beat at the bottom, drive up, a beat at the top. The
// pauses are what stop the loop reading as a pendulum.
function phase(t) {
  if (t < 0.42) return smoothstep(t / 0.42);
  if (t < 0.5) return 1;
  if (t < 0.92) return 1 - smoothstep((t - 0.5) / 0.42);
  return 0;
}

function sample(lift) {
  const keys = Object.keys(lift.top);
  const frames = [];
  for (let i = 0; i < FRAMES; i++) {
    const u = phase(i / FRAMES);
    const pose = {};
    for (const k of keys) pose[k] = lift.top[k] + (lift.bottom[k] - lift.top[k]) * u;
    frames.push(lift.build(pose));
  }
  return frames;
}

// The floor and the bench, in world coordinates, so the ASCII check and the
// SVG draw the same thing — scenery the preview can't see is scenery nobody
// checked.
function sceneryOf(lift, bounds) {
  const chains = [];
  if (lift.ground) chains.push([pt(bounds.minX, 0), pt(bounds.maxX, 0)]);
  if (lift.bench) {
    // The pad sits just under the shoulder (world y=22) so the lifter lies ON
    // it rather than floating above it, with one leg down to the floor.
    chains.push([pt(-19, 18), pt(9, 18)]);
    chains.push([pt(-7, 18), pt(-7, 0)]);
  }
  return chains;
}

// ---------------------------------------------------------------------------
// ASCII preview — the only way to check the geometry without a browser.
// ---------------------------------------------------------------------------

function ascii(frame, bounds, lift, w = 54, h = 28) {
  const grid = Array.from({ length: h }, () => Array(w).fill(" "));
  const sx = (w - 1) / (bounds.maxX - bounds.minX);
  const sy = (h - 1) / (bounds.maxY - bounds.minY);
  const s = Math.min(sx, sy);
  const ox = (w - (bounds.maxX - bounds.minX) * s) / 2;
  const map = (p) => [
    Math.round((p.x - bounds.minX) * s + ox),
    Math.round((bounds.maxY - p.y) * s),
  ];
  const plot = (x, y, ch) => {
    if (x >= 0 && x < w && y >= 0 && y < h) grid[y][x] = ch;
  };
  const line = (a, b, ch) => {
    const [x0, y0] = map(a);
    const [x1, y1] = map(b);
    const steps = Math.max(Math.abs(x1 - x0), Math.abs(y1 - y0), 1);
    for (let i = 0; i <= steps; i++) {
      plot(Math.round(x0 + ((x1 - x0) * i) / steps), Math.round(y0 + ((y1 - y0) * i) / steps), ch);
    }
  };
  for (const chain of sceneryOf(lift, bounds)) {
    for (let i = 0; i < chain.length - 1; i++) line(chain[i], chain[i + 1], ".");
  }
  for (const chain of frame.chains) {
    for (let i = 0; i < chain.length - 1; i++) line(chain[i], chain[i + 1], "*");
  }
  const [hx, hy] = map(frame.head);
  const hr = Math.round(SEG.headR * s);
  for (let a = 0; a < 360; a += 12) {
    plot(Math.round(hx + hr * Math.cos(a * D)), Math.round(hy + hr * Math.sin(a * D) * 0.55), "o");
  }
  if (frame.bar) {
    const [bx, by] = map(frame.bar);
    plot(bx, by, "#");
    plot(bx - 1, by, "#");
    plot(bx + 1, by, "#");
  }
  return grid.map((r) => r.join("").replace(/\s+$/, "")).join("\n");
}

// ---------------------------------------------------------------------------
// SVG emit
// ---------------------------------------------------------------------------

function boundsOf(frames) {
  let minX = Infinity, maxX = -Infinity, minY = Infinity, maxY = -Infinity;
  for (const f of frames) {
    const ps = [...f.chains.flat(), f.head, f.bar].filter(Boolean);
    for (const p of ps) {
      minX = Math.min(minX, p.x);
      maxX = Math.max(maxX, p.x);
      minY = Math.min(minY, p.y);
      maxY = Math.max(maxY, p.y);
    }
  }
  const pad = SEG.headR + 6;
  return { minX: minX - pad, maxX: maxX + pad, minY: Math.min(0, minY) - 4, maxY: maxY + pad };
}

const r1 = (n) => Math.round(n * 10) / 10;
// The single y-flip: world is y-up, SVG is y-down.
const svgPts = (chain) => chain.map((p) => `${r1(p.x)},${r1(-p.y)}`).join(" ");

function toSvg(lift, frames, bounds) {
  const vb = [
    r1(bounds.minX),
    r1(-bounds.maxY),
    r1(bounds.maxX - bounds.minX),
    r1(bounds.maxY - bounds.minY),
  ].join(" ");

  // A segment that never moves gets no <animate> at all. The planted foot is
  // identical in all 40 frames, and writing it out 40 times was most of the
  // file — this is the difference between a ~6 KB asset and a ~2 KB one.
  const anim = (attr, values) => {
    if (values.every((v) => v === values[0])) return "";
    return (
      `\n      <animate attributeName="${attr}" dur="4s" repeatCount="indefinite" ` +
      `calcMode="linear" values="${values.join(";")}"/>`
    );
  };
  const el = (open, body, close) =>
    body ? `    ${open}>${body}\n    ${close}` : `    ${open}/>`;

  const chains = frames[0].chains
    .map((_, ci) =>
      el(
        `<polyline points="${svgPts(frames[0].chains[ci])}"`,
        anim("points", frames.map((f) => svgPts(f.chains[ci]))),
        `</polyline>`,
      ),
    )
    .join("\n");

  const circle = (p, r, get) =>
    el(
      `<circle cx="${r1(get(frames[0]).x)}" cy="${r1(-get(frames[0]).y)}" r="${r}"`,
      anim("cx", frames.map((f) => r1(get(f).x))) +
        anim("cy", frames.map((f) => r1(-get(f).y))),
      `</circle>`,
    );

  const head = circle(null, SEG.headR, (f) => f.head);

  const bar = frames[0].bar
    ? `  <g class="bar" fill="none" stroke="var(--demo-bar, #b026ff)" stroke-width="3.5">\n` +
      circle(null, 5, (f) => f.bar) +
      `\n  </g>\n`
    : "";

  const scenery = sceneryOf(lift, bounds)
    .map((chain) => `    <polyline points="${svgPts(chain)}"/>\n`)
    .join("");

  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="${vb}" role="img" aria-labelledby="${lift.slug}-title">
  <title id="${lift.slug}-title">${lift.name} — animated demonstration of the movement</title>
${scenery ? `  <g class="scenery" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" opacity="0.28">\n${scenery}  </g>\n` : ""}  <g class="figure" fill="none" stroke="currentColor" stroke-width="3.5" stroke-linecap="round" stroke-linejoin="round">
${chains}
${head}
  </g>
${bar}</svg>
`;
}

// ---------------------------------------------------------------------------

mkdirSync(OUT, { recursive: true });
const built = [];

for (const lift of LIFTS) {
  const frames = sample(lift);
  const bounds = boundsOf(frames);
  const svg = toSvg(lift, frames, bounds);
  writeFileSync(`${OUT}/${lift.slug}.svg`, svg);
  built.push({ lift, svg });

  const top = lift.build(lift.top);
  const bottom = lift.build(lift.bottom);
  console.log(`\n${"=".repeat(58)}\n${lift.name}  (${(svg.length / 1024).toFixed(1)} KB)\n${"=".repeat(58)}`);
  console.log("-- top " + "-".repeat(50));
  console.log(ascii(top, bounds, lift));
  console.log("-- bottom " + "-".repeat(47));
  console.log(ascii(bottom, bounds, lift));
}

// A standalone page so the motion can be judged without a dev server. The
// SVGs are inlined rather than <img>-referenced so currentColor reaches them
// and the theme toggle actually does something.
const page = `<!doctype html>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Form demonstrations — animated SVG spike</title>
<style>
  :root { --bg: #0b0b0f; --fg: #e7e7ea; --muted: #8b8b96; --card: #15151c; --demo-bar: #b026ff; }
  body.light { --bg: #f7f7f9; --fg: #1a1a1f; --muted: #6b6b76; --card: #fff; }
  * { box-sizing: border-box; }
  body { margin: 0; padding: 24px 16px 64px; background: var(--bg); color: var(--fg);
         font: 15px/1.5 ui-sans-serif, system-ui, sans-serif; }
  .wrap { max-width: 420px; margin: 0 auto; }
  h1 { font-size: 19px; margin: 0 0 4px; }
  p.sub { color: var(--muted); margin: 0 0 20px; font-size: 13px; }
  .card { background: var(--card); border: 1px solid color-mix(in srgb, var(--fg) 12%, transparent);
          border-radius: 14px; padding: 14px; margin-bottom: 14px; }
  .card h2 { font-size: 12px; letter-spacing: .18em; text-transform: uppercase;
             color: var(--muted); margin: 0 0 10px; font-weight: 600; }
  .card svg { display: block; width: 100%; height: 190px; color: var(--fg); }
  .row { display: flex; gap: 8px; margin-bottom: 20px; flex-wrap: wrap; }
  button { font: inherit; font-size: 13px; padding: 7px 13px; border-radius: 999px; cursor: pointer;
           background: transparent; color: var(--fg);
           border: 1px solid color-mix(in srgb, var(--fg) 25%, transparent); }
  .note { color: var(--muted); font-size: 12px; border-top: 1px solid color-mix(in srgb, var(--fg) 12%, transparent);
          padding-top: 14px; }
  .sizes { color: var(--muted); font-size: 12px; margin-top: 2px; }
</style>
<div class="wrap">
  <h1>Form demonstrations</h1>
  <p class="sub">Animated SVG spike — sized as it would appear on the exercise page.</p>
  <div class="row">
    <button id="theme">Toggle light / dark</button>
    <button id="play">Pause</button>
  </div>
${built.map(({ lift, svg }) => `  <div class="card">\n    <h2>${lift.name}</h2>\n${svg.replace(/^/gm, "    ").trimEnd()}\n  </div>`).join("\n")}
  <p class="note">
    Each figure is one file, no JavaScript, no network. The stroke is
    <code>currentColor</code>, so it inherits the theme; the bar is the one
    accent colour. SMIL ignores <code>prefers-reduced-motion</code>, so the
    pause below is what a real component would call automatically —
    <code>svg.pauseAnimations()</code> — when the lifter has asked for less
    motion.
  </p>
  <p class="sizes">${built.map(({ lift, svg }) => `${lift.name}: ${(svg.length / 1024).toFixed(1)} KB`).join(" · ")}</p>
</div>
<script>
  const svgs = [...document.querySelectorAll('.card svg')];
  document.getElementById('theme').onclick = () => document.body.classList.toggle('light');
  const btn = document.getElementById('play');
  let running = true;
  btn.onclick = () => {
    running = !running;
    svgs.forEach(s => running ? s.unpauseAnimations() : s.pauseAnimations());
    btn.textContent = running ? 'Pause' : 'Play';
  };
  // What the production component would do on its own.
  if (matchMedia('(prefers-reduced-motion: reduce)').matches) btn.click();
</script>
`;

writeFileSync(`${OUT}/index.html`, page);
console.log(`\nWrote ${built.length} SVGs + index.html to docs/demos/form-svg/`);
