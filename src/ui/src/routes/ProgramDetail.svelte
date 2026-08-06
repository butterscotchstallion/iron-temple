<script lang="ts">
  import { onMount } from "svelte";
  import { push, link } from "svelte-spa-router";
  import {
    getProgram,
    previewNextSession,
    createSession,
    type Program,
  } from "../lib/api";

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
    href="/"
    use:link
    class="text-sm uppercase tracking-[0.3em] text-cyan/80 transition hover:text-cyan"
  >
    ← Programs
  </a>

  {#if loading}
    <div class="h-40 animate-pulse rounded-2xl border border-neon/20 bg-surface/50"></div>
  {:else if failed}
    <div
      class="rounded-2xl border border-magenta/40 bg-surface/60 p-6 text-center"
      role="alert"
    >
      <p class="text-sm text-ink/80">Couldn't load this program.</p>
      <button
        class="mt-3 rounded-full border border-cyan/60 bg-cyan/10 px-5 py-2 font-semibold text-ink transition hover:bg-cyan/25"
        onclick={load}
      >
        Retry
      </button>
    </div>
  {:else if program}
    <div>
      <h2 class="text-3xl font-black text-ink">{program.name}</h2>
      <p class="mt-1 text-sm text-ink/70">{program.description}</p>
    </div>

    {#if startFailed}
      <p
        class="rounded-xl border border-magenta/40 bg-surface/60 p-3 text-center text-sm text-ink/80"
        role="alert"
      >
        Couldn't start the session. Try again.
      </p>
    {/if}

    {#each days as day (day.id)}
      <section class="rounded-2xl border border-neon/30 bg-surface/70 p-5 backdrop-blur">
        <div class="flex items-center justify-between gap-3">
          <h3 class="text-lg font-bold text-ink">{day.name}</h3>
          <button
            class="rounded-full border border-neon/60 bg-neon/15 px-4 py-1.5 text-sm font-semibold text-ink transition hover:bg-neon/30 disabled:opacity-40"
            onclick={() => start(day.id)}
            disabled={startingDayId !== null}
          >
            {startingDayId === day.id ? "Starting…" : "Start"}
          </button>
        </div>
        <ul class="mt-3 flex flex-col gap-1.5">
          {#each day.exercises as ex (ex.exerciseId)}
            <li class="flex items-baseline justify-between text-sm">
              <span class="text-ink">{ex.exerciseName}</span>
              <span class="tabular-nums text-ink/70">
                {ex.sets}×{ex.reps} · {ex.weightLb} lb
              </span>
            </li>
          {/each}
        </ul>
      </section>
    {/each}
  {/if}
</div>
