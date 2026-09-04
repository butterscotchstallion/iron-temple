<script lang="ts">
  import { onMount } from "svelte";
  import { link } from "svelte-spa-router";
  import { getExerciseHistory, type ExerciseHistoryPoint } from "../lib/api";
  import { estimateOneRepMax } from "../lib/oneRepMax";
  import { formatLongDate } from "../lib/date";
  import ProgressChart from "../lib/ProgressChart.svelte";
  import { Card } from "$lib/components/ui/card";
  import ArrowLeft from "@lucide/svelte/icons/arrow-left";
  import ChartLine from "@lucide/svelte/icons/chart-line";
  import Trophy from "@lucide/svelte/icons/trophy";
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
    // One request. This used to fetch the WHOLE exercise library alongside the
    // history — all 53 movements — so it could look up a single name in it; the
    // lift now arrives named by its own history endpoint.
    const { data, error } = await getExerciseHistory({ path: { exerciseId } });
    if (error || !data) {
      failed = true;
    } else {
      points = data.points;
      name = data.exerciseName;
    }
    loading = false;
  }

  onMount(load);
</script>

<div class="flex flex-col gap-6">
  <a
    href="/progress"
    use:link
    class="inline-flex items-center gap-1.5 self-start text-sm text-muted-foreground transition hover:text-foreground"
  >
    <ArrowLeft class="size-4" aria-hidden="true" />
    Progress
  </a>

  {#if loading}
    <Card class="h-40 animate-pulse"></Card>
  {:else if failed}
    <ErrorCard message="Couldn't load this lift's history." onRetry={load} />
  {:else}
    <h2 class="text-2xl font-black text-foreground">{name}</h2>

    {#if points.length === 0}
      <Card class="flex flex-col items-center p-6 text-center">
        <ChartLine class="size-8 text-muted-foreground/60" aria-hidden="true" />
        <p class="mt-3 text-sm text-muted-foreground">
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
                  <!-- The icon is aria-hidden, so the label it replaces (the
                       trophy emoji, which announced as "trophy") is restated
                       for screen readers rather than left to the tooltip. -->
                  {#if r.isPr}<span title="New heaviest weight">
                      <Trophy
                        class="inline size-3.5 align-[-0.15em] text-primary"
                        aria-hidden="true"
                      />
                      <span class="sr-only">New heaviest weight</span>
                    </span>{/if}
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
