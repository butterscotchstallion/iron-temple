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
  import ErrorCard from "../lib/ErrorCard.svelte";
  import ErrorBanner from "../lib/ErrorBanner.svelte";
  import { weekdayOptions, todayWeekday } from "../lib/weekday";
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

  // Weekday choices labeled with each day's next upcoming date, e.g.
  // "Friday, August 7". Computed once from today when the card mounts.
  const dayChoices = weekdayOptions();

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
  // One or more days' next-session preview couldn't be computed.
  let previewFailed = $state(false);
  // A weekday assignment couldn't be saved.
  let weekdayFailed = $state(false);

  async function load() {
    loading = true;
    failed = false;
    previewFailed = false;
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
        if (preview.error) previewFailed = true;
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

  // Assign (or clear) the weekday this day is scheduled on. Takes the <select>
  // element so a failed save can roll the control back to the saved value.
  async function setWeekday(day: DayView, select: HTMLSelectElement) {
    weekdayFailed = false;
    const value = select.value;
    const weekday = value === "" ? null : Number(value);
    const { error } = await updateProgramDayWeekday({
      path: { programId, dayId: day.id },
      body: { weekday },
    });
    if (error) {
      // The one-way `value={...}` binding won't re-assert the old value when
      // `day.weekday` is unchanged, so reset the DOM control explicitly.
      select.value = day.weekday === null ? "" : String(day.weekday);
      weekdayFailed = true;
    } else {
      days = days.map((d) => (d.id === day.id ? { ...d, weekday } : d));
    }
  }

  onMount(load);
</script>

<div class="flex flex-col gap-6">
  {#if loading}
    <Card class="h-40 animate-pulse"></Card>
  {:else if failed}
    <ErrorCard message="Couldn't load this program." onRetry={load} />
  {:else if program}
    <div>
      <h2 class="text-3xl font-black text-foreground">{program.name}</h2>
      <p class="mt-1 text-sm text-muted-foreground">{program.description}</p>
    </div>

    {#if startFailed}
      <ErrorBanner
        message="Couldn't start the session. Try again."
        onDismiss={() => (startFailed = false)}
      />
    {/if}

    {#if weekdayFailed}
      <ErrorBanner
        message="Couldn't update the schedule."
        onDismiss={() => (weekdayFailed = false)}
      />
    {/if}

    {#if previewFailed}
      <ErrorBanner
        message="Couldn't load some workout details."
        onRetry={load}
        onDismiss={() => (previewFailed = false)}
      />
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
                onchange={(e) => setWeekday(day, e.currentTarget)}
              >
                <option value="">Unscheduled</option>
                {#each dayChoices as choice (choice.value)}
                  <option value={String(choice.value)}>{choice.label}</option>
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
