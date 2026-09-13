<script lang="ts">
  import type { SessionRecapPace, SessionRecapProgress } from "./api";
  import { Card } from "$lib/components/ui/card";
  import Gauge from "@lucide/svelte/icons/gauge";
  import TrendingUp from "@lucide/svelte/icons/trending-up";
  import { formatDelta } from "./racked";
  import { formatLongDate } from "./date";
  import { formatOrdinal, formatPace } from "./recap";

  // The two questions a month's recap cannot answer: was that quick, and is the
  // bar heavier than last time? Both are absent on a first workout, and the
  // section disappears entirely rather than showing two empty cards.

  let {
    pace,
    progress,
    dayName,
  }: {
    pace: SessionRecapPace | null;
    progress: SessionRecapProgress | null;
    dayName: string;
  } = $props();

  // liftsCompared is the guard, not weightDeltaPct alone: the server sends a
  // previous session whenever one exists, but the percentage is null when no
  // lift appeared in both, and "vs. Sep 6" with no figure beside it is a card
  // that says nothing.
  const showProgress = $derived(progress != null && progress.weightDeltaPct != null);
</script>

{#if pace || showProgress}
  <section class="grid gap-4 sm:grid-cols-2">
    {#if pace}
      <Card class="p-4" data-testid="stat-pace">
        <h3
          class="flex items-center gap-2 text-xs uppercase tracking-[0.2em] text-muted-foreground"
        >
          <Gauge class="size-4 shrink-0" aria-hidden="true" />
          Pace
        </h3>
        <p class="mt-1 text-2xl font-black tabular-nums text-foreground">
          {formatPace(pace.deltaPct)}
        </p>
        <p class="text-xs tabular-nums text-muted-foreground">
          {formatOrdinal(pace.rank)} fastest {dayName} of {pace.of}
        </p>
        <!-- A "usual" drawn from one previous workout is not a usual. Saying so
             is cheaper than withholding the card, and the ranking above is
             still true either way. -->
        {#if pace.sampleSize < 2}
          <p class="text-[10px] text-muted-foreground">against a single earlier session</p>
        {/if}
      </Card>
    {/if}

    {#if showProgress && progress}
      <Card class="p-4" data-testid="stat-progress">
        <h3
          class="flex items-center gap-2 text-xs uppercase tracking-[0.2em] text-muted-foreground"
        >
          <TrendingUp class="size-4 shrink-0" aria-hidden="true" />
          On the bar
        </h3>
        <p class="mt-1 text-2xl font-black tabular-nums text-primary">
          {formatDelta(progress.weightDeltaPct ?? 0)}
        </p>
        {#if progress.previousPerformedOn}
          <p class="text-xs text-muted-foreground">
            vs. {formatLongDate(progress.previousPerformedOn)}
          </p>
        {/if}
        <!-- Named because the figure is a pairwise comparison: a percentage
             across one lift of five is a different claim from one across all
             five, and the reader cannot tell which without being told. -->
        <p class="text-[10px] text-muted-foreground">
          across {progress.liftsCompared}
          {progress.liftsCompared === 1 ? "lift" : "lifts"}{progress.liftsNew > 0
            ? ` · ${progress.liftsNew} new`
            : ""}
        </p>
      </Card>
    {/if}
  </section>
{/if}
