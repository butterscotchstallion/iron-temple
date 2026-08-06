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

  const completedCount = $derived(
    (session?.sets ?? []).filter((s) => s.completed).length,
  );

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

  // Toggle a set complete/incomplete; on completion, log the target reps.
  async function toggle(set: SessionSet) {
    const completed = !set.completed;
    const wasAllComplete = allComplete;
    const { data } = await updateSessionSet({
      path: { sessionId, setId: set.id },
      body: { completed, actualReps: completed ? set.targetReps : null },
    });
    if (!data || !session) return;

    session.sets = session.sets.map((s) => (s.id === data.id ? data : s));
    if (!completed) return;

    const nowAllComplete = session.sets.every((s) => s.completed);
    if (nowAllComplete && !wasAllComplete) {
      // Session finished: stop the rest timer and celebrate.
      restResetKey += 1;
      showComplete = true;
      confetti({ particleCount: 140, spread: 75, origin: { y: 0.6 } });
    } else {
      // Otherwise a normal set completion (re)starts the rest countdown.
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
          {completedCount} / {session.sets.length} sets done
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
        <h3 class="text-lg font-bold text-card-foreground">{group.name}</h3>
        <div class="mt-3 flex flex-col gap-2">
          {#each group.sets as set (set.id)}
            <Button
              variant={set.completed ? "default" : "outline"}
              class="w-full justify-between"
              onclick={() => toggle(set)}
            >
              <span>Set {set.setNumber} · {set.targetReps} reps · {set.weightLb} lb</span>
              <span class="text-lg">{set.completed ? "✓" : "○"}</span>
            </Button>
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
          <AlertDialog.Action>Nice!</AlertDialog.Action>
        </AlertDialog.Footer>
      </AlertDialog.Content>
    </AlertDialog.Root>
  {/if}
</div>
