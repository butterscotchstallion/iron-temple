<script lang="ts">
  import { onMount } from "svelte";
  import { link } from "svelte-spa-router";
  import { listExercises, listSessions, type Exercise } from "../lib/api";
  import { Card } from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import CalendarHeatmap from "../lib/CalendarHeatmap.svelte";

  let exercises = $state<Exercise[]>([]);
  let sessionDates = $state<string[]>([]);
  let loading = $state(true);
  let failed = $state(false);

  async function load() {
    loading = true;
    failed = false;
    const [exs, sessions] = await Promise.all([
      listExercises(),
      listSessions({ query: { limit: 100 } }),
    ]);
    if (exs.error || !exs.data) {
      failed = true;
    } else {
      exercises = exs.data;
      sessionDates = sessions.data?.items.map((s) => s.performedOn) ?? [];
    }
    loading = false;
  }

  onMount(load);
</script>

<div class="flex flex-col gap-4">
  <h2 class="text-2xl font-black text-foreground">Progress</h2>
  <p class="text-sm text-muted-foreground">Pick a lift to see how it's trending.</p>

  {#if loading}
    <Card class="h-24 animate-pulse"></Card>
  {:else if failed}
    <Card class="p-6 text-center" role="alert">
      <p class="text-sm text-muted-foreground">Couldn't load exercises.</p>
      <Button variant="outline" class="mt-3" onclick={load}>Retry</Button>
    </Card>
  {:else}
    <Card class="p-4">
      <h3
        class="mb-3 text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground"
      >
        Training days
      </h3>
      <CalendarHeatmap dates={sessionDates} />
    </Card>
    <ul class="flex flex-col gap-3">
      {#each exercises as exercise (exercise.id)}
        <li>
          <a use:link href="/exercises/{exercise.id}" class="group block">
            <Card
              class="flex items-center justify-between p-4 transition group-hover:ring-primary/60"
            >
              <span class="font-bold text-card-foreground">{exercise.name}</span>
              <span class="text-sm text-muted-foreground">View →</span>
            </Card>
          </a>
        </li>
      {/each}
    </ul>
  {/if}
</div>
