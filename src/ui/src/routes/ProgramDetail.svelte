<script lang="ts">
  import { onMount } from "svelte";
  import { push, link } from "svelte-spa-router";
  import {
    getProgram,
    previewNextSession,
    createSession,
    updateProgramDayWeekday,
    type Program,
  } from "../lib/api";
  import { Card } from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import { WEEKDAYS, todayWeekday } from "../lib/weekday";
  import Calendar from "@lucide/svelte/icons/calendar";

  let { params }: { params?: { id?: string } } = $props();
  let programId = $derived(Number(params?.id));

  // A day plus its progression-computed next-session prescription.
  type DayView = {
    id: number;
    name: string;
    weekday: number | null;
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

  // Float today's scheduled workout to the top so the highlighted day leads.
  // Stable sort keeps every other day in its original order.
  let orderedDays = $derived(
    [...days].sort(
      (a, b) =>
        Number(b.weekday === todayWeekday()) -
        Number(a.weekday === todayWeekday()),
    ),
  );
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
          weekday: day.weekday ?? null,
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

  // Assign (or clear) the weekday this day is scheduled on.
  async function setWeekday(day: DayView, value: string) {
    const weekday = value === "" ? null : Number(value);
    const { error } = await updateProgramDayWeekday({
      path: { programId, dayId: day.id },
      body: { weekday },
    });
    if (!error) {
      days = days.map((d) => (d.id === day.id ? { ...d, weekday } : d));
    }
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

    {#each orderedDays as day (day.id)}
      <Card
        class="p-5 {day.weekday === todayWeekday() ? 'ring-2 ring-primary' : ''}"
      >
        <div class="flex items-center justify-between gap-3">
          <div class="flex flex-wrap items-center gap-2">
            <h3 class="text-lg font-bold text-card-foreground">{day.name}</h3>
            {#if day.weekday === todayWeekday()}
              <span
                class="rounded-full bg-primary px-2 py-0.5 text-[10px] font-bold uppercase tracking-wide text-primary-foreground"
              >
                Today
              </span>
            {/if}
            <label
              class="flex items-center gap-1 rounded-md border border-input px-2 py-1 text-xs text-muted-foreground"
            >
              <Calendar class="size-3.5" />
              <select
                class="bg-transparent outline-none"
                value={day.weekday === null ? "" : String(day.weekday)}
                onchange={(e) => setWeekday(day, e.currentTarget.value)}
              >
                <option value="">Unscheduled</option>
                {#each WEEKDAYS as name, i (i)}
                  <option value={String(i)}>{name}</option>
                {/each}
              </select>
            </label>
          </div>
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
              <a
                use:link
                href="/exercises/{ex.exerciseId}"
                class="text-card-foreground underline-offset-2 transition hover:text-primary hover:underline"
              >
                {ex.exerciseName}
              </a>
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
