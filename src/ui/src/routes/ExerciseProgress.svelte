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
  import Loading from "../lib/skeleton/Loading.svelte";
  import Skeleton from "../lib/skeleton/Skeleton.svelte";
  import SkeletonTiles from "../lib/skeleton/SkeletonTiles.svelte";
  import FormFigure from "../lib/FormFigure.svelte";
  import { exerciseDemo } from "../lib/exerciseDemo";

  let { params }: { params?: { id?: string } } = $props();
  let exerciseId = $derived(Number(params?.id));

  let name = $state("");
  let points = $state<ExerciseHistoryPoint[]>([]);
  let loading = $state(true);
  let failed = $state(false);

  // Rows in the placeholder session table.
  //
  // This reserves the shape of a lift WITH a history, which is a deliberate bet
  // rather than an oversight: this screen is reached from /progress, and that
  // page lists only lifts that have one. A movement opened from the library and
  // never performed collapses to its empty state instead, which is the rarer
  // path and the one that can afford the jump.
  const SKELETON_SESSIONS = [0, 1, 2, 3, 4];

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
    const history = await getExerciseHistory(exerciseId);
    if (history.status !== 200) {
      failed = true;
    } else {
      points = history.data.points;
      name = history.data.exerciseName;
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
    <!-- Four blocks, because the loaded page is four: the lift's name, the two
         record tiles, the chart, and the session table. It used to be a single
         h-40 card, which meant every lift with any history at all pushed the
         page down by several hundred pixels the moment it landed. -->
    <Loading label="Loading this lift" class="flex flex-col gap-6">
      <Skeleton text="2xl" class="w-48" />
      <SkeletonTiles count={2} class="grid-cols-2 sm:grid-cols-2" />
      <Card class="p-4">
        <!-- The chart is a `w-full` SVG on a fixed 320×190 viewBox, so its
             rendered height is the column's width times that ratio. An
             aspect-ratio box tracks it at every breakpoint; a fixed `h-48`
             would be right at one width and wrong at the rest. -->
        <Skeleton class="aspect-[320/190] w-full" />
      </Card>
      <Card class="p-4">
        <Skeleton text="xs" class="mb-3 w-24" />
        <div class="flex items-center gap-4 pb-2">
          <Skeleton text="xs" class="w-16" />
          <Skeleton text="xs" class="w-16" />
          <Skeleton text="xs" class="ml-auto w-16" />
        </div>
        {#each SKELETON_SESSIONS as n (n)}
          <div class="flex items-center gap-4 border-t border-border/50 py-1.5">
            <Skeleton text="sm" class="w-32" />
            <Skeleton text="sm" class="w-24" />
            <Skeleton text="sm" class="ml-auto w-16" />
          </div>
        {/each}
      </Card>
    </Loading>
  {:else if failed}
    <ErrorCard message="Couldn't load this lift's history." onRetry={load} />
  {:else}
    <h2 class="text-2xl font-black text-foreground">{name}</h2>

    <!-- Above the history, and deliberately outside the empty-history branch
         below: a lifter who has never performed the movement is exactly the one
         who wants to see how it goes. Renders nothing at all for a movement
         with no figure, so there is no empty state to design. -->
    {#if exerciseDemo(name)}
      <Card class="p-4">
        <h3
          class="mb-2 text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground"
        >
          How it goes
        </h3>
        <FormFigure {name} class="h-48" />
      </Card>
    {/if}

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
