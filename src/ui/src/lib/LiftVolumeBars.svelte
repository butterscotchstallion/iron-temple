<script lang="ts">
  import { barFraction, formatPercent } from "./racked";
  import { formatVolume } from "./volume";

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
    <li>
      <div class="flex items-baseline justify-between gap-2 text-xs">
        <span class="truncate font-semibold text-foreground">
          {row.exerciseName}
          {#if row.isAssistance}
            <span class="ml-1 align-[1px] text-[10px] font-normal uppercase tracking-wider text-muted-foreground">
              assistance
            </span>
          {/if}
        </span>
        <span class="shrink-0 tabular-nums text-muted-foreground">
          {formatVolume(row.volumeLb)} lb · {formatPercent(row.share)}
        </span>
      </div>
      <div class="mt-1 h-2 w-full overflow-hidden rounded-full bg-muted/40">
        <div
          class="h-full rounded-full {row.color ? '' : 'bg-muted-foreground/50'}"
          style="width: {barFraction(row.volumeLb, max) * 100}%{row.color
            ? `; background: ${row.color}`
            : ''}"
        ></div>
      </div>
    </li>
  {/each}
</ul>
