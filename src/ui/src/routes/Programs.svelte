<script lang="ts">
  import { onMount } from "svelte";
  import { link } from "svelte-spa-router";
  import { listPrograms, listSessions, type ProgramSummary } from "../lib/api";
  import { auth } from "../lib/auth.svelte";
  import { programSubtitle } from "../lib/programs";
  import { Card } from "$lib/components/ui/card";
  import ClipboardList from "@lucide/svelte/icons/clipboard-list";
  import ErrorCard from "../lib/ErrorCard.svelte";

  let programs = $state<ProgramSummary[]>([]);
  // The program the user is on, highlighted so they can see which one. Same
  // expression as Home's, so the ring can't disagree with what "/" opens:
  // the saved program, else the most recently used one, else nothing for a
  // first-time user with no history.
  let lastSessionProgramId = $state<number | null>(null);
  let currentProgramId = $derived(auth.me?.currentProgramId ?? lastSessionProgramId);
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
    lastSessionProgramId = sessions.data?.items[0]?.programId ?? null;
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
      <ErrorCard
        class="col-span-full"
        message="Couldn't load programs."
        onRetry={load}
      />
    {:else if programs.length === 0}
      <div class="col-span-full flex flex-col items-center text-center">
        <ClipboardList class="size-8 text-muted-foreground/60" aria-hidden="true" />
        <p class="mt-3 text-sm text-muted-foreground">No programs yet.</p>
      </div>
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
