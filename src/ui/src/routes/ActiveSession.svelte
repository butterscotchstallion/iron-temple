<script lang="ts">
  import { onMount } from "svelte";
  import { push, link } from "svelte-spa-router";
  import {
    getSession,
    getExerciseHistory,
    updateSession,
    updateSessionSet,
    addSessionSet,
    removeSessionSet,
    finishSession,
    type Session,
    type SessionSet,
  } from "../lib/api";
  import { invalidateTraining } from "../lib/cache.svelte";
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
    restSeconds = data.sets[0]?.restSeconds ?? restSeconds;
    loading = false;

    // Deliberately not awaited. PRs cost one history request per distinct lift
    // in the session, and they feed nothing but the celebration — so blocking
    // the screen on them made the one page you use mid-workout wait on N
    // requests to show sets that were already in hand.
    //
    // The race this opens is real but benign, and `prReady` is what closes it:
    // a set logged before the histories land doesn't get confetti. That was
    // already the behaviour on a failed history request, and a missed flourish
    // beats a set you couldn't tick because the screen was still spinning.
    void loadPRs();
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

    // track() here, and on the three writes below, is what lets the update
    // prompt reload safely: it holds the reload until every request in the air
    // has landed, so a rep can't be lost between the tap and the response.
    const { data, error } = await track(
      updateSessionSet({
        path: { sessionId, setId: set.id },
        body: { actualReps: reps, completed },
      }),
    );
    if (error) actionError = "Couldn't save that set.";
    if (!data || !session) return;
    actionError = null;
    session.sets = session.sets.map((s) => (s.id === data.id ? data : s));

    // Clearing a set (wrap back to 0) doesn't touch the timer.
    if (reps == null) return;

    // Set before the key is bumped, so the restart below already counts down
    // this lift's rest rather than the previous exercise's.
    restSeconds = set.restSeconds;

    // New PR: a completed set above this lift's prior best.
    if (completed && prReady && set.weightLb > (prBest[set.exerciseId] ?? 0)) {
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
  async function finish() {
    if (finishing) return;
    finishing = true;
    const { data, error } = await track(finishSession({ path: { sessionId } }));
    finishing = false;
    confirmFinish = false;
    if (error || !data) {
      actionError = "Couldn't finish the session.";
      return;
    }
    actionError = null;
    session = data;
    showComplete = true;
    celebrate({ particleCount: 140, spread: 75, origin: { y: 0.6 } });
  }

  // Record (or, with null, erase) what the lifter weighed today. The response is
  // the whole session, so it also refreshes lastWeighIn — no second request to
  // find out what the box should carry next time.
  async function saveBodyweight(weightLb: number | null) {
    const { data, error } = await track(
      updateSession({
        path: { sessionId },
        body: { bodyweightLb: weightLb },
      }),
    );
    if (error || !data) {
      actionError = "Couldn't save your weight.";
      return;
    }
    actionError = null;
    session = data;
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
    // the count reflects what's actually outstanding if some land first.
    const results = await Promise.all(
      sets.map((set) =>
        track(
          updateSessionSet({
            path: { sessionId, setId: set.id },
            body: { weightLb: weightFor(set) },
          }),
        ),
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

  // One more set of the same lift — the extra set, the AMRAP, the day that went
  // better than the prescription. The server copies the rep target and weight
  // from that lift's current last set, so nothing has to be sent but the lift.
  async function addSet(exerciseId: number) {
    if (isOver) return;
    const { data, error } = await track(
      addSessionSet({ path: { sessionId }, body: { exerciseId } }),
    );
    if (error || !data) {
      actionError = "Couldn't add a set.";
      return;
    }
    actionError = null;
    if (!session) return;
    // Appended rather than re-sorted: the server numbers it past the lift's last
    // set, and the group it joins is already in prescription order.
    session.sets = [...session.sets, data];
  }

  // Drop a set that wasn't performed. Removing a lift's last set takes the lift
  // out of the session, which is what skipping it looks like.
  async function removeSet(set: SessionSet) {
    if (isOver) return;
    const { error } = await track(
      removeSessionSet({ path: { sessionId, setId: set.id } }),
    );
    if (error) {
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
