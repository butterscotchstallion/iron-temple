<script lang="ts">
  import { onMount } from "svelte";
  import { push, link } from "svelte-spa-router";
  import {
    getProgram,
    previewNextSession,
    createSession,
    type Program,
  } from "../lib/api";
  import { Card } from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";

  let { params }: { params?: { id?: string } } = $props();
  let programId = $derived(Number(params?.id));

  // A day plus its progression-computed next-session prescription.
  type DayView = {
    id: number;
    name: string;
    exercises: {
      exerciseId: number;
      exerciseName: string;
      sets: number;
      reps: number;
      weightLb: number;
    }[];
  };

  let program = $state<Program | null>(null);
  let days = $state<DayView[]>([]);
  let loading = $state(true);
  let failed = $state(false);
  let startingDayId = $state<number | null>(null);
  let startFailed = $state(false);

  async function load() {
    loading = true;
    failed = false;
    const prog = await getProgram({ path: { programId } });
    if (prog.error || !prog.data) {
      failed = true;
      loading = false;
      return;
    }
    program = prog.data;

    // Each day's next-session weights come from the server's progression engine.
    days = await Promise.all(
      prog.data.days.map(async (day) => {
        const preview = await previewNextSession({
          path: { programId, dayId: day.id },
        });
        return {
          id: day.id,
          name: day.name,
          exercises: preview.data?.exercises ?? [],
        };
      }),
    );
    loading = false;
  }

  async function start(dayId: number) {
    startFailed = false;
    startingDayId = dayId;
    const res = await createSession({ body: { programDayId: dayId } });
    startingDayId = null;
    if (res.error || !res.data) {
      startFailed = true;
      return;
    }
    push(`/sessions/${res.data.id}`);
  }

  onMount(load);
</script>

<div class="flex flex-col gap-6">
  <a
    href="/programs"
    use:link
    class="text-sm text-muted-foreground transition hover:text-foreground"
  >
    Change program
  </a>

  {#if loading}
    <Card class="h-40 animate-pulse"></Card>
  {:else if failed}
    <Card class="p-6 text-center" role="alert">
      <p class="text-sm text-muted-foreground">Couldn't load this program.</p>
      <Button variant="outline" class="mt-3" onclick={load}>Retry</Button>
    </Card>
  {:else if program}
    <div>
      <h2 class="text-3xl font-black text-foreground">{program.name}</h2>
      <p class="mt-1 text-sm text-muted-foreground">{program.description}</p>
    </div>

    {#if startFailed}
      <p
        class="rounded-md border border-destructive/40 bg-destructive/10 p-3 text-center text-sm text-destructive"
        role="alert"
      >
        Couldn't start the session. Try again.
      </p>
    {/if}

    {#each days as day (day.id)}
      <Card class="p-5">
        <div class="flex items-center justify-between gap-3">
          <h3 class="text-lg font-bold text-card-foreground">{day.name}</h3>
          <Button
            size="sm"
            onclick={() => start(day.id)}
            disabled={startingDayId !== null}
          >
            {startingDayId === day.id ? "Starting…" : "Start"}
          </Button>
        </div>
        <ul class="mt-3 flex flex-col gap-1.5">
          {#each day.exercises as ex (ex.exerciseId)}
            <li class="flex items-baseline justify-between text-sm">
              <span class="text-card-foreground">{ex.exerciseName}</span>
              <span class="tabular-nums text-muted-foreground">
                {ex.sets}×{ex.reps} · {ex.weightLb} lb
              </span>
            </li>
          {/each}
        </ul>
      </Card>
    {/each}
  {/if}
</div>
