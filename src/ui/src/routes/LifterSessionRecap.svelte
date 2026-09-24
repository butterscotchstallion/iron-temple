<script lang="ts">
  import { onMount } from "svelte";
  import { Card } from "$lib/components/ui/card";
  import ErrorCard from "../lib/ErrorCard.svelte";
  import Loading from "../lib/skeleton/Loading.svelte";
  import SkeletonRecap from "../lib/skeleton/SkeletonRecap.svelte";
  import RecapHeroTiles from "../lib/RecapHeroTiles.svelte";
  import RecapComparisons from "../lib/RecapComparisons.svelte";
  import RecapHighlights from "../lib/RecapHighlights.svelte";
  import RecapLiftTable from "../lib/RecapLiftTable.svelte";
  import MuscleVolumeBars from "../lib/MuscleVolumeBars.svelte";
  import SessionSocial from "../lib/SessionSocial.svelte";
  import LifterName from "../lib/LifterName.svelte";
  import { markedCommentId } from "../lib/commentAnchor";
  import { formatLongDate } from "../lib/date";
  import { formatVolume } from "../lib/volume";
  import type { RecapLiftRow, RecapPRRow } from "../lib/recap";
  import {
    getLifter,
    getLifterSessionRecap,
    type LifterProfile,
    type SessionRecap,
  } from "../lib/api";

  // Somebody else's session, in review.
  //
  // A separate route from SessionRecap rather than a mode of it, and the reason
  // is that almost nothing in that file applies here. It tries three sources in
  // order because it is the screen you land on at the rack with no signal: the
  // session handed across by finish(), the cache, then the network. It fires
  // confetti. It re-fetches when the write queue drains. It offers a share card
  // built from the reader's own name. None of that is true of a workout somebody
  // else did last Tuesday, which is one fact, already final, fetched over a
  // network you evidently have.
  //
  // So this reuses the PRESENTATION — the same five components, fed the same view
  // model — and none of the machinery. What it must not do is compute any of the
  // figures itself: they arrive from the same endpoint, from the same code, that
  // produced the owner's own recap.
  //
  // One section is deliberately dropped: "what you earned", the weights the next
  // session of this day will prescribe. It is a forecast for the lifter whose
  // history it was computed from, and on a page about somebody else it invites
  // being read as your own.
  let { params }: { params: { lifterId: string; sessionId: string } } = $props();

  const lifterId = $derived(Number(params.lifterId));
  const sessionId = $derived(Number(params.sessionId));

  // Which comment a notification sent this lifter here to read, if any.
  const markedComment = $derived(markedCommentId());

  let recap = $state<SessionRecap | null>(null);
  // The lifter themselves rather than their name, so the heading can draw
  // whatever they are currently wearing beside it. A failure here still costs
  // only the name.
  let lifter = $state<LifterProfile | null>(null);
  let loading = $state(true);
  let failed = $state(false);
  let missing = $state(false);

  async function load() {
    failed = false;
    missing = false;
    loading = true;

    const result = await getLifterSessionRecap(lifterId, sessionId);
    // 404 covers both "no such session" and "not that lifter's session" — the
    // endpoint deliberately does not distinguish them, so neither does this.
    if (result.status === 404) {
      missing = true;
      loading = false;
      return;
    }
    if (result.status !== 200) {
      failed = true;
      loading = false;
      return;
    }
    recap = result.data;

    // Whose it was, for the heading. A second request, and a cheap one — the
    // recap describes a session and carries no lifter. A failure here costs the
    // name and not the page.
    const who = await getLifter(lifterId);
    if (who.status === 200) {
      lifter = who.data;
    }
    loading = false;
  }

  // The same view models SessionRecap builds, minus its offline fallbacks: there
  // is no local source here, so every field comes from the response or the page
  // is not drawn at all.
  const tiles = $derived(
    recap
      ? {
          durationSeconds: recap.durationSeconds,
          volumeLb: recap.volume.totalLb,
          setsLogged: recap.volume.setsLogged,
          setsPrescribed: recap.volume.setsPrescribed,
          repsLogged: recap.volume.repsLogged,
          repsTargeted: recap.volume.repsTargeted,
          setsBonus: recap.volume.setsBonus,
        }
      : null,
  );

  const lifts = $derived<RecapLiftRow[]>(
    (recap?.lifts ?? []).map((l) => ({
      exerciseId: l.exerciseId,
      exerciseName: l.exerciseName,
      kind: l.kind,
      topWeightLb: l.topWeightLb,
      topReps: l.topReps,
      setsLogged: l.setsLogged,
      setsPrescribed: l.setsPrescribed,
      repsLogged: l.repsLogged,
      repsTargeted: l.repsTargeted,
      setsBonus: l.setsBonus,
      hitEveryTarget: l.hitEveryTarget,
      previousTopWeightLb: l.previous?.topWeightLb ?? null,
      weightDeltaPct: l.weightDeltaPct,
    })),
  );

  const prs = $derived<RecapPRRow[]>(
    (recap?.prs ?? []).map((p) => ({
      exerciseName: p.exerciseName,
      kind: p.kind,
      valueLb: p.valueLb,
      previousLb: p.previousLb,
    })),
  );

  onMount(load);
</script>

{#if missing}
  <Card class="p-8 text-center">
    <h2 class="text-lg font-bold text-card-foreground">No such session</h2>
    <p class="mt-2 text-sm text-muted-foreground">
      That session either doesn't exist or isn't this lifter's.
    </p>
  </Card>
{:else if failed}
  <ErrorCard message="Couldn't load this session." onRetry={load} />
{:else if loading || !recap || !tiles}
  <Loading label="Loading this session" class="flex flex-col gap-4">
    <SkeletonRecap centered />
  </Loading>
{:else}
  <div class="flex flex-col gap-4">
    <div class="text-center">
      <h2 class="text-2xl font-black text-foreground">
        {recap.session.programDayName}
      </h2>
      <p class="mt-1 text-sm text-muted-foreground">
        {#if lifter}<LifterName {lifter} crownSize="size-3" /> ·
        {/if}{recap.session.programName} · {formatLongDate(recap.session.performedOn)}
      </p>
    </div>

    <RecapHeroTiles {...tiles} />

    <RecapComparisons
      pace={recap.pace}
      progress={recap.progress}
      dayName={recap.session.programDayName}
    />

    <RecapHighlights
      {prs}
      milestones={recap.milestones}
      streakSessions={recap.streak.sessions}
      streakWeeks={recap.streak.weeks}
    />

    {#if recap.volume.comparison.count > 0}
      <p class="text-center text-sm text-muted-foreground">
        {formatVolume(recap.volume.totalLb)} lb — about
        {recap.volume.comparison.count.toLocaleString("en-US")}
        {recap.volume.comparison.label}.
      </p>
    {/if}

    <RecapLiftTable {lifts} />

    {#if recap.muscles.length > 0}
      <Card class="p-4">
        <h3 class="text-xs uppercase tracking-[0.2em] text-muted-foreground">
          Where it went
        </h3>
        <div class="mt-2">
          <MuscleVolumeBars rows={recap.muscles} />
        </div>
      </Card>
    {/if}

    <!-- Applaud it and say something. ownerId is the lifter in the URL, which is
         somebody else by construction on this route — so the buttons are live
         here, where on your own recap they stand down. -->
    <SessionSocial
      sessionId={recap.session.sessionId}
      ownerId={lifterId}
      highlightCommentId={markedComment}
    />
  </div>
{/if}
