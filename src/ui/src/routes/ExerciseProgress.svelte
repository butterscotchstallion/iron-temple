<script lang="ts">
  import { onMount } from "svelte";
  import { link } from "svelte-spa-router";
  import {
    getExerciseHistory,
    listExercises,
    type ExerciseHistoryPoint,
  } from "../lib/api";
  import { estimateOneRepMax } from "../lib/oneRepMax";
  import ProgressChart from "../lib/ProgressChart.svelte";
  import { Card } from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";

  let { params }: { params?: { id?: string } } = $props();
  let exerciseId = $derived(Number(params?.id));

  let name = $state("");
  let points = $state<ExerciseHistoryPoint[]>([]);
  let loading = $state(true);
  let failed = $state(false);

  const pr = $derived(
    points.length ? Math.max(...points.map((p) => p.weightLb)) : 0,
  );
  const latest = $derived(points.length ? points[points.length - 1] : null);
  const oneRepMax = $derived(
    latest ? estimateOneRepMax(latest.weightLb, latest.reps) : 0,
  );

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
    <Card class="p-6 text-center" role="alert">
      <p class="text-sm text-muted-foreground">Couldn't load this lift's history.</p>
      <Button variant="outline" class="mt-3" onclick={load}>Retry</Button>
    </Card>
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
            Est. 1RM
          </p>
          <p class="mt-1 text-2xl font-black text-primary tabular-nums">
            {oneRepMax} lb
          </p>
        </Card>
      </div>

      <Card class="p-4">
        <ProgressChart {points} />
      </Card>
    {/if}
  {/if}
</div>
