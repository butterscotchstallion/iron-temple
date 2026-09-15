<script lang="ts">
  // The horizontal rules behind a plot, with their axis labels.
  //
  // Shared by all three hand-drawn charts, which had a copy each. They differ
  // only in what a tick is called — pounds, a signed percentage, a scale
  // reading — so the caller formats the label and this draws it.
  //
  // Takes rows already positioned in pixel space rather than a tick value and a
  // scale function, so the component never needs to know the chart's domain or
  // padding. It is markup, not geometry.

  let {
    rows,
    x1,
    x2,
    labelX,
  }: {
    /** Each gridline: where it sits in viewBox units, and what to call it. */
    rows: { y: number; label: string }[];
    x1: number;
    x2: number;
    /** Where the label's right edge sits — charts differ by a unit or two. */
    labelX: number;
  } = $props();
</script>

<!-- Keyed by index: two ticks can legitimately carry the same label (a flat
     series rounds every gridline to the same number) and would collide on a
     value key. -->
{#each rows as row, i (i)}
  <line
    {x1}
    {x2}
    y1={row.y}
    y2={row.y}
    class="stroke-border"
    stroke-width="1"
  />
  <text
    x={labelX}
    y={row.y + 3}
    text-anchor="end"
    class="fill-muted-foreground text-[9px] tabular-nums"
  >
    {row.label}
  </text>
{/each}
