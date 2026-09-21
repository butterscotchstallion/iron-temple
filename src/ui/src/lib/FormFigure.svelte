<script lang="ts">
  // An animated demonstration of how a movement is performed.
  //
  // The figure is computed rather than fetched: pose data goes through forward
  // kinematics at render time, so there is no asset to load and nothing to fail
  // in a basement with no signal. See formKinematics.ts for why the frames
  // interpolate angles instead of positions.
  //
  // Renders nothing at all for a movement with no figure — a lifter's own
  // custom exercise, or one deliberately left undrawn. That is the normal path,
  // not an error, so the caller does not have to guard.

  import { exerciseDemo } from "./exerciseDemo";
  import { r1, type Frame, type Point } from "./formKinematics";
  import { prefersReducedMotion } from "./reducedMotion";

  let {
    name,
    /** Draw a single pose with no animation — for dense lists like the library. */
    static: isStatic = false,
    class: className = "",
  }: { name: string; static?: boolean; class?: string } = $props();

  const demo = $derived(exerciseDemo(name));

  // Asked once per render rather than cached at module scope, matching
  // celebrate()'s reasoning: the preference is a Control Centre toggle people
  // reach for mid-session.
  const still = $derived(isStatic || prefersReducedMotion());

  const frames = $derived(still ? [] : (demo?.frames() ?? []));

  // The single flip from a y-up world to SVG's y-down. Every angle in
  // formArchetypes.ts is written the way it would be on graph paper because of
  // this one line.
  const pts = (chain: Point[]) => chain.map((p) => `${r1(p.x)},${r1(-p.y)}`).join(" ");

  const poses = $derived<Frame[]>(demo ? (still ? [demo.still()] : frames) : []);
  const first = $derived(poses[0]);

  // A segment that never moves gets no <animate> at all. The planted foot is
  // identical in all forty frames, and a Plank is identical throughout, so this
  // is also what makes a static hold render as a still image for free.
  function values(pick: (f: Frame) => string): string | null {
    if (still) return null;
    const all = frames.map(pick);
    return all.every((v) => v === all[0]) ? null : all.join(";");
  }

  const headX = $derived(values((f) => String(r1(f.head.x))));
  const headY = $derived(values((f) => String(r1(-f.head.y))));
  const barX = $derived(values((f) => String(r1(f.bar?.x ?? 0))));
  const barY = $derived(values((f) => String(r1(-(f.bar?.y ?? 0)))));
  const chainValues = $derived(
    (first?.chains ?? []).map((_, i) => values((f) => pts(f.chains[i]))),
  );

  const label = $derived(
    still
      ? `Diagram of the ${name} position`
      : `Animated demonstration of how to perform the ${name}`,
  );
</script>

{#if demo && first}
  <!-- Keyed on the name: SMIL does not reliably restart when a live animation's
       values are replaced, so a figure swapped in place would keep the previous
       movement's timeline. -->
  {#key name}
    <svg
      viewBox="{r1(demo.skeleton.view.minX)} {r1(-demo.skeleton.view.maxY)} {r1(
        demo.skeleton.view.maxX - demo.skeleton.view.minX,
      )} {r1(demo.skeleton.view.maxY - demo.skeleton.view.minY)}"
      class="w-full select-none {className}"
      role="img"
      aria-label={label}
    >
      <g
        class="fill-none stroke-muted-foreground/30"
        stroke-width="2"
        stroke-linecap="round"
      >
        {#each demo.skeleton.scenery as chain, i (i)}
          <polyline points={pts(chain)} />
        {/each}
      </g>

      <g
        class="fill-none stroke-foreground"
        stroke-width="3.5"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        {#each first.chains as chain, i (i)}
          <polyline points={pts(chain)}>
            {#if chainValues[i]}
              <animate
                attributeName="points"
                dur="4s"
                repeatCount="indefinite"
                calcMode="linear"
                values={chainValues[i]}
              />
            {/if}
          </polyline>
        {/each}

        <circle cx={r1(first.head.x)} cy={r1(-first.head.y)} r={demo.skeleton.headRadius}>
          {#if headX}
            <animate
              attributeName="cx"
              dur="4s"
              repeatCount="indefinite"
              calcMode="linear"
              values={headX}
            />
          {/if}
          {#if headY}
            <animate
              attributeName="cy"
              dur="4s"
              repeatCount="indefinite"
              calcMode="linear"
              values={headY}
            />
          {/if}
        </circle>
      </g>

      {#if first.bar}
        <g class="fill-none stroke-primary" stroke-width="3.5">
          <circle cx={r1(first.bar.x)} cy={r1(-first.bar.y)} r="5">
            {#if barX}
              <animate
                attributeName="cx"
                dur="4s"
                repeatCount="indefinite"
                calcMode="linear"
                values={barX}
              />
            {/if}
            {#if barY}
              <animate
                attributeName="cy"
                dur="4s"
                repeatCount="indefinite"
                calcMode="linear"
                values={barY}
              />
            {/if}
          </circle>
        </g>
      {/if}
    </svg>
  {/key}
{/if}
