<script lang="ts">
  import { onMount } from "svelte";
  import { push, link } from "svelte-spa-router";
  import {
    getSession,
    getExerciseHistory,
    updateSessionSet,
    finishSession,
    type Session,
    type SessionSet,
  } from "../lib/api";
  import confetti from "canvas-confetti";
  import ArrowLeft from "@lucide/svelte/icons/arrow-left";
  import Dumbbell from "@lucide/svelte/icons/dumbbell";
  import Flag from "@lucide/svelte/icons/flag";
  import PartyPopper from "@lucide/svelte/icons/party-popper";
  import Trophy from "@lucide/svelte/icons/trophy";
  import RestTimer from "../lib/RestTimer.svelte";
  import ExerciseCard from "../lib/ExerciseCard.svelte";
  import { Card } from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import * as AlertDialog from "$lib/components/ui/alert-dialog";
  import ErrorCard from "../lib/ErrorCard.svelte";
  import ErrorBanner from "../lib/ErrorBanner.svelte";

  let { params }: { params?: { id?: string } } = $props();
  let sessionId = $derived(Number(params?.id));

  let session = $state<Session | null>(null);
  let loading = $state(true);
  let failed = $state(false);
  // Transient failure from a set or weight action (the tap otherwise no-ops).
  let actionError = $state<string | null>(null);

  // Bumped on each set completion to auto-restart the rest timer.
  let restTimerKey = $state(0);
  // Controls the end-of-workout celebration dialog.
  let showComplete = $state(false);
  // Controls the "some sets aren't logged" confirmation before finishing.
  let confirmFinish = $state(false);
  // A finish request is in flight (guards a double-tap).
  let finishing = $state(false);

  // Personal-record tracking: best weight per exercise BEFORE this session.
  let prBest: Record<number, number> = {};
  let prReady = false;
  let prMessage = $state<string | null>(null);
  let prTimer: ReturnType<typeof setTimeout> | undefined;

  async function load() {
    loading = true;
    failed = false;
    const { data, error } = await getSession({ path: { sessionId } });
    if (error || !data) {
      failed = true;
      loading = false;
      return;
    }
    session = data;
    await loadPRs(); // resolve PRs before the session is interactive
    loading = false;
  }

  // Each lift's best weight before this session, used to detect a new PR. A
  // failed history request only disables PR celebration (not core data), so it
  // stays silent and simply won't flag a record for that lift.
  async function loadPRs() {
    if (!session) return;
    const ids = [...new Set(session.sets.map((s) => s.exerciseId))];
    const results = await Promise.all(
      ids.map((id) => getExerciseHistory({ path: { exerciseId: id } })),
    );
    ids.forEach((id, i) => {
      const points = results[i].data ?? [];
      prBest[id] = points.length
        ? Math.max(...points.map((p) => p.weightLb))
        : 0;
    });
    prReady = true;
  }

  onMount(load);

  $effect(() => () => {
    if (prTimer) clearTimeout(prTimer);
  });

  // Sets grouped by exercise, preserving prescription order.
  const groups = $derived.by(() => {
    const byExercise = new Map<string, SessionSet[]>();
    for (const set of session?.sets ?? []) {
      const list = byExercise.get(set.exerciseName) ?? [];
      list.push(set);
      byExercise.set(set.exerciseName, list);
    }
    return [...byExercise.entries()].map(([name, sets]) => ({ name, sets }));
  });

  // A set is "logged" once it has a rep count (success or a miss).
  const loggedCount = $derived(
    (session?.sets ?? []).filter((s) => s.actualReps != null).length,
  );

  // Every set hit its target reps — a clean session.
  const allComplete = $derived(
    (session?.sets.length ?? 0) > 0 &&
      (session?.sets ?? []).every((s) => s.completed),
  );

  // The server decides whether a session is over: finished by hand, or started
  // more than 12 hours ago. An over session is a record and can't be edited.
  const isOver = $derived(session?.isOver ?? false);

  // Nothing to rest from until a rep is on the board, and a session that's over
  // is a record to read rather than a workout to pace — so the timer only
  // exists between those two points.
  const showRestTimer = $derived(loggedCount > 0 && !isOver);

  // Sets with no rep count at all — what the confirm prompt warns about.
  const unloggedCount = $derived(
    (session?.sets ?? []).filter((s) => s.actualReps == null).length,
  );

  // Total weight moved this session (weight × reps over all logged sets).
  const totalVolume = $derived(
    (session?.sets ?? []).reduce(
      (sum, s) => sum + s.weightLb * (s.actualReps ?? 0),
      0,
    ),
  );

  // Reps count up from 0 on each tap, up to the target, then clear.
  function nextReps(set: SessionSet): number | null {
    if (set.actualReps == null) return 1;
    if (set.actualReps >= set.targetReps) return null; // wrap after the target
    return set.actualReps + 1;
  }

  // Tap a set to add a rep (wrapping to cleared after the target). Each rep tap
  // (re)starts the rest timer; hitting the target on the final set celebrates.
  async function cycle(set: SessionSet) {
    if (isOver) return;
    const wasAllComplete = allComplete;
    const reps = nextReps(set);
    const completed = reps != null && reps >= set.targetReps;

    const { data, error } = await updateSessionSet({
      path: { sessionId, setId: set.id },
      body: { actualReps: reps, completed },
    });
    if (error) actionError = "Couldn't save that set.";
    if (!data || !session) return;
    actionError = null;
    session.sets = session.sets.map((s) => (s.id === data.id ? data : s));

    // Clearing a set (wrap back to 0) doesn't touch the timer.
    if (reps == null) return;

    // New PR: a completed set above this lift's prior best.
    if (completed && prReady && set.weightLb > (prBest[set.exerciseId] ?? 0)) {
      prMessage = `${set.exerciseName} · ${set.weightLb} lb`;
      confetti({ particleCount: 120, spread: 70, origin: { y: 0.5 } });
      if (prTimer) clearTimeout(prTimer);
      prTimer = setTimeout(() => (prMessage = null), 6000);
    }

    // Hitting every target ends the workout outright — no need to also press
    // Finish. A miss anywhere leaves it running until the lifter says so.
    const nowAllComplete = session.sets.every((s) => s.completed);
    if (nowAllComplete && !wasAllComplete) {
      await finish(); // takes the rest timer off screen as part of finishing
    } else {
      restTimerKey += 1;
    }
  }

  // Ask first if anything is unlogged, otherwise end it straight away.
  function requestFinish() {
    if (unloggedCount > 0) {
      confirmFinish = true;
      return;
    }
    void finish();
  }

  // End the session for good. The server stamps finishedAt and returns the
  // session with isOver set, which is what locks the screen.
  async function finish() {
    if (finishing) return;
    finishing = true;
    const { data, error } = await finishSession({ path: { sessionId } });
    finishing = false;
    confirmFinish = false;
    if (error || !data) {
      actionError = "Couldn't finish the session.";
      return;
    }
    actionError = null;
    session = data;
    showComplete = true;
    confetti({ particleCount: 140, spread: 75, origin: { y: 0.6 } });
  }

  // Adjust the working weight for every set of an exercise by delta lb.
  async function changeWeight(sets: SessionSet[], delta: number) {
    if (isOver) return;
    const current = sets[0]?.weightLb ?? 0;
    const weightLb = Math.max(0, current + delta);
    if (weightLb === current) return;
    const results = await Promise.all(
      sets.map((set) =>
        updateSessionSet({
          path: { sessionId, setId: set.id },
          body: { weightLb },
        }),
      ),
    );
    if (!session) return;
    if (results.some((r) => r.error)) actionError = "Couldn't update the weight.";
    for (const { data } of results) {
      if (data) {
        session.sets = session.sets.map((s) => (s.id === data.id ? data : s));
      }
    }
  }
</script>

<!-- Extra bottom padding while the timer is up: it's a fixed overlay, so without
     room to scroll past it the pill would cover the last exercise card's set
     buttons on a narrow screen. (It used to be Finish workout underneath; that
     has moved up into the header, but the last card still needs the clearance.) -->
<div class="flex flex-col gap-6 {showRestTimer ? 'pb-24' : ''}">
  <a
    href="/"
    use:link
    class="inline-flex items-center gap-1.5 self-start text-sm text-muted-foreground transition hover:text-foreground"
  >
    <ArrowLeft class="size-4" aria-hidden="true" />
    Workout
  </a>

  {#if prMessage}
    <div
      class="rounded-2xl border border-primary/60 bg-primary/15 p-3 text-center"
      role="status"
    >
      <p class="flex items-center justify-center gap-2 font-black text-primary">
        <Trophy class="size-5" aria-hidden="true" />
        New PR! {prMessage}
      </p>
    </div>
  {/if}

  {#if loading}
    <Card class="h-40 animate-pulse"></Card>
  {:else if failed}
    <ErrorCard message="Couldn't load this session." onRetry={load} />
  {:else if session}
    <div class="flex items-start justify-between gap-3">
      <header>
        <h2 class="text-2xl font-black text-foreground">
          {session.programName}
        </h2>
        <p class="mt-1 text-sm text-muted-foreground">
          {session.programDayName} · {session.performedOn}
        </p>
        <p class="mt-1 text-xs uppercase tracking-[0.3em] text-primary">
          {loggedCount} / {session.sets.length} sets logged
        </p>
        {#if isOver}
          <p
            class="mt-2 text-xs uppercase tracking-[0.3em] text-muted-foreground"
          >
            {session.finishedAt
              ? `Finished · ${new Date(session.finishedAt).toLocaleDateString()}`
              : "Closed automatically · 12h+ old"}
          </p>
        {/if}
      </header>

      <!-- Nothing to finish until a rep is on the board: the session row exists
           from the moment "Start" is tapped on the program day, so an untouched
           workout would otherwise be closable (and unresumable) by a stray tap.
           Same "has it actually begun" test the rest timer uses. The button's
           own variants carry shrink-0 and whitespace-nowrap, so a long program
           name squeezes the heading rather than the button. -->
      {#if !isOver}
        <Button
          size="sm"
          onclick={requestFinish}
          disabled={finishing || loggedCount === 0}
        >
          <Flag />
          {finishing ? "Finishing…" : "Finish workout"}
        </Button>
      {/if}
    </div>

    {#if actionError}
      <ErrorBanner
        message={actionError}
        onDismiss={() => (actionError = null)}
      />
    {/if}

    {#if showRestTimer}
      <RestTimer seconds={180} autoStartKey={restTimerKey} />
    {/if}

    {#each groups as group (group.name)}
      <ExerciseCard
        name={group.name}
        sets={group.sets}
        onCycle={cycle}
        onChangeWeight={(delta) => changeWeight(group.sets, delta)}
        readonly={isOver}
      />
    {/each}

    <!-- Finishing with sets still unlogged is allowed, but worth confirming. -->
    <AlertDialog.Root bind:open={confirmFinish}>
      <AlertDialog.Content>
        <AlertDialog.Header>
          <AlertDialog.Title>Finish with sets unlogged?</AlertDialog.Title>
          <AlertDialog.Description>
            {unloggedCount} of {session.sets.length} sets have no reps logged. Finishing
            closes the workout for good — you won't be able to log them later.
          </AlertDialog.Description>
        </AlertDialog.Header>
        <AlertDialog.Footer>
          <AlertDialog.Cancel>Keep going</AlertDialog.Cancel>
          <AlertDialog.Action onclick={finish}>Finish anyway</AlertDialog.Action>
        </AlertDialog.Footer>
      </AlertDialog.Content>
    </AlertDialog.Root>

    <AlertDialog.Root bind:open={showComplete}>
      <AlertDialog.Content>
        <AlertDialog.Header>
          <AlertDialog.Title class="flex items-center gap-2">
            {#if allComplete}
              <PartyPopper class="size-5 shrink-0" aria-hidden="true" />
              Workout complete
            {:else}
              <Dumbbell class="size-5 shrink-0" aria-hidden="true" />
              Workout finished
            {/if}
          </AlertDialog.Title>
          <AlertDialog.Description>
            {session.programName} · {session.programDayName}
          </AlertDialog.Description>
        </AlertDialog.Header>
        <p class="text-center text-sm text-muted-foreground">
          {loggedCount} / {session.sets.length} sets · {totalVolume} lb total volume
        </p>
        <AlertDialog.Footer>
          <AlertDialog.Action onclick={() => push("/history")}>
            See history
          </AlertDialog.Action>
        </AlertDialog.Footer>
      </AlertDialog.Content>
    </AlertDialog.Root>
  {/if}
</div>
