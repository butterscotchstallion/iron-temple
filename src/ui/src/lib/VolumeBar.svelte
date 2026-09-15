<script lang="ts">
  import type { Snippet } from "svelte";

  // One row of a "where the weight went" bar list: a label, a figure, and a
  // track underneath showing the share.
  //
  // Shared by LiftVolumeBars and MuscleVolumeBars, which sit one above the other
  // on the Racked page and so have to read as the same kind of thing. They had a
  // copy of this markup each, which is the arrangement where one gains a pixel of
  // track height or a different empty-track colour and nobody notices they have
  // stopped matching.
  //
  // What the two genuinely disagree about is left to them: the label and the
  // figure come in as snippets, and the fill as a class or a colour. This owns
  // the geometry and nothing else.

  let {
    fraction,
    nameClass = "text-foreground",
    fillClass = "",
    fillColor = null,
    name,
    value,
  }: {
    /** 0–1 share of the largest row, from barFraction. */
    fraction: number;
    /** The label's own colour — dimmed for a muscle group with no work on it. */
    nameClass?: string;
    /** A Tailwind fill, for bars coloured by class rather than by value. */
    fillClass?: string;
    /** An explicit colour, so a lift keeps the hue the trend chart gave it. */
    fillColor?: string | null;
    name: Snippet;
    value: Snippet;
  } = $props();
</script>

<li>
  <div class="flex items-baseline justify-between gap-2 text-xs">
    <span class="truncate font-semibold {nameClass}">{@render name()}</span>
    <span class="shrink-0 tabular-nums text-muted-foreground">{@render value()}</span>
  </div>
  <!-- The track is always drawn, so a row with no work on it keeps its shape
       rather than collapsing — an empty track is what makes the gap legible. -->
  <div class="mt-1 h-2 w-full overflow-hidden rounded-full bg-muted/40">
    <div
      class="h-full rounded-full {fillClass}"
      style="width: {fraction * 100}%{fillColor ? `; background: ${fillColor}` : ''}"
    ></div>
  </div>
</li>
