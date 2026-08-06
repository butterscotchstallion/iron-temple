<script lang="ts">
  type Point = {
    performedOn: string;
    weightLb: number;
    reps: number;
    completed: boolean;
  };

  let { points }: { points: Point[] } = $props();

  // viewBox units; the SVG scales to its container via w-full.
  const W = 320;
  const H = 190;
  const padL = 40;
  const padR = 14;
  const padT = 18;
  const padB = 34;

  const n = $derived(points.length);
  const weights = $derived(points.map((p) => p.weightLb));
  const yMin = $derived(weights.length ? Math.min(...weights) : 0);
  const yMax = $derived(weights.length ? Math.max(...weights) : 1);
  // Headroom; guard a flat series (all sessions at one weight).
  const yLo = $derived(yMin === yMax ? yMin - 5 : yMin - (yMax - yMin) * 0.15);
  const yHi = $derived(yMin === yMax ? yMax + 5 : yMax + (yMax - yMin) * 0.15);

  function xAt(i: number): number {
    if (n <= 1) return (padL + (W - padR)) / 2;
    return padL + (i / (n - 1)) * (W - padL - padR);
  }
  function yAt(w: number): number {
    const t = (w - yLo) / (yHi - yLo || 1);
    return H - padB - t * (H - padT - padB);
  }

  const linePath = $derived(
    points
      .map((p, i) => `${i === 0 ? "M" : "L"} ${xAt(i)} ${yAt(p.weightLb)}`)
      .join(" "),
  );
  const yTicks = $derived([yHi, (yLo + yHi) / 2, yLo].map((v) => Math.round(v)));

  // Up to four evenly-spaced session indices for x-axis date labels.
  const xTicks = $derived.by(() => {
    if (n === 0) return [];
    if (n === 1) return [0];
    const count = Math.min(4, n);
    const set = new Set<number>();
    for (let k = 0; k < count; k++) {
      set.add(Math.round((k / (count - 1)) * (n - 1)));
    }
    return [...set];
  });

  function shortDate(iso: string): string {
    const parts = iso.split("-");
    return `${Number(parts[1])}/${Number(parts[2])}`;
  }

  let hover = $state<number | null>(null);
  function onMove(e: PointerEvent) {
    const rect = (e.currentTarget as SVGRectElement).getBoundingClientRect();
    const x = ((e.clientX - rect.left) / rect.width) * W;
    if (n === 0) return;
    const i = Math.round(((x - padL) / (W - padL - padR)) * (n - 1));
    hover = Math.max(0, Math.min(n - 1, i));
  }
</script>

<svg
  viewBox="0 0 {W} {H}"
  class="w-full touch-none select-none"
  role="img"
  aria-label={`Weight over ${n} session${n === 1 ? "" : "s"}`}
>
  <!-- y gridlines + weight labels -->
  {#each yTicks as t (t)}
    <line
      x1={padL}
      x2={W - padR}
      y1={yAt(t)}
      y2={yAt(t)}
      class="stroke-border"
      stroke-width="1"
    />
    <text
      x={padL - 6}
      y={yAt(t) + 3}
      text-anchor="end"
      class="fill-muted-foreground text-[9px] tabular-nums"
    >
      {t}
    </text>
  {/each}
  <text
    x={padL - 6}
    y={padT - 6}
    text-anchor="end"
    class="fill-muted-foreground text-[8px] uppercase tracking-wider"
  >
    lb
  </text>

  <!-- x axis baseline -->
  <line
    x1={padL}
    x2={W - padR}
    y1={H - padB}
    y2={H - padB}
    class="stroke-border"
    stroke-width="1"
  />

  {#if n > 1}
    <path
      d={linePath}
      fill="none"
      class="stroke-primary"
      stroke-width="2"
      stroke-linejoin="round"
      stroke-linecap="round"
    />
  {/if}

  {#each points as p, i (i)}
    <circle cx={xAt(i)} cy={yAt(p.weightLb)} r="3" class="fill-primary" />
  {/each}

  <!-- x-axis date ticks + labels -->
  {#each xTicks as i (i)}
    <line
      x1={xAt(i)}
      x2={xAt(i)}
      y1={H - padB}
      y2={H - padB + 3}
      class="stroke-border"
      stroke-width="1"
    />
    <text
      x={xAt(i)}
      y={H - padB + 13}
      text-anchor="middle"
      class="fill-muted-foreground text-[8px] tabular-nums"
    >
      {shortDate(points[i].performedOn)}
    </text>
  {/each}

  {#if hover !== null}
    {@const p = points[hover]}
    <line
      x1={xAt(hover)}
      x2={xAt(hover)}
      y1={padT}
      y2={H - padB}
      class="stroke-muted-foreground/50"
      stroke-width="1"
      stroke-dasharray="2 2"
    />
    <circle
      cx={xAt(hover)}
      cy={yAt(p.weightLb)}
      r="4.5"
      class="fill-primary stroke-background"
      stroke-width="1.5"
    />
    <text
      x={xAt(hover)}
      y={padT - 4}
      text-anchor="middle"
      class="fill-foreground text-[9px] font-bold tabular-nums"
    >
      {p.weightLb} lb × {p.reps}
    </text>
  {/if}

  <!-- Pointer-only hover overlay; the data is described by the SVG aria-label
       and the PR / 1RM tiles, so this layer is decorative for AT. -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <rect
    x={padL}
    y={padT}
    width={W - padL - padR}
    height={H - padT - padB}
    fill="transparent"
    role="presentation"
    onpointermove={onMove}
    onpointerleave={() => (hover = null)}
  />
</svg>

{#if hover !== null}
  <p class="mt-1 text-center text-xs text-muted-foreground">
    {points[hover].performedOn}
  </p>
{/if}
