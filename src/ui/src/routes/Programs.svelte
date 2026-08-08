<script lang="ts">
  import { onMount } from "svelte";
  import { link } from "svelte-spa-router";
  import { listPrograms, listSessions, type ProgramSummary } from "../lib/api";
  import { programSubtitle } from "../lib/programs";
  import { Card } from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";

  let programs = $state<ProgramSummary[]>([]);
  // The most recently used program, highlighted so the user can see which one
  // they're currently on. Null for first-time users with no history.
  let currentProgramId = $state<number | null>(null);
  let loading = $state(true);
  let failed = $state(false);

  // Placeholder cards shown while the request is in flight.
  const skeletons = [0, 1, 2];

  async function load() {
    loading = true;
    failed = false;
    const [{ data, error }, sessions] = await Promise.all([
      listPrograms(),
      listSessions({ query: { limit: 1 } }),
    ]);
    if (error || !data) {
      failed = true;
    } else {
      programs = data;
    }
    currentProgramId = sessions.data?.items[0]?.programId ?? null;
    loading = false;
  }

  onMount(load);
</script>

<div class="flex flex-col gap-4">
  <h2 class="text-2xl font-black text-foreground">Choose a program</h2>

  <section class="grid gap-4 sm:grid-cols-3">
    {#if loading}
      {#each skeletons as n (n)}
        <Card class="h-24 animate-pulse" aria-hidden="true"></Card>
      {/each}
    {:else if failed}
      <Card class="col-span-full p-6 text-center" role="alert">
        <p class="text-sm text-muted-foreground">Couldn't load programs.</p>
        <Button variant="outline" class="mt-3" onclick={load}>Retry</Button>
      </Card>
    {:else if programs.length === 0}
      <p class="col-span-full text-center text-sm text-muted-foreground">
        No programs yet.
      </p>
    {:else}
      {#each programs as program (program.id)}
        {@const isCurrent = program.id === currentProgramId}
        <a use:link href="/programs/{program.id}" class="group block">
          <Card
            class="h-full p-5 transition group-hover:ring-primary/60 {isCurrent
              ? 'ring-2 ring-primary'
              : ''}"
          >
            <h2 class="text-lg font-bold text-card-foreground">{program.name}</h2>
            <p class="mt-1 text-sm text-muted-foreground">
              {programSubtitle(program)}
            </p>
          </Card>
        </a>
      {/each}
    {/if}
  </section>
</div>
