<script lang="ts">
  import { formatDelta } from "./racked";
  import { dateTicks, isoDayNumber, scale, shortDate } from "./chartScale";
  import ChartGridlines from "./ChartGridlines.svelte";
  import type { IndexedSeries } from "./racked";

  // Improvement across every lift, as percentage change from each lift's first
  // session in the period.
  //
  // One shared axis, deliberately. These lifts do not share a scale — a deadlift
  // works near 400 lb and a press near 100 — so pounds would flatten the press
  // against the bottom of the plot. A second y-axis is the usual way out and is
  // worse: two scales on one chart invite comparing slopes that cannot be
  // compared. See indexedSeries in racked.ts.

  let { series }: { series: IndexedSeries[] } = $props();

  // viewBox units; the SVG scales to its container via w-full.
  const W = 320;
  const H = 190;
  const padL = 34;
  const padR = 10;
  const padT = 14;
  const padB = 28;

  const points = $derived(series.flatMap((s) => s.points));
  const days = $derived(points.map((p) => isoDayNumber(p.performedOn)));
  const xMin = $derived(days.length ? Math.min(...days) : 0);
  const xMax = $derived(days.length ? Math.max(...days) : 1);

  const pcts = $derived(points.map((p) => p.pct));
  // Zero is always in view: it is the baseline every lift is measured from, and
  // a chart that crops it would show a gain and a loss looking alike.
  const yLoRaw = $derived(Math.min(0, ...(pcts.length ? pcts : [0])));
  const yHiRaw = $derived(Math.max(0, ...(pcts.length ? pcts : [0])));
  const pad = $derived((yHiRaw - yLoRaw || 0.04) * 0.15);
  const yLo = $derived(yLoRaw - pad);
  const yHi = $derived(yHiRaw + pad);

  function xAt(iso: string): number {
    return scale(isoDayNumber(iso), xMin, xMax, padL, W - padR);
  }
  function yAt(pct: number): number {
    return scale(pct, yLo, yHi, H - padB, padT);
  }

  function path(s: IndexedSeries): string {
    return s.points
      .map((p, i) => `${i === 0 ? "M" : "L"} ${xAt(p.performedOn)} ${yAt(p.pct)}`)
      .join(" ");
  }

  const yTicks = $derived([yHi, (yLo + yHi) / 2, yLo]);

  const xTicks = $derived(dateTicks(points.map((p) => p.performedOn)));

  // Hover reads out the nearest dated column rather than the nearest point of
  // one line, because the question is "how did the lifts compare that day".
  let hover = $state<string | null>(null);
  const dates = $derived([...new Set(points.map((p) => p.performedOn))].sort());

  function onMove(e: PointerEvent) {
    if (!dates.length) return;
    const rect = (e.currentTarget as SVGRectElement).getBoundingClientRect();
    const x = ((e.clientX - rect.left) / rect.width) * W;
    let best = dates[0];
    let bestDist = Infinity;
    for (const d of dates) {
      const dist = Math.abs(xAt(d) - x);
      if (dist < bestDist) {
        best = d;
        bestDist = dist;
      }
    }
    hover = best;
  }

  const hovered = $derived(
    hover === null
      ? []
      : series
          .map((s) => ({ s, p: s.points.find((p) => p.performedOn === hover) }))
          .filter((h) => h.p !== undefined),
  );

  const label = $derived(
    `Improvement for ${series.length} lift${series.length === 1 ? "" : "s"}, ` +
      `as percentage change from each lift's first session. ` +
      series.map((s) => `${s.exerciseName} ${formatDelta(s.points[s.points.length - 1].pct)}`).join(", "),
  );
</script>

<svg
  viewBox="0 0 {W} {H}"
  class="w-full touch-none select-none"
  role="img"
  aria-label={label}
>
  <ChartGridlines
    rows={yTicks.map((t) => ({ y: yAt(t), label: formatDelta(t) }))}
    x1={padL}
    x2={W - padR}
    labelX={padL - 5}
  />

  <!-- The zero baseline is the reference every line is measured against, so it
       reads stronger than the other gridlines. -->
  <line
    x1={padL}
    x2={W - padR}
    y1={yAt(0)}
    y2={yAt(0)}
    class="stroke-muted-foreground/50"
    stroke-width="1"
  />

  {#each xTicks as iso (iso)}
    <text
      x={xAt(iso)}
      y={H - padB + 12}
      text-anchor="middle"
      class="fill-muted-foreground text-[8px] tabular-nums"
    >
      {shortDate(iso)}
    </text>
  {/each}

  {#if hover !== null}
    <line
      x1={xAt(hover)}
      x2={xAt(hover)}
      y1={padT}
      y2={H - padB}
      class="stroke-muted-foreground/50"
      stroke-width="1"
      stroke-dasharray="2 2"
    />
  {/if}

  {#each series as s (s.exerciseId)}
    <path
      d={path(s)}
      fill="none"
      stroke={s.color}
      stroke-width="2"
      stroke-linejoin="round"
      stroke-linecap="round"
    />
    <!-- Keyed by position, not by date: a lifter can train the same lift twice
         in a day, which gives two points sharing a performedOn. Position is the
         identity here anyway — these are plotted marks, not rows of a list. -->
    {#each s.points as p, i (i)}
      <!-- A 1px surface-coloured ring keeps two lines crossing at a point from
           merging into one blob. -->
      <circle
        cx={xAt(p.performedOn)}
        cy={yAt(p.pct)}
        r={hover === p.performedOn ? 4 : 2.5}
        fill={s.color}
        class="stroke-card"
        stroke-width="1"
      />
    {/each}
  {/each}

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

<!-- Legend. Always present, and carrying each lift's name and final change, so
     identity and outcome never depend on telling two colours apart. -->
<ul class="mt-2 flex flex-wrap gap-x-4 gap-y-1">
  {#each series as s (s.exerciseId)}
    <li class="flex items-center gap-1.5 text-xs">
      <span
        class="size-2.5 shrink-0 rounded-[2px]"
        style="background: {s.color}"
        aria-hidden="true"
      ></span>
      <span class="text-muted-foreground">{s.exerciseName}</span>
      <span class="font-semibold tabular-nums text-foreground">
        {formatDelta(
          hovered.find((h) => h.s.exerciseId === s.exerciseId)?.p?.pct ??
            s.points[s.points.length - 1].pct,
        )}
      </span>
    </li>
  {/each}
</ul>

{#if hover !== null}
  <p class="mt-1 text-xs text-muted-foreground tabular-nums">{hover}</p>
{/if}
