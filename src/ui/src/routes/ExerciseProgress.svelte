<script lang="ts">
  import { onMount } from "svelte";
  import { link } from "svelte-spa-router";
  import {
    getExerciseHistory,
    listExercises,
    type ExerciseHistoryPoint,
  } from "../lib/api";
  import { estimateOneRepMax } from "../lib/oneRepMax";
  import { formatLongDate } from "../lib/date";
  import ProgressChart from "../lib/ProgressChart.svelte";
  import { Card } from "$lib/components/ui/card";
  import ErrorCard from "../lib/ErrorCard.svelte";

  let { params }: { params?: { id?: string } } = $props();
  let exerciseId = $derived(Number(params?.id));

  let name = $state("");
  let points = $state<ExerciseHistoryPoint[]>([]);
  let loading = $state(true);
  let failed = $state(false);

  // Heaviest top-set weight ever recorded.
  const pr = $derived(
    points.length ? Math.max(...points.map((p) => p.weightLb)) : 0,
  );
  // Best estimated 1RM across every session, not just the latest.
  const bestOneRepMax = $derived(
    points.reduce(
      (best, p) => Math.max(best, estimateOneRepMax(p.weightLb, p.reps)),
      0,
    ),
  );

  // Per-session rows (most recent first) with est. 1RM and a flag for sessions
  // that set a new heaviest-weight PR.
  const rows = $derived.by(() => {
    let max = 0;
    const asc = points.map((p) => {
      const isPr = p.weightLb > max;
      if (isPr) max = p.weightLb;
      return {
        ...p,
        e1rm: estimateOneRepMax(p.weightLb, p.reps),
        isPr,
      };
    });
    return asc.reverse();
  });

  async function load() {
    loading = true;
    failed = false;
    const [history, exercises] = await Promise.all([
      getExerciseHistory({ path: { exerciseId } }),
      listExercises(),
    ]);
    if (history.error || !history.data) {
      failed = true;
    } else {
      points = history.data;
      name =
        exercises.data?.find((e) => e.id === exerciseId)?.name ?? "Exercise";
    }
    loading = false;
  }

  onMount(load);
</script>

<div class="flex flex-col gap-6">
  <a
    href="/progress"
    use:link
    class="text-sm text-muted-foreground transition hover:text-foreground"
  >
    ← Progress
  </a>

  {#if loading}
    <Card class="h-40 animate-pulse"></Card>
  {:else if failed}
    <ErrorCard message="Couldn't load this lift's history." onRetry={load} />
  {:else}
    <h2 class="text-2xl font-black text-foreground">{name}</h2>

    {#if points.length === 0}
      <Card class="p-6 text-center">
        <p class="text-sm text-muted-foreground">
          No logged sessions yet for this lift.
        </p>
      </Card>
    {:else}
      <div class="grid grid-cols-2 gap-3">
        <Card class="p-4 text-center">
          <p class="text-xs uppercase tracking-[0.2em] text-muted-foreground">
            Personal record
          </p>
          <p class="mt-1 text-2xl font-black text-primary tabular-nums">{pr} lb</p>
        </Card>
        <Card class="p-4 text-center">
          <p class="text-xs uppercase tracking-[0.2em] text-muted-foreground">
            Best est. 1RM
          </p>
          <p class="mt-1 text-2xl font-black text-primary tabular-nums">
            {bestOneRepMax} lb
          </p>
        </Card>
      </div>

      <Card class="p-4">
        <ProgressChart {points} />
      </Card>

      <Card class="p-4">
        <h3
          class="mb-3 text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground"
        >
          {points.length} session{points.length === 1 ? "" : "s"}
        </h3>
        <table class="w-full text-sm">
          <thead>
            <tr class="text-left text-xs uppercase tracking-wide text-muted-foreground">
              <th class="pb-2 font-medium">Date</th>
              <th class="pb-2 font-medium">Top set</th>
              <th class="pb-2 text-right font-medium">Est. 1RM</th>
            </tr>
          </thead>
          <tbody>
            {#each rows as r, i (i)}
              <tr class="border-t border-border/50">
                <td class="py-1.5 pr-4 text-card-foreground">
                  {formatLongDate(r.performedOn)}
                  {#if r.isPr}<span title="New heaviest weight">🏆</span>{/if}
                </td>
                <td class="py-1.5 pr-4 tabular-nums text-muted-foreground">
                  {r.weightLb} lb × {r.reps}
                </td>
                <td class="py-1.5 text-right tabular-nums text-muted-foreground">
                  {r.e1rm} lb
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </Card>
    {/if}
  {/if}
</div>
