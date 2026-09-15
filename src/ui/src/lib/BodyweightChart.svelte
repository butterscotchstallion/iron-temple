<script lang="ts">
  import { formatWeighIn } from "./racked";
  import { dateTicks, isoDayNumber, scale, shortDate } from "./chartScale";
  import ChartGridlines from "./ChartGridlines.svelte";
  import type { RackedWeighIn } from "./api";

  // The lifter's weigh-ins across the period.
  //
  // Built in LiftTrendChart's idiom — same viewBox, same padding, same date
  // ticks — with one deliberate difference: the y-axis is fitted to the data and
  // is NOT anchored to zero. On the trend chart zero is the baseline every lift
  // is measured from, so cropping it would make a gain and a loss look alike.
  // Here zero is not a reference, it is an impossibility, and an axis running
  // down to it would compress a real three-pound month into a flat line at the
  // top of the plot — hiding exactly what the chart is for.
  //
  // Nothing is interpolated between points. A weigh-in exists on the days
  // somebody stood on a scale and nowhere else, which is why 0010 left the
  // column NULL rather than carrying values forward; the line joins the readings
  // to make the trend legible, and the dots say where the readings actually are.

  let { points }: { points: RackedWeighIn[] } = $props();

  // viewBox units; the SVG scales to its container via w-full.
  const W = 320;
  const H = 160;
  const padL = 34;
  const padR = 10;
  const padT = 14;
  const padB = 28;

  const days = $derived(points.map((p) => isoDayNumber(p.performedOn)));
  const xMin = $derived(days.length ? Math.min(...days) : 0);
  const xMax = $derived(days.length ? Math.max(...days) : 1);

  const weights = $derived(points.map((p) => p.weightLb));
  const loRaw = $derived(weights.length ? Math.min(...weights) : 0);
  const hiRaw = $derived(weights.length ? Math.max(...weights) : 0);
  // A floor on the span, so a lifter whose weight held to within half a pound
  // gets a line down the middle of the plot rather than noise magnified to fill
  // it. Two pounds is about the day-to-day swing of the same body on the same
  // scale, which is the smallest range worth drawing.
  const span = $derived(Math.max(hiRaw - loRaw, 2));
  const mid = $derived((loRaw + hiRaw) / 2);
  const yLo = $derived(mid - span * 0.65);
  const yHi = $derived(mid + span * 0.65);

  function xAt(iso: string): number {
    return scale(isoDayNumber(iso), xMin, xMax, padL, W - padR);
  }
  function yAt(lb: number): number {
    return scale(lb, yLo, yHi, H - padB, padT);
  }

  const path = $derived(
    points
      .map((p, i) => `${i === 0 ? "M" : "L"} ${xAt(p.performedOn)} ${yAt(p.weightLb)}`)
      .join(" "),
  );

  const yTicks = $derived([yHi, mid, yLo]);

  const xTicks = $derived(dateTicks(points.map((p) => p.performedOn)));

  const label = $derived(
    `Bodyweight across ${points.length} weigh-in${points.length === 1 ? "" : "s"}, ` +
      `${formatWeighIn(loRaw)} to ${formatWeighIn(hiRaw)} lb.`,
  );
</script>

<svg viewBox="0 0 {W} {H}" class="w-full select-none" role="img" aria-label={label}>
  <ChartGridlines
    rows={yTicks.map((t) => ({ y: yAt(t), label: formatWeighIn(t) }))}
    x1={padL}
    x2={W - padR}
    labelX={padL - 5}
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

  <path
    d={path}
    fill="none"
    class="stroke-primary"
    stroke-width="2"
    stroke-linejoin="round"
    stroke-linecap="round"
  />

  <!-- Keyed by position, not by date: two sessions in a day are two readings,
       which share a performedOn. Position is the identity here anyway — these
       are plotted marks, not rows of a list. -->
  {#each points as p, i (i)}
    <circle
      cx={xAt(p.performedOn)}
      cy={yAt(p.weightLb)}
      r="2.5"
      class="fill-primary stroke-card"
      stroke-width="1"
    />
  {/each}
</svg>
