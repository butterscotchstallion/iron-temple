<script lang="ts">
  import { onMount } from "svelte";
  import { push, link } from "svelte-spa-router";
  import {
    getSession,
    getExerciseHistory,
    updateSessionSet,
    deleteSession,
    type Session,
    type SessionSet,
  } from "../lib/api";
  import confetti from "canvas-confetti";
  import RestTimer from "../lib/RestTimer.svelte";
  import PlateBar from "../lib/PlateBar.svelte";
  import { Card } from "$lib/components/ui/card";
  import { Button, buttonVariants } from "$lib/components/ui/button";
  import * as AlertDialog from "$lib/components/ui/alert-dialog";

  let { params }: { params?: { id?: string } } = $props();
  let sessionId = $derived(Number(params?.id));

  let session = $state<Session | null>(null);
  let loading = $state(true);
  let failed = $state(false);

  // Bumped on each set completion to auto-restart the rest timer.
  let restTimerKey = $state(0);
  // Bumped to reset (stop) the rest timer once the whole session is done.
  let restResetKey = $state(0);
  // Controls the "Sets complete" celebration dialog.
  let showComplete = $state(false);

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

  // Each lift's best weight before this session, used to detect a new PR.
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

  // Tailwind classes for a set's circular button by state.
  function setStateClass(set: SessionSet): string {
    if (set.actualReps == null || set.actualReps === 0) {
      return "border-border bg-transparent text-muted-foreground";
    }
    if (set.completed) {
      return "border-primary bg-primary text-primary-foreground"; // hit target
    }
    return "border-primary bg-primary/20 text-foreground"; // in progress
  }

  // Tap a set to add a rep (wrapping to cleared after the target). Each rep tap
  // (re)starts the rest timer; hitting the target on the final set celebrates.
  async function cycle(set: SessionSet) {
    const wasAllComplete = allComplete;
    const reps = nextReps(set);
    const completed = reps != null && reps >= set.targetReps;

    const { data } = await updateSessionSet({
      path: { sessionId, setId: set.id },
      body: { actualReps: reps, completed },
    });
    if (!data || !session) return;
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

    const nowAllComplete = session.sets.every((s) => s.completed);
    if (nowAllComplete && !wasAllComplete) {
      restResetKey += 1;
      showComplete = true;
      confetti({ particleCount: 140, spread: 75, origin: { y: 0.6 } });
    } else {
      restTimerKey += 1;
    }
  }

  // Adjust the working weight for every set of an exercise by delta lb.
  async function changeWeight(sets: SessionSet[], delta: number) {
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
    for (const { data } of results) {
      if (data) {
        session.sets = session.sets.map((s) => (s.id === data.id ? data : s));
      }
    }
  }

  async function remove() {
    const { error } = await deleteSession({ path: { sessionId } });
    if (!error) push("/history");
  }
</script>

<div class="flex flex-col gap-6">
  <a
    href="/"
    use:link
    class="text-sm text-muted-foreground transition hover:text-foreground"
  >
    ← Workout
  </a>

  {#if prMessage}
    <div
      class="rounded-2xl border border-primary/60 bg-primary/15 p-3 text-center"
      role="status"
    >
      <p class="font-black text-primary">🏆 New PR! {prMessage}</p>
    </div>
  {/if}

  {#if loading}
    <Card class="h-40 animate-pulse"></Card>
  {:else if failed}
    <Card class="p-6 text-center" role="alert">
      <p class="text-sm text-muted-foreground">Couldn't load this session.</p>
      <Button variant="outline" class="mt-3" onclick={load}>Retry</Button>
    </Card>
  {:else if session}
    <div class="flex items-start justify-between gap-3">
      <header>
        <h2 class="text-2xl font-black text-foreground">{session.programName}</h2>
        <p class="mt-1 text-sm text-muted-foreground">
          {session.programDayName} · {session.performedOn}
        </p>
        <p class="mt-1 text-xs uppercase tracking-[0.3em] text-primary">
          {loggedCount} / {session.sets.length} sets logged
        </p>
      </header>

      <AlertDialog.Root>
        <AlertDialog.Trigger class={buttonVariants({ variant: "destructive", size: "sm" })}>
          Delete
        </AlertDialog.Trigger>
        <AlertDialog.Content>
          <AlertDialog.Header>
            <AlertDialog.Title>Delete this session?</AlertDialog.Title>
            <AlertDialog.Description>
              This permanently removes the session and its logged sets. This
              can't be undone.
            </AlertDialog.Description>
          </AlertDialog.Header>
          <AlertDialog.Footer>
            <AlertDialog.Cancel>Cancel</AlertDialog.Cancel>
            <AlertDialog.Action variant="destructive" onclick={remove}>
              Delete
            </AlertDialog.Action>
          </AlertDialog.Footer>
        </AlertDialog.Content>
      </AlertDialog.Root>
    </div>

    <Card class="p-6 text-center">
      <h3 class="mb-4 text-xs uppercase tracking-[0.3em] text-muted-foreground">
        Rest Timer
      </h3>
      <RestTimer
        seconds={180}
        autoStartKey={restTimerKey}
        resetKey={restResetKey}
      />
    </Card>

    {#each groups as group (group.name)}
      <Card class="p-5">
        <div class="flex items-center justify-between gap-3">
          <h3 class="text-lg font-bold text-card-foreground">{group.name}</h3>
          <div class="flex items-center gap-3">
            <span class="text-sm tabular-nums text-muted-foreground">
              {group.sets[0].targetReps} reps
            </span>
            <div class="flex items-center gap-1.5">
              <Button
                variant="outline"
                size="icon-sm"
                onclick={() => changeWeight(group.sets, -5)}
                aria-label="Decrease weight by 5 lb"
              >
                −
              </Button>
              <span
                class="min-w-16 text-center text-sm font-bold tabular-nums text-card-foreground"
              >
                {group.sets[0].weightLb} lb
              </span>
              <Button
                variant="outline"
                size="icon-sm"
                onclick={() => changeWeight(group.sets, 5)}
                aria-label="Increase weight by 5 lb"
              >
                +
              </Button>
            </div>
          </div>
        </div>
        <div class="mt-3 flex justify-center">
          <PlateBar weightLb={group.sets[0].weightLb} />
        </div>
        <p class="mt-3 text-xs text-muted-foreground">
          Tap a set to add a rep; it clears after the target.
        </p>
        <div class="mt-3 flex flex-wrap gap-3">
          {#each group.sets as set (set.id)}
            <button
              type="button"
              class="flex size-12 cursor-pointer items-center justify-center rounded-full border text-base font-bold tabular-nums transition {setStateClass(
                set,
              )}"
              onclick={() => cycle(set)}
              aria-label={`Set ${set.setNumber}: ${
                set.actualReps == null ? "not logged" : `${set.actualReps} reps`
              }`}
            >
              {set.actualReps ?? 0}
            </button>
          {/each}
        </div>
      </Card>
    {/each}

    <AlertDialog.Root bind:open={showComplete}>
      <AlertDialog.Content>
        <AlertDialog.Header>
          <AlertDialog.Title>Sets complete 🎉</AlertDialog.Title>
          <AlertDialog.Description>
            {session.programName} · {session.programDayName}
          </AlertDialog.Description>
        </AlertDialog.Header>
        <p class="text-center text-sm text-muted-foreground">
          {session.sets.length} sets · {totalVolume} lb total volume
        </p>
        <AlertDialog.Footer>
          <AlertDialog.Action onclick={() => (showComplete = false)}>
            Nice!
          </AlertDialog.Action>
        </AlertDialog.Footer>
      </AlertDialog.Content>
    </AlertDialog.Root>
  {/if}
</div>
