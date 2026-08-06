<script lang="ts">
  import { onMount } from "svelte";
  import { push, link } from "svelte-spa-router";
  import {
    getSession,
    updateSession,
    updateSessionSet,
    deleteSession,
    type Session,
    type SessionSet,
  } from "../lib/api";
  import confetti from "canvas-confetti";
  import RestTimer from "../lib/RestTimer.svelte";
  import { Card } from "$lib/components/ui/card";
  import { Button, buttonVariants } from "$lib/components/ui/button";
  import { Textarea } from "$lib/components/ui/textarea";
  import * as AlertDialog from "$lib/components/ui/alert-dialog";

  let { params }: { params?: { id?: string } } = $props();
  let sessionId = $derived(Number(params?.id));

  let session = $state<Session | null>(null);
  let loading = $state(true);
  let failed = $state(false);

  let notes = $state("");
  let savingNotes = $state(false);
  let notesSaved = $state(false);

  // Bumped on each set completion to auto-restart the rest timer.
  let restTimerKey = $state(0);
  // Bumped to reset (stop) the rest timer once the whole session is done.
  let restResetKey = $state(0);
  // Controls the "Sets complete" celebration dialog.
  let showComplete = $state(false);

  async function load() {
    loading = true;
    failed = false;
    const { data, error } = await getSession({ path: { sessionId } });
    if (error || !data) {
      failed = true;
    } else {
      session = data;
      notes = data.notes;
    }
    loading = false;
  }

  onMount(load);

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

  const allLogged = $derived(
    (session?.sets.length ?? 0) > 0 &&
      (session?.sets ?? []).every((s) => s.actualReps != null),
  );

  // Total weight moved this session (weight × reps over all logged sets).
  const totalVolume = $derived(
    (session?.sets ?? []).reduce(
      (sum, s) => sum + s.weightLb * (s.actualReps ?? 0),
      0,
    ),
  );

  // The reps a set cycles to on the next tap (StrongLifts style):
  // unlogged -> target (success), then decrement each tap, 1 -> cleared.
  function nextReps(set: SessionSet): number | null {
    if (set.actualReps == null) return set.targetReps;
    if (set.actualReps <= 1) return null;
    return set.actualReps - 1;
  }

  // Tailwind classes for a set's circular button by state.
  function setStateClass(set: SessionSet): string {
    if (set.actualReps == null) {
      return "border-border bg-transparent text-muted-foreground";
    }
    if (set.completed) {
      return "border-primary bg-primary text-primary-foreground"; // hit target
    }
    return "border-primary bg-primary/20 text-foreground"; // logged a miss
  }

  // Tap a set: cycle its reps. `completed` (hit target) drives progression; the
  // first tap that logs a set (re)starts the rest timer, and finishing the last
  // set celebrates.
  async function cycle(set: SessionSet) {
    const firstLog = set.actualReps == null;
    const wasAllLogged = allLogged;
    const reps = nextReps(set);
    const completed = reps != null && reps >= set.targetReps;

    const { data } = await updateSessionSet({
      path: { sessionId, setId: set.id },
      body: { actualReps: reps, completed },
    });
    if (!data || !session) return;
    session.sets = session.sets.map((s) => (s.id === data.id ? data : s));

    // Only the first tap (finishing a set) affects the timer/celebration;
    // subsequent taps just adjust the rep count.
    if (!firstLog) return;

    const nowAllLogged = session.sets.every((s) => s.actualReps != null);
    if (nowAllLogged && !wasAllLogged) {
      restResetKey += 1;
      showComplete = true;
      confetti({ particleCount: 140, spread: 75, origin: { y: 0.6 } });
    } else {
      restTimerKey += 1;
    }
  }

  async function saveNotes() {
    if (!session) return;
    savingNotes = true;
    notesSaved = false;
    const { data } = await updateSession({ path: { sessionId }, body: { notes } });
    savingNotes = false;
    if (data && session) {
      session.notes = data.notes;
      notesSaved = true;
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
        <div class="flex items-baseline justify-between gap-3">
          <h3 class="text-lg font-bold text-card-foreground">{group.name}</h3>
          <span class="text-sm tabular-nums text-muted-foreground">
            {group.sets[0].targetReps} reps · {group.sets[0].weightLb} lb
          </span>
        </div>
        <p class="mt-1 text-xs text-muted-foreground">
          Tap a set to log it; tap again to drop the reps if you missed.
        </p>
        <div class="mt-3 flex flex-wrap gap-3">
          {#each group.sets as set (set.id)}
            <button
              type="button"
              class="flex size-12 items-center justify-center rounded-full border text-base font-bold tabular-nums transition {setStateClass(
                set,
              )}"
              onclick={() => cycle(set)}
              aria-label={`Set ${set.setNumber}: ${
                set.actualReps == null ? "not logged" : `${set.actualReps} reps`
              }`}
            >
              {set.actualReps ?? set.targetReps}
            </button>
          {/each}
        </div>
      </Card>
    {/each}

    <Card class="p-5">
      <h3 class="text-lg font-bold text-card-foreground">Notes</h3>
      <Textarea bind:value={notes} placeholder="How did it feel?" class="mt-3" />
      <div class="mt-3 flex items-center gap-3">
        <Button variant="outline" size="sm" onclick={saveNotes} disabled={savingNotes}>
          {savingNotes ? "Saving…" : "Save notes"}
        </Button>
        {#if notesSaved}
          <span class="text-xs text-muted-foreground">Saved</span>
        {/if}
      </div>
    </Card>

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
