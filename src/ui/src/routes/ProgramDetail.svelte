<script lang="ts">
  import { onMount } from "svelte";
  import { push, link } from "svelte-spa-router";
  import {
    getProgram,
    previewNextSession,
    createSession,
    updateProgramDayWeekday,
    updateMe,
    type Program,
  } from "../lib/api";
  import { auth, setMe } from "../lib/auth.svelte";
  import { Card } from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import { Badge } from "$lib/components/ui/badge";
  import ErrorCard from "../lib/ErrorCard.svelte";
  import ErrorBanner from "../lib/ErrorBanner.svelte";
  import { weekdayOptions, todayWeekday } from "../lib/weekday";
  import Calendar from "@lucide/svelte/icons/calendar";
  import Play from "@lucide/svelte/icons/play";

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
      progression: {
        status: "start" | "advance" | "hold" | "deload";
        failureCount: number;
        failuresBeforeDeload: number;
        previousWeightLb: number;
      };
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
    void remember();

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

  // Save this as the program to land on next time. Opening a program is what
  // counts as selecting it, so this runs on load rather than behind a button.
  //
  // Guarded on the value actually changing, which keeps it to one write per
  // switch: Home renders this component for the program it already resolved,
  // so a re-render is a no-op. The first such render for an account that
  // predates the column does write once, persisting the value Home derived
  // from history — after which the guard holds.
  //
  // A failure is swallowed on purpose. It costs the user a preference, not a
  // workout, and there is nothing they could do about it from here.
  async function remember() {
    if (!auth.me || auth.me.currentProgramId === programId) return;
    const { data } = await updateMe({ body: { currentProgramId: programId } });
    if (data) setMe(data);
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

  // The badge + hint shown for a lift's progression state. Only the interesting
  // states (an impending stall, a deload) get a badge; advancing/first-time lifts
  // stay clean. Returns null when nothing should be shown.
  type ProgressionView = {
    label: string;
    variant: "secondary" | "destructive";
    hint: string;
  };
  function progressionView(
    p: DayView["exercises"][number]["progression"] | undefined,
    weightLb: number,
  ): ProgressionView | null {
    if (!p) return null;
    if (p.status === "deload") {
      return {
        label: "Deload",
        variant: "destructive",
        hint: `stalled ${p.failureCount}× at ${p.previousWeightLb} → ${weightLb} lb`,
      };
    }
    if (p.status === "hold") {
      const left = p.failuresBeforeDeload - p.failureCount;
      return {
        label: `Stalled ${p.failureCount}×`,
        variant: "secondary",
        hint: `${left} more before deload`,
      };
    }
    return null;
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
            <Play />
            {startingDayId === day.id ? "Starting…" : "Start"}
          </Button>
        </div>
        <ul class="mt-3 flex flex-col gap-1.5">
          {#each day.exercises as ex (ex.exerciseId)}
            {@const prog = progressionView(ex.progression, ex.weightLb)}
            <li class="flex flex-col gap-0.5 text-sm">
              <div class="flex items-baseline justify-between gap-2">
                <a
                  use:link
                  href="/exercises/{ex.exerciseId}"
                  class="text-card-foreground underline-offset-2 transition hover:text-primary hover:underline"
                >
                  {ex.exerciseName}
                </a>
                <span class="flex items-center gap-2">
                  {#if prog}
                    <Badge variant={prog.variant}>{prog.label}</Badge>
                  {/if}
                  <span class="tabular-nums text-muted-foreground">
                    {ex.sets}×{ex.reps} · {ex.weightLb} lb
                  </span>
                </span>
              </div>
              {#if prog}
                <span class="text-right text-xs text-muted-foreground">
                  {prog.hint}
                </span>
              {/if}
            </li>
          {/each}
        </ul>
      </Card>
    {/each}
  {/if}
</div>
