<script lang="ts">
  import { onMount } from "svelte";
  import { push, link } from "svelte-spa-router";
  import {
    getProgram,
    previewNextSession,
    createSession,
    updateProgramDayWeekday,
    addAssistance,
    removeAssistance,
    updateMe,
    setBaseline,
    type Layoff,
    type Program,
    type ProgramDayAssistance,
  } from "../lib/api";
  import { auth, setMe } from "../lib/auth.svelte";
  import { Card } from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import { Badge } from "$lib/components/ui/badge";
  import ErrorCard from "../lib/ErrorCard.svelte";
  import ErrorBanner from "../lib/ErrorBanner.svelte";
  import AssistancePicker from "../lib/AssistancePicker.svelte";
  import { weekdayOptions, todayWeekday } from "../lib/weekday";
  import {
    deloadLabel,
    layoffHeadline,
    pctLabel,
    shouldPrompt,
  } from "../lib/layoff";
  import Calendar from "@lucide/svelte/icons/calendar";
  import Play from "@lucide/svelte/icons/play";
  import Plus from "@lucide/svelte/icons/plus";
  import TrendingDown from "@lucide/svelte/icons/trending-down";
  import X from "@lucide/svelte/icons/x";

  let { params }: { params?: { id?: string } } = $props();
  let programId = $derived(Number(params?.id));

  // A day plus its progression-computed next-session prescription.
  //
  // exercises holds the whole prescription, main lifts and assistance alike, as
  // the preview returned it. assistance is the editable list from the program
  // itself, which is what carries the row ids the remove control needs — the
  // preview knows what will be lifted, not which row said so.
  type DayView = {
    id: number;
    name: string;
    weekday: number | null;
    exercises: {
      exerciseId: number;
      exerciseName: string;
      kind: "main" | "assistance";
      sets: number;
      reps: number;
      weightLb: number;
      progression: {
        status: "start" | "advance" | "hold" | "deload" | "layoff" | "fixed";
        failureCount: number;
        failuresBeforeDeload: number;
        previousWeightLb: number;
        layoffPct: number;
      };
    }[];
    assistance: ProgramDayAssistance[];
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

  // Time away from training, as the server measured it — null when there is
  // none, which is the common case and means no prompt.
  let layoff = $state<Layoff | null>(null);
  // The lifter's answer. `deload` rides on every preview and on Start, so what
  // is on screen is what will be lifted; `decided` retires the prompt once
  // either button has been pressed.
  let deload = $state(false);
  let decided = $state(false);
  // The re-previews behind "Deload" are one request per day, so the answer is
  // not instant and Start must not be pressable through it — a session started
  // mid-flight would be prescribed off the answer that hasn't landed yet.
  let deloading = $state(false);
  // Re-previewing after accepting the deload failed, leaving the old weights on
  // screen. Its own flag rather than previewFailed's wording, which would say
  // "couldn't load some workout details" over numbers that loaded fine and are
  // simply not the ones the lifter just asked for.
  let deloadFailed = $state(false);
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
          query: { deload },
        });
        if (preview.error) previewFailed = true;
        // Every day reports the same layoff — it is a fact about the lifter,
        // not about the day — so the last one to answer simply wins.
        if (preview.data) layoff = preview.data.layoff;
        return {
          id: day.id,
          name: day.name,
          weekday: day.weekday ?? null,
          exercises: preview.data?.exercises ?? [],
          assistance: day.assistance,
        };
      }),
    );
    loading = false;
  }

  // Take the deload. Re-previews every day so the lifter sees the weights they
  // just agreed to before pressing Start on any of them — the whole reason the
  // question is asked here rather than behind the Start button.
  //
  // The previews are re-run in place rather than through load(), which would
  // blank the program to a skeleton and re-save the current program over an
  // answer to a question about weights.
  async function acceptDeload() {
    deloadFailed = false;
    deloading = true;
    const previews = await Promise.all(
      days.map((day) =>
        previewNextSession({
          path: { programId, dayId: day.id },
          query: { deload: true },
        }),
      ),
    );
    deloading = false;

    // All of the days or none of them. Start sends one answer for the whole
    // program, so a half-applied deload would put cut weights on some cards and
    // uncut ones on others with no way to tell which the session would use.
    //
    // Nothing is marked decided on the way out, so the prompt comes back and
    // the lifter can simply press it again — a failed request is not an answer.
    if (previews.some((p) => p.error || !p.data)) {
      deloadFailed = true;
      return;
    }
    days = days.map((day, i) => ({
      ...day,
      exercises: previews[i].data?.exercises ?? day.exercises,
    }));
    deload = true;
    decided = true;
  }

  // Keep the prescribed weights. Nothing to reload: the previews on screen were
  // computed without the deload already.
  function declineDeload() {
    deload = false;
    decided = true;
  }

  // The weight the next session will actually prescribe for an assistance lift:
  // carried forward from the last time it was logged, which is what the preview
  // computed. Falls back to the stored weight for a lift never performed, which
  // is also what the server would send.
  function assistanceWeight(day: DayView, entry: ProgramDayAssistance): number {
    const prescribed = day.exercises.find(
      (e) => e.kind === "assistance" && e.exerciseId === entry.exerciseId,
    );
    return prescribed?.weightLb ?? entry.weightLb;
  }

  // Which day has the picker open, if any. One at a time: two open pickers on a
  // three-day program is a lot of screen for a decision about one of them.
  let pickerDayId = $state<number | null>(null);
  // An assistance add or remove failed.
  let assistanceError = $state<string | null>(null);

  // Add an exercise to a day. Returns whether it worked, so the picker can keep
  // the lifter's numbers on screen if it didn't.
  async function add(
    day: DayView,
    choice: { exerciseId: number; sets: number; reps: number; weightLb: number },
  ): Promise<boolean> {
    assistanceError = null;
    const { data, error } = await addAssistance({
      path: { programId, dayId: day.id },
      body: choice,
    });
    if (error || !data) {
      assistanceError = error?.message ?? "Couldn't add that exercise.";
      return false;
    }
    pickerDayId = null;
    // Re-preview this day rather than calling load(): the new entry's weight
    // carries forward from history and is the server's to compute, but a full
    // reload would blank the whole program to a skeleton over one added lift.
    await refreshDay(day.id, [...day.assistance, data]);
    return true;
  }

  // Recompute one day's prescription in place, leaving the rest of the page
  // alone. A failed preview leaves the day's old weights on screen behind the
  // banner, which is better than replacing them with nothing.
  async function refreshDay(dayId: number, assistance: ProgramDayAssistance[]) {
    // Carries the deload answer, so adding a lift after accepting one doesn't
    // re-draw the day at its undeloaded weights.
    const preview = await previewNextSession({
      path: { programId, dayId },
      query: { deload },
    });
    days = days.map((d) =>
      d.id === dayId
        ? { ...d, assistance, exercises: preview.data?.exercises ?? d.exercises }
        : d,
    );
    if (preview.error) previewFailed = true;
  }

  async function remove(day: DayView, entry: ProgramDayAssistance) {
    assistanceError = null;
    const { error } = await removeAssistance({
      path: { programId, dayId: day.id, assistanceId: entry.id },
    });
    if (error) {
      assistanceError = error.message ?? `Couldn't remove ${entry.exerciseName}.`;
      return;
    }
    // Local removal is enough here: nothing else on the card depends on it, so
    // a full reload would only cost a flash of skeleton.
    days = days.map((d) =>
      d.id === day.id
        ? {
            ...d,
            assistance: d.assistance.filter((a) => a.id !== entry.id),
            exercises: d.exercises.filter(
              (e) => !(e.kind === "assistance" && e.exerciseId === entry.exerciseId),
            ),
          }
        : d,
    );
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
    const res = await createSession({ body: { programDayId: dayId, deload } });
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
    if (p.status === "layoff") {
      return {
        label: `Deload −${pctLabel(p.layoffPct)}`,
        variant: "destructive",
        hint: `time off · ${p.previousWeightLb} → ${weightLb} lb`,
      };
    }
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

  // ---- Starting weights -------------------------------------------------
  //
  // A lift with no history starts at the program's seeded weight, and those
  // seeds assume a 45 lb bar. On a rack whose bar is heavier they are not just
  // light, they are unloadable — so the lifter needs a way to say where they
  // actually start. It is offered only while a lift reads "start", because a
  // baseline is consulted only when there is no history and would otherwise be
  // a control that visibly does nothing.

  // The exercise whose starting weight is being edited, and the value typed.
  let baselineFor = $state<number | null>(null);
  let baselineWeight = $state(0);
  let baselineSaving = $state(false);
  let baselineFailed = $state(false);

  function openBaseline(exerciseId: number, current: number) {
    baselineFor = exerciseId;
    baselineWeight = current;
    baselineFailed = false;
  }

  async function saveBaseline() {
    if (baselineFor == null) return;
    baselineSaving = true;
    baselineFailed = false;
    const { error } = await setBaseline({
      path: { exerciseId: baselineFor },
      body: { weightLb: baselineWeight },
    });
    baselineSaving = false;
    if (error) {
      baselineFailed = true;
      return;
    }
    baselineFor = null;
    // Re-preview so the new starting weight shows on the card the lifter is
    // looking at. Through load() rather than in place: a baseline can change
    // more than one day, since the same lift appears on several.
    await load();
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

    {#if assistanceError}
      <ErrorBanner
        message={assistanceError}
        onDismiss={() => (assistanceError = null)}
      />
    {/if}

    {#if deloadFailed}
      <ErrorBanner
        message="Couldn't apply the deload. Your weights are unchanged."
        onDismiss={() => (deloadFailed = false)}
      />
    {/if}

    <!-- Above the days, because it changes every number below it: answering
         re-prescribes the whole program, and the lifter should see what they
         agreed to before they press Start on any of it. -->
    {#if shouldPrompt(layoff, decided) && layoff}
      <Card class="border-primary/40 bg-primary/5 p-5">
        <h3
          class="flex items-center gap-2 text-lg font-bold text-card-foreground"
        >
          <TrendingDown class="size-5 text-primary" aria-hidden="true" />
          Welcome back
        </h3>
        <p class="mt-1 text-sm text-muted-foreground">
          {layoffHeadline(layoff)}. Ease back in with {pctLabel(
            layoff.deloadPct,
          )} off your working weights?
        </p>
        <div class="mt-4 flex flex-wrap gap-2">
          <Button size="sm" onclick={acceptDeload} disabled={deloading}>
            {deloading ? "Deloading…" : deloadLabel(layoff)}
          </Button>
          <Button
            variant="outline"
            size="sm"
            onclick={declineDeload}
            disabled={deloading}
          >
            Keep my weights
          </Button>
        </div>
      </Card>
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
            disabled={startingDayId !== null || deloading}
          >
            <Play />
            {startingDayId === day.id ? "Starting…" : "Start"}
          </Button>
        </div>
        <ul class="mt-3 flex flex-col gap-1.5">
          {#each day.exercises.filter((e) => e.kind !== "assistance") as ex (ex.exerciseId)}
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

              <!-- Only while the lift has no history: that is the whole window
                   in which a starting weight has any effect. -->
              {#if ex.progression?.status === "start"}
                {#if baselineFor === ex.exerciseId}
                  <div class="flex items-center justify-end gap-2">
                    <label class="flex items-center gap-1.5">
                      <span class="sr-only">
                        Starting weight for {ex.exerciseName}
                      </span>
                      <input
                        bind:value={baselineWeight}
                        type="number"
                        min="0"
                        max="2000"
                        step="2.5"
                        class="w-24 rounded-md border border-border/60 bg-input/40 px-2 py-1 text-right text-sm tabular-nums text-foreground outline-none focus:border-primary"
                      />
                      <span class="text-xs text-muted-foreground">lb</span>
                    </label>
                    <Button
                      type="button"
                      size="sm"
                      disabled={baselineSaving}
                      onclick={saveBaseline}
                    >
                      {baselineSaving ? "Saving…" : "Save"}
                    </Button>
                    <Button
                      type="button"
                      size="sm"
                      variant="ghost"
                      onclick={() => (baselineFor = null)}
                    >
                      Cancel
                    </Button>
                  </div>
                  {#if baselineFailed}
                    <span class="text-right text-xs text-destructive">
                      Couldn't save that starting weight.
                    </span>
                  {/if}
                {:else}
                  <button
                    type="button"
                    class="self-end text-xs text-muted-foreground underline-offset-2 transition hover:text-primary hover:underline"
                    onclick={() => openBaseline(ex.exerciseId, ex.weightLb)}
                  >
                    Set starting weight
                  </button>
                {/if}
              {/if}
            </li>
          {/each}
        </ul>

        <!-- Assistance sits below the program's own work, visibly separate,
             because that is exactly what it is: the prescription above is the
             program's and is never edited, and this part is yours. -->
        <div class="mt-4 border-t border-border/60 pt-3">
          <h4
            class="text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground"
          >
            Assistance
          </h4>

          {#if day.assistance.length > 0}
            <ul class="mt-2 flex flex-col gap-1.5">
              {#each day.assistance as entry (entry.id)}
                <li class="flex items-baseline justify-between gap-2 text-sm">
                  <a
                    use:link
                    href="/exercises/{entry.exerciseId}"
                    class="text-card-foreground underline-offset-2 transition hover:text-primary hover:underline"
                  >
                    {entry.exerciseName}
                  </a>
                  <span class="flex items-center gap-2">
                    <span class="tabular-nums text-muted-foreground">
                      {entry.sets}×{entry.reps}
                      {#if assistanceWeight(day, entry) > 0}
                        · {assistanceWeight(day, entry)} lb
                      {:else}
                        · bodyweight
                      {/if}
                    </span>
                    <button
                      type="button"
                      class="rounded-md p-1 text-muted-foreground transition hover:text-destructive"
                      aria-label="Remove {entry.exerciseName} from {day.name}"
                      onclick={() => remove(day, entry)}
                    >
                      <X class="size-4" aria-hidden="true" />
                    </button>
                  </span>
                </li>
              {/each}
            </ul>
          {:else if pickerDayId !== day.id}
            <p class="mt-1 text-sm text-muted-foreground">
              Nothing yet — add accessory work to finish this day off.
            </p>
          {/if}

          {#if pickerDayId === day.id}
            <!-- Hide what can't be added: the lifts already on this day as
                 assistance, and the ones the program itself prescribes. Adding
                 the latter is a 409, and offering a choice that can only fail
                 is worse than not offering it. -->
            <AssistancePicker
              exclude={[
                ...day.assistance.map((a) => a.exerciseId),
                ...day.exercises
                  .filter((e) => e.kind !== "assistance")
                  .map((e) => e.exerciseId),
              ]}
              onAdd={(choice) => add(day, choice)}
              onCancel={() => (pickerDayId = null)}
            />
          {:else}
            <Button
              variant="outline"
              size="sm"
              class="mt-2"
              onclick={() => (pickerDayId = day.id)}
            >
              <Plus />
              Add assistance
            </Button>
          {/if}
        </div>
      </Card>
    {/each}
  {/if}
</div>
