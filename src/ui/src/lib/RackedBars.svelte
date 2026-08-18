<script lang="ts">
  import { barFraction } from "./racked";

  // A single-series magnitude chart: weekday volume, session start times.
  //
  // One hue rather than a categorical palette, because these bars are one
  // measure sliced by time, not competing entities — colouring each bar
  // differently would imply an identity the data does not have. The accent is
  // reserved for the bar the recap is actually pointing at.

  let {
    values,
    labels,
    format,
    highlight = -1,
    labelEvery = 1,
    caption,
  }: {
    values: number[];
    labels: string[];
    format: (value: number, index: number) => string;
    /** Index to accent — the winning weekday, the peak hour. -1 for none. */
    highlight?: number;
    /** Render every nth label; 24 hourly ticks do not fit on a phone. */
    labelEvery?: number;
    caption: string;
  } = $props();

  const max = $derived(Math.max(0, ...values));
  const empty = $derived(max <= 0);
</script>

<div class="w-full">
  <div class="flex h-24 items-end gap-[3px]" role="img" aria-label={caption}>
    {#each values as value, i (i)}
      <!-- A floor of 2% keeps a zero column visible as an empty slot rather
           than a gap, so the shape of the week stays readable. -->
      <!-- data-peak marks the accented bar. The accent is otherwise only a
           colour, which neither a test nor a screen reader can see. -->
      <div
        class="flex-1 rounded-t-[3px] transition-colors {i === highlight
          ? 'bg-primary'
          : 'bg-primary/35'}"
        style="height: {empty ? 2 : Math.max(barFraction(value, max) * 100, 2)}%"
        title="{labels[i]} · {format(value, i)}"
        data-peak={i === highlight ? "true" : undefined}
      ></div>
    {/each}
  </div>
  <div class="mt-1 flex gap-[3px]">
    {#each values as _, i (i)}
      <div class="flex-1 text-center text-[10px] text-muted-foreground">
        {i % labelEvery === 0 ? labels[i] : ""}
      </div>
    {/each}
  </div>
</div>
