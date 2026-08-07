<script lang="ts">
  import { onMount } from "svelte";
  import { link } from "svelte-spa-router";
  import { listExercises, type Exercise } from "../lib/api";
  import { Card } from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";

  let exercises = $state<Exercise[]>([]);
  let loading = $state(true);
  let failed = $state(false);

  async function load() {
    loading = true;
    failed = false;
    const { data, error } = await listExercises();
    if (error || !data) {
      failed = true;
    } else {
      exercises = data;
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
