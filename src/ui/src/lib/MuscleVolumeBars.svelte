<script lang="ts">
  import { barFraction, formatPercent } from "./racked";
  import { muscleGroupLabel } from "./library";
  import { formatVolume } from "./volume";

  // What the period trained, by muscle group.
  //
  // The companion to LiftVolumeBars, and deliberately a different shape of
  // answer. That chart ranks what the lifter DID; this one accounts for the
  // whole body, so it keeps a row for every group in the taxonomy — including
  // the ones with nothing against them. A month with no core work in it produces
  // no row at all in a ranking of lifts, which is exactly the month worth
  // saying something about.
  //
  // An untrained group therefore keeps its row and loses its bar: the empty
  // track is what makes the gap legible. It is dimmed rather than dropped, and
  // rather than coloured red — this is a measurement, not a scolding, and a
  // program that skips a group on purpose is a normal thing.
  //
  // muscleGroupLabel is the Library's own, so a group is named the same in both
  // places rather than each screen inventing its own capitalisation.

  type Row = {
    group: string;
    volumeLb: number;
    sets: number;
    lifts: number;
    share: number;
    trained: boolean;
  };

  let { rows }: { rows: Row[] } = $props();

  const max = $derived(Math.max(0, ...rows.map((r) => r.volumeLb)));
</script>

<ul class="flex flex-col gap-2" data-testid="muscle-bars">
  {#each rows as row (row.group)}
    <li>
      <div class="flex items-baseline justify-between gap-2 text-xs">
        <span
          class="truncate font-semibold {row.trained
            ? 'text-foreground'
            : 'text-muted-foreground'}"
        >
          {muscleGroupLabel(row.group)}
        </span>
        <span class="shrink-0 tabular-nums text-muted-foreground">
          {#if row.trained}
            {formatVolume(row.volumeLb)} lb · {formatPercent(row.share)}
          {:else}
            not trained
          {/if}
        </span>
      </div>
      <div class="mt-1 h-2 w-full overflow-hidden rounded-full bg-muted/40">
        <div
          class="h-full rounded-full bg-cyan"
          style="width: {barFraction(row.volumeLb, max) * 100}%"
        ></div>
      </div>
    </li>
  {/each}
</ul>
