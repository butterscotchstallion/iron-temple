<script lang="ts">
  import { barFraction, formatPercent } from "./racked";
  import { formatVolume } from "./volume";
  import VolumeBar from "./VolumeBar.svelte";

  // Where the period's tonnage actually went, heaviest lift first.
  //
  // Bars rather than a donut: these are magnitudes to be compared, and a bar
  // chart compares lengths against a shared baseline, which people read far more
  // accurately than angles. It also has room for the lift's name, so nothing
  // depends on matching a slice to a legend.
  //
  // Colours come from the page, not from this component, so a lift keeps the
  // same colour here as in the trend chart above it. Lifts that the trend chart
  // could not draw — a single session gives no trend — get a neutral bar rather
  // than a colour that would imply a line exists somewhere.
  //
  // Assistance is tagged rather than sorted away or given a bar of its own.
  // These bars answer "where did the month's weight go", and a lifter who spent
  // a fifth of it on accessories wants to see that fifth in the same ranking as
  // the rest — moving it to a separate list would answer a different question.

  type Row = {
    exerciseId: number;
    exerciseName: string;
    volumeLb: number;
    sets: number;
    share: number;
    color: string | null;
    isAssistance: boolean;
  };

  let { rows }: { rows: Row[] } = $props();

  const max = $derived(Math.max(0, ...rows.map((r) => r.volumeLb)));
</script>

<ul class="flex flex-col gap-2">
  {#each rows as row (row.exerciseId)}
    <VolumeBar
      fraction={barFraction(row.volumeLb, max)}
      fillClass={row.color ? "" : "bg-muted-foreground/50"}
      fillColor={row.color}
    >
      {#snippet name()}
        {row.exerciseName}
        {#if row.isAssistance}
          <span class="ml-1 align-[1px] text-[10px] font-normal uppercase tracking-wider text-muted-foreground">
            assistance
          </span>
        {/if}
      {/snippet}
      {#snippet value()}
        {formatVolume(row.volumeLb)} lb · {formatPercent(row.share)}
      {/snippet}
    </VolumeBar>
  {/each}
</ul>
