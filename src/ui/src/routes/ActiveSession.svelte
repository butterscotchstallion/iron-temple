<script lang="ts">
  import { onMount } from "svelte";
  import { push, link } from "svelte-spa-router";
  import {
    getSession,
    updateSession,
    updateSessionSet,
    addSessionSet,
    removeSessionSet,
    finishSession,
    type Session,
    type SessionSet,
  } from "../lib/api";
  import { invalidateTraining } from "../lib/cache.svelte";
  import { observe } from "../lib/connectivity.svelte";
  import {
    enqueue,
    mustQueue,
    nextTempSetId,
    onDrained,
    type PendingWrite,
  } from "../lib/writeQueue.svelte";
  import type { Options as ConfettiOptions } from "canvas-confetti";
  import ArrowLeft from "@lucide/svelte/icons/arrow-left";
  import Dumbbell from "@lucide/svelte/icons/dumbbell";
  import Flag from "@lucide/svelte/icons/flag";
  import PartyPopper from "@lucide/svelte/icons/party-popper";
  import Trophy from "@lucide/svelte/icons/trophy";
  import RestTimer from "../lib/RestTimer.svelte";
  import ExerciseCard from "../lib/ExerciseCard.svelte";
  import BodyweightCard from "../lib/BodyweightCard.svelte";
  import { Card } from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import * as AlertDialog from "$lib/components/ui/alert-dialog";
  import ErrorCard from "../lib/ErrorCard.svelte";
  import ErrorBanner from "../lib/ErrorBanner.svelte";
  import { track } from "../lib/pendingWrites.svelte";

  let { params }: { params?: { id?: string } } = $props();
  let sessionId = $derived(Number(params?.id));

  let session = $state<Session | null>(null);
  let loading = $state(true);
  let failed = $state(false);
  // Transient failure from a set or weight action (the tap otherwise no-ops).
  let actionError = $state<string | null>(null);

  // Bumped on each set completion to auto-restart the rest timer.
  let restTimerKey = $state(0);
  // How long the timer counts down, taken from the lift whose set was just
  // logged rather than from the session as a whole — the point of the
  // prescription is that a deadlift and a curl disagree about it. Seeded from
  // the day's first exercise so the pill reads correctly before anything is
  // tapped, and replaced on every rep from then on.
  let restSeconds = $state(180);
  // Controls the end-of-workout celebration dialog.
  let showComplete = $state(false);
  // Controls the "some sets aren't logged" confirmation before finishing.
  let confirmFinish = $state(false);
  // A finish request is in flight (guards a double-tap).
  let finishing = $state(false);

  // canvas-confetti is physics that only runs when something goes right, so it
  // is fetched at the first celebration rather than carried in the route's
  // chunk. The promise is cached, so a session full of PRs imports it once.
  type ConfettiFn = (options?: ConfettiOptions) => unknown;
  let confettiLoader: Promise<ConfettiFn> | undefined;

  /**
   * Fire the confetti, loading it if this is the first time.
   *
   * Deliberately not awaited by callers: the celebration must never sit in
   * front of finishing a set. A failed chunk fetch is swallowed for the same
   * reason — losing the confetti is not losing the PR.
   *
   * The cast reconciles a mismatch in the package itself: canvas-confetti's
   * types are written for its CJS entry (`export = confetti`, so TypeScript
   * types the dynamic import as the bare callable), while the bundler resolves
   * its ESM build, which has a real default export. `.default` is what is
   * actually there at runtime.
   */
  function celebrate(options: ConfettiOptions): void {
    const loader = (confettiLoader ??= import("canvas-confetti").then(
      (m) => (m as unknown as { default: ConfettiFn }).default,
    ));
    void loader.then((fire) => fire(options)).catch(() => {});
  }

  // Personal-record tracking: the record to beat per lift, which the session
  // response now carries. It used to be one history request per distinct lift
  // in the session — five or six round trips, each returning a whole training
  // history to yield a single number — which also meant a window after load
  // where a logged set could not be recognised as a record because the
  // histories had not arrived yet. There is no such window now: the numbers
  // come with the sets they are compared against.
  //
  // A lift with no prior performance is simply absent, so `?? 0` reads as "no
  // record to beat" and any completed set clears it.
  const prBest = $derived(
    new Map((session?.previousBests ?? []).map((b) => [b.exerciseId, b.weightLb])),
  );
  let prMessage = $state<string | null>(null);
  let prTimer: ReturnType<typeof setTimeout> | undefined;

  async function load() {
    loading = true;
    failed = false;
    const result = await getSession({ path: { sessionId } });
    // Reads are not queued — there is nothing to replay about a GET — but a
    // read that never lands is still the clearest evidence the app has that it
    // is offline, and it is usually the first request a cold load makes. Told
    // here, the banner is up before the lifter taps anything.
    observe(result);
    const { data, error } = result;
    if (error || !data) {
      failed = true;
      loading = false;
      return;
    }
    session = data;
    restSeconds = data.sets[0]?.restSeconds ?? restSeconds;
    loading = false;
  }

  onMount(load);

  // When the queue finishes replaying, take the server's version of the
  // session wholesale rather than trying to reconcile the optimistic copy
  // against it. This is what makes the temp ids a non-problem on screen: the
  // reload replaces every set, placeholder or not, with the row the server
  // actually has.
  $effect(() => {
    onDrained(() => void load());
    return () => onDrained(null);
  });

  /** What a write produced: the server's row, or the one we assumed. */
  type WriteOutcome<T> = { ok: true; value: T } | { ok: false };

  /**
   * Send a write, or queue it and pretend.
   *
   * The three paths, in the order they are tried:
   *
   *   Already queueing — offline, or writes are waiting ahead of this one. Do
   *   not touch the network; enqueue and hand back the optimistic value.
   *
   *   Sent, but never arrived. Same outcome, decided a round trip later. The
   *   tap is not lost and the lifter is not told anything, because from where
   *   they are standing nothing went wrong.
   *
   *   The server answered. Its version wins, error or otherwise. A refusal is
   *   a real refusal and is reported; queueing it would only retry a request
   *   that has already been declined.
   */
  async function write<T>(
    queued: PendingWrite,
    live: () => Promise<{ data?: T; error?: unknown; response?: Response }>,
    optimistic: () => T,
  ): Promise<WriteOutcome<T>> {
    if (mustQueue()) {
      enqueue(queued);
      return { ok: true, value: optimistic() };
    }

    const result = await track(live());
    if (observe(result)) {
      enqueue(queued);
      return { ok: true, value: optimistic() };
    }
    if (result.error) return { ok: false };
    // A 204 carries no body — removeSet's success looks exactly like this — so
    // the optimistic value stands in as the sentinel.
    return { ok: true, value: result.data ?? optimistic() };
  }

  $effect(() => () => {
    if (prTimer) clearTimeout(prTimer);
    // Whatever was logged while this screen was open — a rep, a weight nudge, a
    // whole finished workout — has moved the streak, the lifetime volume and
    // the top sets that Home, History and Progress cache. Dropped here, once,
    // rather than after each individual write.
    invalidateTraining();
  });

  // Sets grouped by exercise, preserving prescription order. The server already
  // returns main lifts first and assistance after them, so insertion order into
  // the Map is the order to render.
  const groups = $derived.by(() => {
    const byExercise = new Map<string, SessionSet[]>();
    for (const set of session?.sets ?? []) {
      const list = byExercise.get(set.exerciseName) ?? [];
      list.push(set);
      byExercise.set(set.exerciseName, list);
    }
    return [...byExercise.entries()].map(([name, sets]) => ({
      name,
      sets,
      // A group is assistance when its sets are. They cannot disagree — the
      // kind comes from the exercise, and a group is one exercise.
      assistance: sets[0]?.kind === "assistance",
    }));
  });

  // The first assistance group, so a divider can be drawn above it exactly once
  // rather than between every pair of assistance cards.
  const firstAssistanceName = $derived(
    groups.find((g) => g.assistance)?.name ?? null,
  );

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

    // track() inside write(), here and on every mutation below, is what lets
    // the update prompt reload safely: it holds the reload until every request
    // in the air has landed, so a rep can't be lost between the tap and the
    // response. A queued write needs no such protection — it is already on disk.
    const outcome = await write<SessionSet>(
      {
        kind: "updateSet",
        sessionId,
        setId: set.id,
        body: { actualReps: reps, completed },
      },
      () =>
        updateSessionSet({
          path: { sessionId, setId: set.id },
          body: { actualReps: reps, completed },
        }),
      () => ({ ...set, actualReps: reps, completed }),
    );
    if (!outcome.ok) {
      actionError = "Couldn't save that set.";
      return;
    }
    if (!session) return;
    actionError = null;
    const saved = outcome.value;
    session.sets = session.sets.map((s) => (s.id === saved.id ? saved : s));

    // Clearing a set (wrap back to 0) doesn't touch the timer.
    if (reps == null) return;

    // Set before the key is bumped, so the restart below already counts down
    // this lift's rest rather than the previous exercise's.
    restSeconds = set.restSeconds;

    // New PR: a completed set above this lift's prior best.
    if (completed && set.weightLb > (prBest.get(set.exerciseId) ?? 0)) {
      prMessage = `${set.exerciseName} · ${set.weightLb} lb`;
      celebrate({ particleCount: 120, spread: 70, origin: { y: 0.5 } });
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
  // Queued like everything else when there is no network. Finishing is the last
  // thing that happens at the rack, which makes it the write most likely to be
  // made in the worst signal of the session — and refusing it would leave the
  // lifter staring at a workout they have plainly finished.
  async function finish() {
    if (finishing || !session) return;
    finishing = true;
    const current = session;
    const outcome = await write<Session>(
      { kind: "finishSession", sessionId },
      () => finishSession({ path: { sessionId } }),
      () => ({
        ...current,
        isOver: true,
        finishedAt: new Date().toISOString(),
      }),
    );
    finishing = false;
    confirmFinish = false;
    if (!outcome.ok) {
      actionError = "Couldn't finish the session.";
      return;
    }
    actionError = null;
    session = outcome.value;
    showComplete = true;
    celebrate({ particleCount: 140, spread: 75, origin: { y: 0.6 } });
  }

  // Record (or, with null, erase) what the lifter weighed today. The response is
  // the whole session, so it also refreshes lastWeighIn — no second request to
  // find out what the box should carry next time.
  async function saveBodyweight(weightLb: number | null) {
    if (!session) return;
    const current = session;
    const outcome = await write<Session>(
      { kind: "updateSession", sessionId, bodyweightLb: weightLb },
      () =>
        updateSession({
          path: { sessionId },
          body: { bodyweightLb: weightLb },
        }),
      () => ({ ...current, bodyweightLb: weightLb }),
    );
    if (!outcome.ok) {
      actionError = "Couldn't save your weight.";
      return;
    }
    actionError = null;
    session = outcome.value;
  }

  // Adjust an exercise's weight by delta lb.
  //
  // For a uniform block that is every set by the same amount, which is what it
  // has always been. For a ramping lift it is the TOP set by that amount, with
  // every other set scaled to keep its share of it: the rungs of a Madcow day
  // are 50/62.5/75/87.5% of the top set, and shifting them all by a flat 5 lb
  // would leave a ramp that is no longer a ramp of anything.
  //
  // Scaled from the resolved weights rather than from the percentages, which the
  // session does not store — it holds what to lift, not the rule that produced
  // it. Rounded to the nearest 5 lb for the same reason the server rounds.
  async function changeWeight(sets: SessionSet[], delta: number) {
    if (isOver || sets.length === 0) return;

    const top = Math.max(...sets.map((s) => s.weightLb));
    const nextTop = Math.max(0, top + delta);
    if (nextTop === top) return;

    const ramping = sets.some((s) => s.weightLb !== top);
    const weightFor = (set: SessionSet) => {
      if (!ramping) return nextTop;
      if (top === 0) return nextTop;
      return Math.max(0, Math.round((set.weightLb * nextTop) / top / 5) * 5);
    };

    // Each leg is tracked separately rather than the Promise.all as a whole, so
    // the count reflects what's actually outstanding if some land first. They
    // queue independently too, which is safe precisely because each names a
    // different set — the ordering the queue protects is between edits to the
    // SAME row, and there are none here.
    const outcomes = await Promise.all(
      sets.map((set) => {
        const weightLb = weightFor(set);
        return write<SessionSet>(
          { kind: "updateSet", sessionId, setId: set.id, body: { weightLb } },
          () =>
            updateSessionSet({
              path: { sessionId, setId: set.id },
              body: { weightLb },
            }),
          () => ({ ...set, weightLb }),
        );
      }),
    );
    if (!session) return;
    if (outcomes.some((o) => !o.ok)) actionError = "Couldn't update the weight.";
    for (const outcome of outcomes) {
      if (!outcome.ok) continue;
      const saved = outcome.value;
      session.sets = session.sets.map((s) => (s.id === saved.id ? saved : s));
    }
  }

  // One more set of the same lift — the extra set, the AMRAP, the day that went
  // better than the prescription. The server copies the rep target and weight
  // from that lift's current last set, so nothing has to be sent but the lift.
  async function addSet(exerciseId: number) {
    if (isOver || !session) return;
    // What the server would copy from: the lift's current last set. Needed up
    // front because the offline stand-in has to be built from it, and it is the
    // same row the server itself reads.
    const previous = session.sets.filter((s) => s.exerciseId === exerciseId).at(-1);
    if (!previous) return;

    // One id, used by both the queued entry and the row on screen. Allocating
    // it separately in each would leave the replay remapping an id the screen
    // has never heard of, and the two would drift apart silently.
    const tempSetId = nextTempSetId();
    const outcome = await write<SessionSet>(
      { kind: "addSet", sessionId, exerciseId, tempSetId },
      () => addSessionSet({ path: { sessionId }, body: { exerciseId } }),
      // A placeholder with a negative id. It behaves like any other set on
      // screen — it can be tapped, edited, even removed — and is replaced by
      // the real row when the queue drains and the session reloads.
      () => ({
        ...previous,
        id: tempSetId,
        setNumber: previous.setNumber + 1,
        actualReps: null,
        completed: false,
      }),
    );
    if (!outcome.ok) {
      actionError = "Couldn't add a set.";
      return;
    }
    actionError = null;
    if (!session) return;
    // Appended rather than re-sorted: the server numbers it past the lift's last
    // set, and the group it joins is already in prescription order.
    session.sets = [...session.sets, outcome.value];
  }

  // Drop a set that wasn't performed. Removing a lift's last set takes the lift
  // out of the session, which is what skipping it looks like.
  async function removeSet(set: SessionSet) {
    if (isOver) return;
    // void, because a delete answers 204 with no body. Only `ok` is read here.
    const outcome = await write<void>(
      { kind: "removeSet", sessionId, setId: set.id },
      () => removeSessionSet({ path: { sessionId, setId: set.id } }),
      () => undefined,
    );
    if (!outcome.ok) {
      actionError = "Couldn't remove that set.";
      return;
    }
    actionError = null;
    if (!session) return;
    session.sets = session.sets.filter((s) => s.id !== set.id);
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

    <!-- Above the lifting, because weighing in is what you do before you start
         — and because it opens pre-filled, so it needs no attention on the days
         the number hasn't moved. -->
    <BodyweightCard
      bodyweightLb={session.bodyweightLb}
      lastWeighIn={session.lastWeighIn}
      readonly={isOver}
      onSave={saveBodyweight}
    />

    {#if showRestTimer}
      <!-- Keyed by session so the countdown survives a reload — taking an
           update mid-workout, or a stray refresh — without one workout's rest
           ever being restored into the next. -->
      <RestTimer
        seconds={restSeconds}
        autoStartKey={restTimerKey}
        storageKey={String(sessionId)}
      />
    {/if}

    {#each groups as group (group.name)}
      <!-- One rule between the program's work and the lifter's own, so the
           barbell lifts still read as the session and assistance reads as what
           comes after them. -->
      {#if group.name === firstAssistanceName}
        <div class="flex items-center gap-3" aria-hidden="true">
          <span class="h-px flex-1 bg-border/60"></span>
          <span
            class="text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground"
          >
            Assistance
          </span>
          <span class="h-px flex-1 bg-border/60"></span>
        </div>
      {/if}
      <ExerciseCard
        name={group.name}
        sets={group.sets}
        onCycle={cycle}
        onChangeWeight={(delta) => changeWeight(group.sets, delta)}
        onAddSet={() => addSet(group.sets[0].exerciseId)}
        onRemoveSet={removeSet}
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
