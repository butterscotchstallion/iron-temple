<script lang="ts">
  import { onMount } from "svelte";
  import { link } from "svelte-spa-router";
  import { listPrograms, listSessions, type ProgramSummary } from "../lib/api";
  import { auth } from "../lib/auth.svelte";
  import { programSubtitle, programAttribution } from "../lib/programs";
  import { Card } from "$lib/components/ui/card";
  import { Badge } from "$lib/components/ui/badge";
  import { Button } from "$lib/components/ui/button";
  import ClipboardList from "@lucide/svelte/icons/clipboard-list";
  import Plus from "@lucide/svelte/icons/plus";
  import Archive from "@lucide/svelte/icons/archive";
  import ErrorCard from "../lib/ErrorCard.svelte";

  let programs = $state<ProgramSummary[]>([]);
  // The caller's own retired programs, fetched only when asked for. Hidden is
  // the point of archiving, so the common visit stays the short list and pays
  // for nothing it isn't showing.
  let showArchived = $state(false);
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
    const [list, sessions] = await Promise.all([
      listPrograms(showArchived ? { includeArchived: true } : undefined),
      listSessions({ limit: 1 }),
    ]);
    if (list.status !== 200) {
      failed = true;
    } else {
      programs = list.data;
    }
    lastSessionProgramId =
      sessions.status === 200 ? (sessions.data.items[0]?.programId ?? null) : null;
    loading = false;
  }

  onMount(load);

  /** Flips the archived disclosure and refetches, since the server decides. */
  async function toggleArchived() {
    showArchived = !showArchived;
    await load();
  }
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
        <a use:link href="/programs/new" class="mt-3">
          <Button size="sm"><Plus />Build your own</Button>
        </a>
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
            <div class="flex items-start justify-between gap-2">
              <h2 class="text-lg font-bold text-card-foreground">{program.name}</h2>
              <div class="flex shrink-0 gap-1">
                {#if program.archivedAt !== null}
                  <Badge variant="outline">Archived</Badge>
                {/if}
                {#if program.isMine}
                  <Badge variant="secondary">Yours</Badge>
                {/if}
              </div>
            </div>
            <p class="mt-1 text-sm text-muted-foreground">
              {programSubtitle(program)}
            </p>
            <!--
              Who a program came from, on the ones that came from somebody. The
              seeded programs say nothing here: they are the install's, and
              attributing them to nobody would be a line of blank space on eight
              of the nine cards a fresh install draws.
            -->
            {#if programAttribution(program)}
              <p class="mt-1 text-xs text-muted-foreground/80">
                {programAttribution(program)}
              </p>
            {/if}
          </Card>
        </a>
      {/each}

      <!--
        A card rather than a button above the grid, so "build your own" reads as
        one more thing you could pick rather than as an action about the list.
      -->
      <a use:link href="/programs/new" class="group block">
        <Card
          class="flex h-full flex-col items-center justify-center border-dashed p-5 text-center transition group-hover:ring-primary/60"
        >
          <Plus class="size-6 text-muted-foreground/70" aria-hidden="true" />
          <span class="mt-2 font-bold text-card-foreground">Build your own</span>
          <span class="mt-1 text-sm text-muted-foreground">
            From scratch, or a copy of one of these
          </span>
        </Card>
      </a>
    {/if}
  </section>

  {#if !loading && !failed}
    <button
      type="button"
      onclick={toggleArchived}
      class="flex items-center gap-1.5 self-start text-sm text-muted-foreground transition hover:text-foreground"
    >
      <Archive class="size-4" aria-hidden="true" />
      {showArchived ? "Hide archived programs" : "Show archived programs"}
    </button>
  {/if}
</div>
