<script lang="ts">
  import { onMount } from "svelte";
  import { link } from "svelte-spa-router";
  import { listPrograms, listSessions, type ProgramSummary } from "../lib/api";
  import { programSubtitle } from "../lib/programs";
  import { currentStreak, STREAK_DISPLAY_THRESHOLD } from "../lib/streak";
  import { Card } from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";

  let programs = $state<ProgramSummary[]>([]);
  let streak = $state(0);
  let loading = $state(true);
  let failed = $state(false);

  // Placeholder cards shown while the request is in flight.
  const skeletons = [0, 1, 2];

  async function load() {
    loading = true;
    failed = false;
    // Programs drive the cards; recent sessions drive the streak badge.
    const [programsRes, sessionsRes] = await Promise.all([
      listPrograms(),
      listSessions({ query: { limit: 100 } }),
    ]);
    if (programsRes.error || !programsRes.data) {
      failed = true;
      loading = false;
      return;
    }
    programs = programsRes.data;
    streak = sessionsRes.data ? currentStreak(sessionsRes.data.items) : 0;
    loading = false;
  }

  onMount(load);
</script>

<div class="flex flex-col gap-8">
  {#if streak >= STREAK_DISPLAY_THRESHOLD}
    <Card class="border-primary/40 bg-primary/5 p-6 text-center ring-primary/30">
      <p class="text-2xl font-black text-primary">🔥 {streak}-session streak</p>
      <p class="mt-0.5 text-xs uppercase tracking-[0.3em] text-muted-foreground">
        Finish every set to keep it alive
      </p>
    </Card>
  {/if}

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
        <a use:link href="/programs/{program.id}" class="group block">
          <Card class="h-full p-5 transition group-hover:ring-primary/60">
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
