# Form demonstrations — animated SVG spike

A spike, not a feature. Nothing here is imported by the app; it exists to answer
one question before any of it gets built: **does an animated line figure read as
coaching, or just as a novelty?**

## Look at it

Open `index.html` in a browser. No dev server, no build, no network — the SVGs
are inlined so `currentColor` reaches them and the theme toggle does something.
There is a pause button next to it.

## Regenerate

```sh
node docs/demos/form-svg/generate.mjs
```

Rewrites the three SVGs and `index.html`, and prints an ASCII rasterisation of
every lift's top and bottom pose to the terminal.

That ASCII is not decoration. There is no browser in the devcontainer, so it is
the only way to check the geometry at all — and it earns its keep: it is what
caught the lifter's head being drawn *through* the bench pad, which the pose
angles alone looked fine about.

## How a figure is built

Each lift is defined as **joint angles**, and forward kinematics turns those
into points. Frames are sampled by interpolating the *angles*, never the
positions. That is the whole trick: a chord is shorter than the arc it spans, so
interpolating positions would shorten a limb mid-rep and the lifter's femur would
visibly telescope on the way down to a squat.

The hip thrust needs two-link IK on top, because it is the one closed chain here
— the foot stays planted on the floor while the shoulders stay pinned to the
bench, and the knee is whatever those two constraints leave it.

Worth checking against the real lifts, and all three hold: the squat's bar path
falls almost dead vertical (x drifts 1.4 units across a 30-unit descent), the hip
travels behind the ankle while the knee travels in front, the RDL's arms hang
plumb throughout, and the hip thrust locks out with shoulder, hip and knee on one
line over a vertical shin.

## Why SMIL and not CSS

CSS can only animate a path's `d` property, which Firefox does not support.
SMIL animation of `points` works everywhere current.

The cost: **SMIL ignores `prefers-reduced-motion`.** A consumer has to call
`svg.pauseAnimations()` itself — `index.html` does exactly that, and it is the
line a real Svelte component would carry.

## Size

Roughly 4.7–5.5 KB raw, **1.4–1.8 KB gzipped**, per lift. `src/ui/nginx.conf`
already lists `image/svg+xml` in `gzip_types`, so that compression costs nothing
to arrange. A full dozen lifts would be about 20 KB on the wire.

A segment that never moves is emitted as a plain `<polyline>` with no `<animate>`
— the planted foot is identical in all 40 frames, and writing it out 40 times was
most of the file.

## If this is worth building

It needs no migration and no API change. `src/ui/src/lib/exerciseIcon.ts` already
maps an exercise name to a presentational asset entirely in the UI; a
`exerciseDemo.ts` doing name → imported SVG is the same move.

Two things to get right at that point:

- **Import the assets from source**, do not drop them in `src/ui/public/`. Only
  Vite's fingerprinted `/assets/` output gets the `immutable, 1y` header in
  `nginx.conf`; `public/` falls through to `location /` with no `Cache-Control`
  at all, and a revalidation is a request that fails in a basement.
- **Key on the exact name**, unlike the emoji map's substring match. A Goblet
  Squat must not inherit the barbell squat's figure.
