<script lang="ts">
  import { onMount } from "svelte";
  import { push, link } from "svelte-spa-router";
  import {
    getSession,
    updateSession,
    updateSessionSet,
    addSessionSet,
    removeSessionSet,
    addSessionAssistance,
    finishSession,
    type Exercise,
    type Session,
    type SessionSet,
  } from "../lib/api";
  import { isOk } from "../lib/apiFetch";
  import { invalidateTraining } from "../lib/cache.svelte";
  import { auth } from "../lib/auth.svelte";
  import { loadLevels } from "../lib/levels.svelte";
  import { observe } from "../lib/connectivity.svelte";
  import {
    enqueue,
    mustQueue,
    nextTempSetId,
    onDrained,
    type PendingWrite,
  } from "../lib/writeQueue.svelte";
  import { displayLb, weightUnitLabel } from "../lib/library";
  import { celebrate } from "../lib/celebrate";
  import { formatLongDate } from "../lib/date";
  import { handOffSession } from "../lib/recapHandoff";
  import ArrowLeft from "@lucide/svelte/icons/arrow-left";
  import Flag from "@lucide/svelte/icons/flag";
  import Trophy from "@lucide/svelte/icons/trophy";
  import Sprout from "@lucide/svelte/icons/sprout";
  import RestTimer from "../lib/RestTimer.svelte";
  import ExerciseCard from "../lib/ExerciseCard.svelte";
  import AssistancePicker from "../lib/AssistancePicker.svelte";
  import Plus from "@lucide/svelte/icons/plus";
  import BodyweightCard from "../lib/BodyweightCard.svelte";
  import Timestamp from "../lib/Timestamp.svelte";
  import { Card } from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import * as AlertDialog from "$lib/components/ui/alert-dialog";
  import ErrorCard from "../lib/ErrorCard.svelte";
  import ErrorBanner from "../lib/ErrorBanner.svelte";
  import Loading from "../lib/skeleton/Loading.svelte";
  import Skeleton from "../lib/skeleton/Skeleton.svelte";
  import { track } from "../lib/pendingWrites.svelte";

  let { params }: { params?: { id?: string } } = $props();
  let sessionId = $derived(Number(params?.id));

  // Lifts in the loading placeholder. Every program day shipped here prescribes
  // three or four movements before any assistance is bolted on.
  const SKELETON_LIFTS = [0, 1, 2];

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
  // Controls the "some sets aren't logged" confirmation before finishing.
  let confirmFinish = $state(false);
  // A finish request is in flight (guards a double-tap).
  let finishing = $state(false);
  // Whether the assistance picker is expanded. One at a time, and only while
  // the session is open.
  let pickerOpen = $state(false);

  // Personal-record tracking: the record to beat per lift, which the session
  // response now carries. It used to be one history request per distinct lift
  // in the session — five or six round trips, each returning a whole training
  // history to yield a single number — which also meant a window after load
  // where a logged set could not be recognised as a record because the
  // histories had not arrived yet. There is no such window now: the numbers
  // come with the sets they are compared against.
  //
  // A lift with no prior performance is simply ABSENT from this map, and that
  // absence is load-bearing rather than a gap to default away. It means there is
  // no record to beat, so the first set of a lift the lifter has never done is a
  // first time and not a personal best — see personalRecords in internal/racked.
  // Read instead as a zero to clear, it made every lift of an opening workout a
  // PR, complete with confetti, which is what left a real one feeling like
  // nothing in particular.
  const prBest = $derived(
    new Map((session?.previousBests ?? []).map((b) => [b.exerciseId, b.weightLb])),
  );
  // What to say over the sets, if anything. Carries WHICH of the two it is
  // rather than just the text, so the banner cannot call a first time a PR.
  let prNote = $state<{ kind: "pr" | "first"; text: string } | null>(null);

  // The heaviest weight already celebrated for each lift in THIS session, so a
  // 5x5 at a new weight fires the confetti once rather than five times.
  //
  // Not $state and deliberately so: nothing renders from it. It only gates a side
  // effect, and making it reactive would invite a reader to draw from a map that
  // is written to mid-update. It resets with the page, which is the right lifetime
  // — a reload means the celebration already happened.
  const celebrated = new Map<number, number>();

  // What each lift has left to chase, keyed the way the cards ask for it.
  //
  // The server decides the rung: which weights count as named is a fact about the
  // equipment and it already owns that ladder, so nothing here reimplements it —
  // see racked.NextRung. Null means there is nothing to aim at, which covers a
  // lifter past the top of a ladder, a lift never loaded, and band work.
  const rungByExercise = $derived(
    new Map(
      (session?.previousBests ?? [])
        .filter((b) => b.nextRungLb !== null)
        .map((b) => [b.exerciseId, { targetLb: b.nextRungLb!, currentLb: b.weightLb }]),
    ),
  );

  function rungFor(exerciseId: number) {
    return rungByExercise.get(exerciseId) ?? null;
  }
  let prTimer: ReturnType<typeof setTimeout> | undefined;

  async function load() {
    loading = true;
    failed = false;
    const result = await getSession(sessionId);
    // Reads are not queued — there is nothing to replay about a GET — but a
    // read that never lands is still the clearest evidence the app has that it
    // is offline, and it is usually the first request a cold load makes. Told
    // here, the banner is up before the lifter taps anything.
    observe(result);
    if (result.status !== 200) {
      failed = true;
      loading = false;
      return;
    }
    session = result.data;
    restSeconds = result.data.sets[0]?.restSeconds ?? restSeconds;
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
    live: () => Promise<{ status: number; data: unknown }>,
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
    if (!isOk(result.status)) return { ok: false };
    // A 204 carries no body — removeSet's success looks exactly like this — so
    // the optimistic value stands in as the sentinel.
    //
    // The cast is the price of taking the response union as `unknown` rather
    // than threading each operation's success type through this one helper:
    // every caller pairs a `live` with an `optimistic` that returns the same
    // row type, so on a 2xx the body IS T. The alternative is a conditional
    // type over five different response unions to express what the pairing
    // already guarantees, in a helper each call site can read at a glance.
    return { ok: true, value: (result.data as T | undefined) ?? optimistic() };
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

  // Movements already in this workout, so the picker cannot offer one that would
  // only come back as a 409. The optimistic rows count, which is what stops a
  // second offline add of the same lift being queued behind the first.
  const alreadyHere = $derived([
    ...new Set((session?.sets ?? []).map((s) => s.exerciseId)),
  ]);

  // Sets with no rep count at all — what the confirm prompt warns about.
  const unloggedCount = $derived(
    (session?.sets ?? []).filter((s) => s.actualReps == null).length,
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
      () => updateSessionSet(sessionId, set.id, { actualReps: reps, completed }),
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

    // Two different pieces of news, and only one of them is a record.
    //
    // A completed set above this lift's prior best is a PR and gets the confetti.
    // A completed set of a lift with NO prior best is a first time: still worth
    // marking, because a lifter who just did something for the first time should
    // hear so, but without the confetti — that is reserved for beating
    // something, and a first workout would otherwise fire it on every lift.
    if (completed) {
      const previous = prBest.get(set.exerciseId);
      // In the units the card beside it is in — a note saying 70 lb over a
      // card reading 35 lb each would read as news about some other lift.
      const text = `${set.exerciseName} · ${displayLb(set.weightLb, set.equipment)} ${weightUnitLabel(set.equipment)}`;
      // A NEW note, not merely a note on screen. Testing prNote itself would let
      // an ordinary completed set re-arm the dismissal of whatever is already up,
      // and on a 5x5 that is four more chances each — the banner would sit there
      // for the rest of the workout, announcing a record set ten minutes ago.
      let fresh = false;
      if (previous === undefined) {
        prNote = { kind: "first", text };
        fresh = true;
      } else if (set.weightLb > previous) {
        prNote = { kind: "pr", text };
        fresh = true;
        // ONCE PER LIFT, not once per set. prBest is the standing best as of page
        // load and never advances, so on a 5x5 at a new weight all five sets clear
        // the same old mark and each one used to fire — five bursts of confetti for
        // one achievement, which is precisely how a celebration stops landing.
        //
        // The record itself is still five records' worth of news as far as the
        // banner is concerned: it re-reads on every set, because the lifter should
        // see the note against the set they just did. It is the confetti that is
        // rationed, because that is the part that means "this was rare".
        if ((celebrated.get(set.exerciseId) ?? -1) < set.weightLb) {
          celebrated.set(set.exerciseId, set.weightLb);
          celebrate({ particleCount: 120, spread: 70, origin: { y: 0.5 } });
        }
      }
      if (fresh) {
        if (prTimer) clearTimeout(prTimer);
        prTimer = setTimeout(() => (prNote = null), 6000);
      }
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
      () => finishSession(sessionId),
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

    // The one session whose experience somebody is waiting on. The badge beside a
    // name is polled every ten minutes, which is right for everybody else's and
    // wrong for the lifter who has this second earned it — their own name is in
    // the header of the page they are about to land on. Not awaited and not
    // checked: it is an ornament, the poll will catch it either way, and offline
    // this cannot land any more than the finish itself could.
    //
    // The id is passed, so this is also where a level-up is announced. The server
    // publishes a `level` frame for the same finish and this client receives it
    // like any other, so there are two paths to the same news — and only one
    // announcement, because whichever reading arrives first moves the watch's
    // baseline and the other finds nothing new. Keeping both is what makes the
    // moment survive a socket that never connected.
    void loadLevels(auth.me?.id);

    // Hand the finished session across before navigating. The recap asks the
    // server for the full story, but that is a GET — offline it cannot land,
    // and neither could a refetch of this session. Passing the object we
    // already hold is what lets the recap paint at the rack; see recapHandoff.
    handOffSession(outcome.value);
    push(`/sessions/${sessionId}/recap`);
  }

  // Record (or, with null, erase) what the lifter weighed today. The response is
  // the whole session, so it also refreshes lastWeighIn — no second request to
  // find out what the box should carry next time.
  async function saveBodyweight(weightLb: number | null) {
    if (!session) return;
    const current = session;
    const outcome = await write<Session>(
      { kind: "updateSession", sessionId, bodyweightLb: weightLb },
      () => updateSession(sessionId, { bodyweightLb: weightLb }),
      () => ({ ...current, bodyweightLb: weightLb }),
    );
    if (!outcome.ok) {
      actionError = "Couldn't save your weight.";
      return;
    }
    actionError = null;
    session = outcome.value;
  }

  // Adjust an exercise's weight by delta lb, as the WHOLE load.
  //
  // Stored pounds, not the bells on the card: ExerciseCard shows a dumbbell per
  // bell and steps by 5, and converts back in `stepWeight` before calling this.
  // Everything below — the top set, the ramp scaling, the round to 5 — is in the
  // units the session stores, which is the only way it can agree with the weights
  // already on the rows.
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
          () => updateSessionSet(sessionId, set.id, { weightLb }),
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
      () => addSessionSet(sessionId, { exerciseId }),
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

  // A whole lift, decided on at the rack.
  //
  // This is the only write on this screen that reaches past the session: the
  // server puts the movement on the program day too, so it is prescribed every
  // time that day comes round, with carry-forward and rep progression behind it
  // rather than being a one-off with no future. Both halves are one transaction
  // server-side — see POST /sessions/{id}/assistance.
  //
  // Returns whether it landed, because that is what AssistancePicker wants: on a
  // refusal it keeps the numbers the lifter typed rather than making them pick
  // the movement again.
  async function addAssistanceLift(
    choice: {
      exerciseId: number;
      sets: number;
      reps: number;
      weightLb: number;
      repMin?: number;
      repMax?: number;
    },
    exercise: Exercise,
  ): Promise<boolean> {
    if (isOver || !session) return false;

    // One id per set, allocated here so the queued entry and the rows on screen
    // name the same placeholders — the replay repoints these exact ids.
    const tempSetIds = Array.from({ length: choice.sets }, () => nextTempSetId());

    const outcome = await write<SessionSet[]>(
      {
        kind: "addAssistance",
        sessionId,
        exerciseId: choice.exerciseId,
        reps: choice.reps,
        weightLb: choice.weightLb,
        repMin: choice.repMin,
        repMax: choice.repMax,
        tempSetIds,
      },
      () => addSessionAssistance(sessionId, { ...choice }),
      // Buildable in full, which is why this can be queued at all: the picker
      // handed over the movement, the lifter typed the numbers, the server uses
      // them verbatim, and kind is assistance by construction. restSeconds and
      // equipment are the two fields that had to be put on the library for this
      // — without the first the countdown would start three minutes on a set of
      // curls, and without the second the card would put an empty bar and a
      // plate diagram in front of a pair of dumbbells until the server replied.
      () =>
        tempSetIds.map((id, i) => ({
          id,
          exerciseId: exercise.id,
          exerciseName: exercise.name,
          kind: "assistance" as const,
          setNumber: i + 1,
          targetReps: choice.reps,
          actualReps: null,
          weightLb: choice.weightLb,
          completed: false,
          // Never a bonus set. Only appending to a lift already in the session
          // can produce one; adding a whole lift is a different gesture, and
          // the server agrees — addSessionAssistance does not set the flag.
          isBonus: false,
          restSeconds: exercise.restSeconds,
          equipment: exercise.equipment,
        })),
    );
    if (!outcome.ok) {
      actionError = "Couldn't add that lift.";
      return false;
    }
    actionError = null;
    if (!session) return false;
    // Appended: assistance goes after the program's own work, and the server
    // orders it that way on the next read.
    session.sets = [...session.sets, ...outcome.value];
    pickerOpen = false;
    return true;
  }

  // Drop a set that wasn't performed. Removing a lift's last set takes the lift
  // out of the session, which is what skipping it looks like.
  async function removeSet(set: SessionSet) {
    if (isOver) return;
    // void, because a delete answers 204 with no body. Only `ok` is read here.
    const outcome = await write<void>(
      { kind: "removeSet", sessionId, setId: set.id },
      () => removeSessionSet(sessionId, set.id),
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
    class="inline-flex items-center gap-1.5 self-start text-sm text-muted-foreground transition hover:text-neon-lift"
  >
    <ArrowLeft class="size-4" aria-hidden="true" />
    Workout
  </a>

  {#if prNote}
    <!-- Quieter for a first time, all the way down: a muted border and a sprout
         instead of the primary wash and the trophy. It is a start, not a win, and
         a first workout showing six of these in full voice is exactly what made
         the first real record read as more of the same. -->
    <div
      class="rounded-2xl border p-3 text-center {prNote.kind === 'pr'
        ? 'border-primary/60 bg-primary/15'
        : 'border-border/60 bg-white/5'}"
      role="status"
    >
      <p
        class="flex items-center justify-center gap-2 font-black {prNote.kind === 'pr'
          ? 'text-primary'
          : 'text-muted-foreground'}"
      >
        {#if prNote.kind === "pr"}
          <Trophy class="size-5" aria-hidden="true" />
          New PR! {prNote.text}
        {:else}
          <Sprout class="size-5" aria-hidden="true" />
          First time! {prNote.text}
        {/if}
      </p>
    </div>
  {/if}

  {#if loading}
    <!-- The session header, the weigh-in box and a card per lift. Sized against
         ExerciseCard rather than against a round number: this screen is opened
         standing at a rack, and a layout that settles a beat after the first tap
         is how a rep gets logged against the wrong set. -->
    <Loading label="Loading this workout" class="flex flex-col gap-6">
      <div class="flex items-start justify-between gap-3">
        <div>
          <Skeleton text="2xl" class="w-52" />
          <Skeleton text="sm" class="mt-1 w-64" />
          <Skeleton text="xs" class="mt-1 w-32" />
        </div>
        <Skeleton class="h-8 w-36 shrink-0 rounded-md" />
      </div>
      <Card class="p-5">
        <Skeleton text="lg" class="w-28" />
        <Skeleton class="mt-3 h-11 w-40 rounded-md" />
      </Card>
      {#each SKELETON_LIFTS as lift (lift)}
        <Card class="p-5">
          <div class="flex items-center justify-between gap-3">
            <Skeleton text="lg" class="w-40" />
            <Skeleton text="sm" class="w-16" />
          </div>
          <div class="mt-4 flex flex-wrap items-center gap-x-5 gap-y-3">
            <Skeleton class="h-12 w-12 rounded-full" />
            <Skeleton class="h-12 w-12 rounded-full" />
            <Skeleton class="h-12 w-12 rounded-full" />
          </div>
        </Card>
      {/each}
    </Loading>
  {:else if failed}
    <ErrorCard message="Couldn't load this session." onRetry={load} />
  {:else if session}
    <div class="flex items-start justify-between gap-3">
      <header>
        <h2 class="text-2xl font-black text-foreground">
          {session.programName}
        </h2>
        <p class="mt-1 text-sm text-muted-foreground">
          {session.programDayName} · {formatLongDate(session.performedOn)}
        </p>
        <p class="mt-1 text-xs uppercase tracking-[0.3em] text-primary">
          {loggedCount} / {session.sets.length} sets logged
        </p>
        {#if isOver}
          <p
            class="mt-2 text-xs uppercase tracking-[0.3em] text-muted-foreground"
          >
            {#if session.finishedAt}
              Finished · <Timestamp value={session.finishedAt} />
            {:else}
              Closed automatically · 12h+ old
            {/if}
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
      {:else}
        <!-- An over session is a record to read, and this screen is the set-by-
             set version of it. The recap is the same workout told as a story,
             so it sits where Finish used to — the one action a closed session
             still has. -->
        <Button
          size="sm"
          variant="outline"
          onclick={() => push(`/sessions/${sessionId}/recap`)}
        >
          <Trophy />
          Recap
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
        equipment={group.sets[0].equipment}
        assistance={group.assistance}
        onCycle={cycle}
        onChangeWeight={(delta) => changeWeight(group.sets, delta)}
        onAddSet={() => addSet(group.sets[0].exerciseId)}
        onRemoveSet={removeSet}
        nextRung={rungFor(group.sets[0].exerciseId)}
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

    <!-- Extra work, decided on at the rack. Below the cards because that is
         where it goes in the workout, and out of the way of the sets being
         tapped through.

         Unlike everything above it, this reaches past the session: the lift is
         added to the program day too, so it comes round again with a
         progression behind it. -->
    {#if !isOver}
      {#if pickerOpen}
        <AssistancePicker
          exclude={alreadyHere}
          onAdd={addAssistanceLift}
          onCancel={() => (pickerOpen = false)}
          confirmLabel="Add to this workout"
          footnote="It joins {session.programDayName} too, so it's prescribed next time."
        />
      {:else}
        <Button variant="outline" size="sm" onclick={() => (pickerOpen = true)}>
          <Plus />
          Add assistance
        </Button>
      {/if}
    {/if}

    <!-- Finishing used to open a second dialog here, carrying a sets count and
         a volume. It is a whole screen now — /sessions/:id/recap, which
         finish() navigates to. -->
  {/if}
</div>
