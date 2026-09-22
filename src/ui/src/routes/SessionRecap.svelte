<script lang="ts">
  import { onMount } from "svelte";
  import { push } from "svelte-spa-router";
  import { getSessionRecap } from "../lib/api";
  import type { Session, SessionRecap } from "../lib/api";
  import { cachedValue, fetchThrough, sessionRecapKey } from "../lib/cache.svelte";
  import { observe } from "../lib/connectivity.svelte";
  import { onDrained } from "../lib/writeQueue.svelte";
  import { auth } from "../lib/auth.svelte";
  import { Card } from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import PartyPopper from "@lucide/svelte/icons/party-popper";
  import Dumbbell from "@lucide/svelte/icons/dumbbell";
  import Share2 from "@lucide/svelte/icons/share-2";
  import CloudOff from "@lucide/svelte/icons/cloud-off";
  import ErrorCard from "../lib/ErrorCard.svelte";
  import ShareCardDialog from "../lib/ShareCardDialog.svelte";
  import RecapHeroTiles from "../lib/RecapHeroTiles.svelte";
  import RecapComparisons from "../lib/RecapComparisons.svelte";
  import RecapHighlights from "../lib/RecapHighlights.svelte";
  import RecapLiftTable from "../lib/RecapLiftTable.svelte";
  import RecapEarned from "../lib/RecapEarned.svelte";
  import MuscleVolumeBars from "../lib/MuscleVolumeBars.svelte";
  import SessionSocial from "../lib/SessionSocial.svelte";
  import { formatPercent, formatWeighIn } from "../lib/racked";
  import { takeHandedSession } from "../lib/recapHandoff";
  import { localRecap } from "../lib/localRecap";
  import type { RecapLiftRow, RecapPRRow } from "../lib/recap";
  import { sessionShareCardContent } from "../lib/sessionShareCard";
  import { sessionShareCardFilename } from "../lib/shareImage";
  import { formatLongDate } from "../lib/date";
  import { formatVolume } from "../lib/volume";
  import { celebrate } from "../lib/celebrate";

  // What the workout was worth.
  //
  // Reached by finishing a session, and afterwards by its own URL — a recap is
  // a fact about a workout that has already happened, so it is linkable and
  // does not change.
  //
  // Three sources, tried in order, and the order is the whole offline design:
  //
  //   1. The session handed across by finish(), rendered through localRecap.
  //      Instant, needs no network, and is the case at the rack.
  //   2. The cached server recap, for a reload or a later visit.
  //   3. The network.
  //
  // A failed request with something already on screen leaves it there and says
  // so quietly. Only a cold open with no signal — a recap opened from history
  // in a basement — gets an error card.

  let { params }: { params?: { id?: string } } = $props();
  const sessionId = $derived(Number(params?.id ?? 0));

  let recap = $state<SessionRecap | null>(null);
  let local = $state<ReturnType<typeof localRecap> | null>(null);
  let loading = $state(true);
  let failed = $state(false);
  let degraded = $state(false);
  let sharing = $state(false);

  async function load() {
    failed = false;

    const handed: Session | null = takeHandedSession(sessionId);
    if (handed) {
      local = localRecap(handed);
      // Confetti here rather than in finish(), so it lands over the recap
      // instead of behind a screen the lifter is being navigated away from.
      if (handed.sets.length > 0 && handed.sets.every((s) => s.completed)) {
        celebrate({ particleCount: 140, spread: 75, origin: { y: 0.6 } });
      }
    }

    const remembered = cachedValue<SessionRecap>(sessionRecapKey(sessionId));
    if (remembered) recap = remembered;

    loading = local === null && recap === null;

    const result = await fetchThrough(sessionRecapKey(sessionId), () =>
      getSessionRecap(sessionId),
    );
    // A read that never lands is the clearest evidence the app has that it is
    // offline, and this is often the first request made after a finish.
    observe(result);

    if (result.status === 200) {
      recap = result.data;
      degraded = false;
    } else if (recap === null && local === null) {
      failed = true;
    } else {
      // Keep what is on screen. A failed refresh must not evict a recap the
      // lifter is reading.
      degraded = recap === null;
    }
    loading = false;
  }

  onMount(load);

  // When the queue drains, ask again.
  //
  // Without this the degraded state is a dead end: it promises the rest will
  // "fill in next time you're on signal" and then never looks. The recap is
  // reached by finishing, which is also the write most likely to have been
  // queued — so walking out of the gym and back onto signal is the single most
  // likely thing to happen while this page is open. ActiveSession does the same
  // on the same signal.
  $effect(() => {
    onDrained(() => void load());
    return () => onDrained(null);
  });

  // ---- one view model, from whichever source answered ----

  const header = $derived(
    recap
      ? {
          programName: recap.session.programName,
          dayName: recap.session.programDayName,
          performedOn: recap.session.performedOn,
        }
      : null,
  );

  const allComplete = $derived(
    recap
      ? recap.volume.setsLogged === recap.volume.setsPrescribed &&
          recap.lifts.every((l) => l.hitEveryTarget)
      : (local?.setsLogged === local?.setsPrescribed && local?.lifts.every((l) => l.hitEveryTarget)) ??
          false,
  );

  const tiles = $derived(
    recap
      ? {
          durationSeconds: recap.durationSeconds,
          volumeLb: recap.volume.totalLb,
          setsLogged: recap.volume.setsLogged,
          setsPrescribed: recap.volume.setsPrescribed,
          repsLogged: recap.volume.repsLogged,
          repsTargeted: recap.volume.repsTargeted,
        }
      : local && {
          durationSeconds: local.durationSeconds,
          volumeLb: local.volumeLb,
          setsLogged: local.setsLogged,
          setsPrescribed: local.setsPrescribed,
          repsLogged: local.repsLogged,
          repsTargeted: local.repsTargeted,
        },
  );

  const lifts = $derived<RecapLiftRow[]>(
    recap
      ? recap.lifts.map((l) => ({
          exerciseId: l.exerciseId,
          exerciseName: l.exerciseName,
          kind: l.kind,
          topWeightLb: l.topWeightLb,
          topReps: l.topReps,
          setsLogged: l.setsLogged,
          setsPrescribed: l.setsPrescribed,
          repsLogged: l.repsLogged,
          repsTargeted: l.repsTargeted,
          hitEveryTarget: l.hitEveryTarget,
          previousTopWeightLb: l.previous?.topWeightLb ?? null,
          weightDeltaPct: l.weightDeltaPct,
        }))
      : (local?.lifts ?? []).map((l) => ({
          ...l,
          // Offline there is no previous session to compare against, and an
          // absent comparison is what the table draws as "new to this day".
          // That is a small lie the degraded banner above it accounts for.
          previousTopWeightLb: null,
          weightDeltaPct: null,
        })),
  );

  const prs = $derived<RecapPRRow[]>(
    recap
      ? recap.prs.map((p) => ({
          exerciseName: p.exerciseName,
          kind: p.kind,
          valueLb: p.valueLb,
          previousLb: p.previousLb,
        }))
      : (local?.prs ?? []).map((p) => ({
          exerciseName: p.exerciseName,
          kind: p.kind,
          valueLb: p.valueLb,
          previousLb: p.previousLb,
        })),
  );

  const shareContent = $derived(
    recap ? sessionShareCardContent(recap, auth.me?.displayName ?? "") : null,
  );
</script>

<div class="mx-auto flex w-full max-w-3xl flex-col gap-4 p-4">
  {#if loading}
    <Card class="h-24 animate-pulse" aria-hidden="true"></Card>
    <Card class="h-28 animate-pulse" aria-hidden="true"></Card>
    <Card class="h-56 animate-pulse" aria-hidden="true"></Card>
  {:else if failed || !tiles}
    <ErrorCard message="Couldn't load the recap." onRetry={load} />
  {:else}
    <header class="flex flex-col gap-1">
      <h2 class="flex items-center gap-2 font-display text-2xl font-black text-foreground">
        {#if allComplete}
          <PartyPopper class="size-6 shrink-0 text-primary" aria-hidden="true" />
          Workout complete
        {:else}
          <Dumbbell class="size-6 shrink-0" aria-hidden="true" />
          Workout finished
        {/if}
      </h2>
      {#if header}
        <p class="text-sm text-muted-foreground">
          {header.programName} · {header.dayName} · {formatLongDate(header.performedOn)}
        </p>
      {/if}
    </header>

    {#if degraded}
      <!-- Deliberately not an ErrorCard: nothing failed that the lifter did.
           The numbers on screen are real, and the rest arrives on signal. -->
      <p
        class="flex items-center gap-2 text-xs text-muted-foreground"
        role="status"
        data-testid="recap-degraded"
      >
        <CloudOff class="size-4 shrink-0" aria-hidden="true" />
        Records and comparisons need the server — they'll fill in next time you're on signal.
      </p>
    {/if}

    <RecapHeroTiles {...tiles} />

    {#if recap}
      <RecapComparisons
        pace={recap.pace}
        progress={recap.progress}
        dayName={recap.session.programDayName}
      />
    {/if}

    <RecapHighlights
      {prs}
      milestones={recap?.milestones ?? []}
      streakSessions={recap?.streak.sessions ?? 0}
      streakWeeks={recap?.streak.weeks ?? 0}
    />

    {#if recap && recap.volume.comparison.count > 0}
      <p class="text-center text-sm text-muted-foreground" data-testid="recap-comparison">
        {formatVolume(recap.volume.totalLb)} lb — about
        {recap.volume.comparison.count.toLocaleString("en-US")}
        {recap.volume.comparison.label}.
      </p>
    {/if}

    <RecapLiftTable {lifts} />

    {#if recap && recap.muscles.length > 0}
      <Card class="p-4" data-testid="recap-muscles">
        <h3 class="text-xs uppercase tracking-[0.2em] text-muted-foreground">Where it went</h3>
        <div class="mt-2">
          <MuscleVolumeBars rows={recap.muscles} />
        </div>
        <!-- Only worth saying when the lifter actually bolted something on: on
             a session that was purely the program, "100% prescribed" is a
             sentence that tells them what they already know. -->
        {#if recap.split.assistance.volumeLb > 0}
          <p class="mt-2 text-xs tabular-nums text-muted-foreground">
            {formatPercent(recap.split.main.share)} prescribed ·
            {formatPercent(recap.split.assistance.share)} assistance
          </p>
        {/if}
      </Card>
    {/if}

    {#if recap?.earned}
      <RecapEarned earned={recap.earned} />
    {/if}

    {#if recap?.bodyweightLb}
      <p class="text-center text-xs tabular-nums text-muted-foreground">
        You weighed {formatWeighIn(recap.bodyweightLb)} lb
      </p>
    {/if}

    <!-- Who applauded, and what they said.
         Gated on `recap` rather than shown unconditionally, which is the whole
         point: when this page is rendering from `local` alone it is the screen a
         lifter is standing in front of at a rack with no signal, and a comment box
         that cannot reach the server is furniture. `recap` is non-null only once
         the server has answered, now or into the cache.
         ownerId is the reader: this route only ever shows their own session, so
         the reaction buttons stand down and the counts show instead. -->
    {#if recap}
      <SessionSocial sessionId={recap.session.sessionId} ownerId={auth.me?.id ?? null} />
    {/if}

    <div class="flex flex-wrap gap-2">
      {#if shareContent}
        <Button variant="outline" onclick={() => (sharing = true)}>
          <Share2 class="size-4" aria-hidden="true" />
          Share
        </Button>
      {/if}
      <Button variant="outline" onclick={() => push("/history")}>See history</Button>
      <Button onclick={() => push("/")}>Done</Button>
    </div>

    {#if shareContent && recap}
      <ShareCardDialog
        bind:open={sharing}
        content={shareContent}
        filename={sessionShareCardFilename(
          recap.session.programDayName,
          recap.session.performedOn,
        )}
        alt="{recap.session.programDayName}, {recap.session
          .performedOn}: {formatVolume(recap.volume.totalLb)} lb lifted"
        title="Share this workout"
        subtitle="{recap.session.programDayName} as an image."
      />
    {/if}
  {/if}
</div>
