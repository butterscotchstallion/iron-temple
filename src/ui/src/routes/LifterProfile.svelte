<script lang="ts">
  import { onMount } from "svelte";
  import { link } from "svelte-spa-router";
  import { Card } from "$lib/components/ui/card";
  import Avatar from "../lib/Avatar.svelte";
  import LifterName from "../lib/LifterName.svelte";
  import FollowButton from "../lib/FollowButton.svelte";
  import AchievementList from "../lib/AchievementList.svelte";
  import CalendarHeatmap from "../lib/CalendarHeatmap.svelte";
  import ErrorCard from "../lib/ErrorCard.svelte";
  import Loading from "../lib/skeleton/Loading.svelte";
  import Skeleton from "../lib/skeleton/Skeleton.svelte";
  import SkeletonTiles from "../lib/skeleton/SkeletonTiles.svelte";
  import LiftVolumeBars from "../lib/LiftVolumeBars.svelte";
  import MuscleVolumeBars from "../lib/MuscleVolumeBars.svelte";
  import Timestamp from "../lib/Timestamp.svelte";
  import { auth } from "../lib/auth.svelte";
  import { muscleGroupLabel } from "../lib/library";
  import { formatVolume } from "../lib/volume";
  import SessionCards from "../lib/SessionCards.svelte";
  import { Button, buttonVariants } from "$lib/components/ui/button";
  import {
    getLifter,
    getLifterAchievements,
    getLifterRacked,
    listLifterSessions,
    type LifterAchievement,
    type LifterProfile,
    type RackedReport,
    type SessionSummary,
  } from "../lib/api";

  // One lifter, as seen by another.
  //
  // Two requests, and the division between them is the design rather than an
  // accident. The profile call carries who they are and the two lifetime figures;
  // everything with a statistic in it comes from their Racked report, which is
  // the same report they read on their own page, computed by the same code. This
  // screen deliberately calculates nothing — a streak or a tonnage worked out
  // here would be a second opinion about one history, and the first time it
  // disagreed with theirs the app would have no way to say which was right.
  let { params }: { params: { id: string } } = $props();

  let profile = $state<LifterProfile | null>(null);
  let report = $state<RackedReport | null>(null);
  // What they have earned, current and past. The crown beside their name comes
  // from the global achievements module; this is the history behind it, which
  // only the per-lifter endpoint knows.
  let achievements = $state<LifterAchievement[]>([]);
  let loading = $state(true);
  let failed = $state(false);

  // Their training, which is what turns this page from a summary into somewhere
  // to go. Every row links to that session's recap — the screen where one
  // lifter applauds another — which until now was reachable only through the
  // feed, where finding a particular lifter's Tuesday meant paging past
  // everybody else's week.
  const SESSION_PAGE = 10;
  let sessions = $state<SessionSummary[]>([]);
  let sessionTotal = $state(0);
  let loadingMore = $state(false);
  const hasMore = $derived(sessions.length < sessionTotal);
  // Tracked apart from `failed`: a 404 is a different answer from a network
  // problem, and "no such lifter" is not something a retry button can fix.
  let missing = $state(false);

  const id = $derived(Number(params.id));

  async function load() {
    failed = false;
    missing = false;
    loading = true;

    const who = await getLifter(id);
    if (who.status === 404) {
      missing = true;
      loading = false;
      return;
    }
    if (who.status !== 200) {
      failed = true;
      loading = false;
      return;
    }
    profile = who.data;

    // The month in progress, which is what /racked opens on too. Fetched after
    // the profile rather than alongside it so that a lifter who does not exist
    // costs one request and not two.
    const stats = await getLifterRacked(id, { period: "month" });
    // A failure here is not a failed page. The identity and the lifetime totals
    // have already arrived and are worth showing; the statistics section simply
    // stands down, which is the same call the recap makes when it cannot reach
    // the server.
    if (stats.status === 200) {
      report = stats.data;
    }

    // Their history, on the same terms as the statistics above: a failure here
    // stands the section down rather than failing the page, because the
    // identity and the lifetime totals have already arrived and are worth
    // showing.
    const history = await listLifterSessions(id, { limit: SESSION_PAGE });
    if (history.status === 200) {
      sessions = history.data.items;
      sessionTotal = history.data.total;
    }

    // And what they have won, on exactly the same terms: a failure here leaves
    // the section empty rather than failing a page that has already arrived.
    const earned = await getLifterAchievements(id);
    if (earned.status === 200) {
      achievements = earned.data.items;
    }
    loading = false;
  }

  async function loadMore() {
    if (loadingMore || !hasMore) return;
    loadingMore = true;
    const result = await listLifterSessions(id, {
      limit: SESSION_PAGE,
      offset: sessions.length,
    });
    loadingMore = false;
    if (result.status !== 200) return;
    sessions = [...sessions, ...result.data.items];
    sessionTotal = result.data.total;
  }

  const name = $derived(profile?.displayName || profile?.username || "");
  // Your own profile, which the account menu now links to directly.
  //
  // This page is the SAME page whoever is reading it — one lifter's history
  // drawn once, by the component that draws everybody's. What changes is the
  // person the prose is written in: "Their training" and "Grace Hopper hasn't
  // logged anything this month" are the right sentences about somebody else and
  // an odd way to address the lifter who lived them. The figures, the charts and
  // the requests behind them are untouched.
  const isMe = $derived(profile !== null && profile.id === auth.me?.id);
  const totals = $derived(report?.totals ?? null);
  const muscleRows = $derived(report?.muscles ?? []);
  const untrainedMuscles = $derived(
    muscleRows.filter((m) => !m.trained).map((m) => muscleGroupLabel(m.group)),
  );
  // No trend colours to key off here — this page draws no per-lift chart, so the
  // bars carry their own default rather than a palette matched to a legend that
  // does not exist.
  const liftRows = $derived(
    (report?.lifts ?? []).map((l) => ({
      exerciseId: l.exerciseId,
      exerciseName: l.exerciseName,
      volumeLb: l.volumeLb,
      sets: l.sets,
      share: l.share,
      color: null,
      isAssistance: l.isAssistance,
    })),
  );
  const volumesByDate = $derived(
    Object.fromEntries((report?.days ?? []).map((d) => [d.date, d.volumeLb])),
  );
  const heatmapSessions = $derived(
    (report?.days ?? []).map((d) => ({ performedOn: d.date, day: "" })),
  );
  // A month needs six columns to be sure of covering it — the same number
  // Racked uses for its monthly view.
  const heatmapWeeks = 6;

  onMount(load);
</script>

{#if missing}
  <!-- Not an error card: there is nothing to retry. An id that names nobody is
       an answer, and a button offering to ask again would be a lie. -->
  <Card class="p-8 text-center">
    <h2 class="text-lg font-bold text-card-foreground">No such lifter</h2>
    <p class="mt-2 text-sm text-muted-foreground">
      There's no account here with that id.
    </p>
  </Card>
{:else if failed}
  <ErrorCard message="Couldn't load this lifter." onRetry={load} />
{:else if loading || !profile}
  <!-- The identity card and the two lifetime figures, which are the part of
       this page that always draws. The month's statistics below them are
       skipped: they come from a second request that is allowed to fail on its
       own, and a lifter who has logged nothing this month has no section there
       to hold space for. -->
  <Loading label="Loading this lifter" class="flex flex-col gap-4">
    <Card class="flex items-center gap-4 p-6">
      <Skeleton class="size-14 shrink-0 rounded-full" />
      <div class="min-w-0 flex-1">
        <Skeleton text="2xl" class="w-44" />
        <Skeleton text="sm" class="mt-1 w-60" />
      </div>
    </Card>
    <SkeletonTiles count={2} value="xl" class="grid-cols-2 sm:grid-cols-2" />
  </Loading>
{:else}
  <div class="flex flex-col gap-4">
    <!-- Who -->
    <Card class="flex items-center gap-4 p-6">
      <Avatar user={profile} size={56} />
      <div class="min-w-0">
        <h2 class="flex min-w-0 text-2xl font-black text-foreground">
          <LifterName lifter={profile} crownSize="size-5" />
        </h2>
        <p class="truncate text-sm text-muted-foreground">
          {profile.username}
          {#if profile.lastTrainedOn}
            · last trained <Timestamp
              value={profile.lastTrainedOn}
              kind="date"
            />
          {:else if isMe}
            · you haven't trained yet
          {:else}
            · hasn't trained yet
          {/if}
        </p>
      </div>
      <!-- ml-auto rather than a restructure: the Card is already a flex row, so
           whatever goes here lands right-aligned beside the identity.
           Never a Follow control on your own profile, as on your own roster row.

           What takes its place is the way back to the settings screen. This is
           the page where you notice the display name is stale or the avatar is
           still initials, and until now the form for that was somewhere else
           entirely. An anchor styled as a button rather than a button with a
           push(): it is a navigation, so it should be middle-clickable and show
           its target in the status bar. Same idiom as the Resume link on
           ProgramDetail. -->
      {#if isMe}
        <a
          use:link
          href="/profile"
          class="ml-auto shrink-0 {buttonVariants({ variant: 'outline', size: 'sm' })}"
        >
          Configure profile
        </a>
      {:else}
        <div class="ml-auto">
          <FollowButton
            lifter={profile}
            following={profile.following === true}
            onChange={(next) => {
              if (profile) profile.following = next;
            }}
          />
        </div>
      {/if}
    </Card>

    <!-- Lifetime. Two figures, straight off the profile — the only numbers on
         this page that are not the month's. -->
    <div class="grid grid-cols-2 gap-3">
      <Card class="p-4 text-center">
        <p class="text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground">
          Lifetime volume
        </p>
        <p class="mt-1 text-xl font-black text-foreground">
          {formatVolume(profile.lifetimeVolumeLb)}
        </p>
      </Card>
      <Card class="p-4 text-center">
        <p class="text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground">
          Sessions
        </p>
        <p class="mt-1 text-xl font-black text-foreground">{profile.sessionCount}</p>
      </Card>
    </div>

    <!-- What they have won. Above the month's statistics because it is about
         the lifter rather than about a window — the same reason the lifetime
         figures are up here and not down there. Drawn even when empty, so the
         section does not appear and disappear as crowns change hands. -->
    <Card class="p-6">
      <h3 class="text-lg font-bold text-card-foreground">Achievements</h3>
      <div class="mt-2">
        <!-- `you` only reaches the empty state, which is the one line here that
             has to name somebody. `upcoming` and `onShare` stay unpassed even on
             your own profile: what you are closing in on, and the button that
             turns a crown into a share card, belong to the achievements section
             of the settings screen — this page is the public view of the same
             history, not a second copy of that one. -->
        <AchievementList items={achievements} {name} you={isMe} />
      </div>
    </Card>

    {#if report && totals && totals.sessions > 0}
      <div>
        <h3 class="text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground">
          {report.period.label}
        </h3>
      </div>

      <div class="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <Card class="p-4 text-center">
          <p class="text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground">
            Volume
          </p>
          <p class="mt-1 text-lg font-black text-foreground">
            {formatVolume(totals.volumeLb)}
          </p>
        </Card>
        <Card class="p-4 text-center">
          <p class="text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground">
            Sessions
          </p>
          <p class="mt-1 text-lg font-black text-foreground">{totals.sessions}</p>
        </Card>
        <Card class="p-4 text-center">
          <p class="text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground">
            Sets
          </p>
          <p class="mt-1 text-lg font-black text-foreground">{totals.sets}</p>
        </Card>
        <Card class="p-4 text-center">
          <p class="text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground">
            Week streak
          </p>
          <p class="mt-1 text-lg font-black text-foreground">
            {report.streak.currentWeeks}
          </p>
        </Card>
      </div>

      {#if liftRows.length > 0}
        <Card class="p-4" data-testid="lifter-lifts">
          <h3
            class="mb-3 text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground"
          >
            Where the weight went
          </h3>
          <LiftVolumeBars rows={liftRows} />
        </Card>
      {/if}

      {#if muscleRows.length > 0}
        <Card class="p-4" data-testid="lifter-muscles">
          <h3
            class="mb-3 text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground"
          >
            {isMe ? "What you trained" : "What they trained"}
          </h3>
          <MuscleVolumeBars rows={muscleRows} />
          {#if untrainedMuscles.length > 0}
            <p class="mt-3 text-xs text-muted-foreground">
              Nothing logged for {untrainedMuscles
                .map((m) => m.toLowerCase())
                .join(", ")}.
            </p>
          {/if}
        </Card>
      {/if}

      <Card class="p-4" data-testid="lifter-heatmap">
        <h3 class="mb-2 text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground">
          Every training day
        </h3>
        <CalendarHeatmap
          sessions={heatmapSessions}
          volumes={volumesByDate}
          endDate={report.period.end}
          weeks={heatmapWeeks}
          scheduledWeekdays={report.attendance.weekdays}
        />
      </Card>
    {:else if report}
      <!-- The report arrived and holds nothing. Said plainly rather than drawn as
           a row of zeroes, which reads as a broken page. -->
      <Card class="p-6 text-center">
        <p class="text-sm text-muted-foreground">
          <!-- Branched rather than interpolated: "You hasn't logged anything"
               is what passing "You" as the name would produce. -->
          {#if isMe}
            You haven't logged anything this month.
          {:else}
            {name} hasn't logged anything this month.
          {/if}
        </p>
      </Card>
    {/if}

    <!-- Their sessions. Below the month's statistics because those answer "how
         are they training", and this answers "what did they do" — the question
         you follow into a recap. Absent entirely for an account that has never
         logged a rep: the two cards above already say so. -->
    {#if sessions.length > 0}
      <div data-testid="lifter-sessions" class="flex flex-col gap-3">
        <h3 class="text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground">
          {isMe ? "Your training" : "Their training"}
        </h3>
        <SessionCards {sessions} lifterId={id} />

        {#if hasMore}
          <div class="flex justify-center">
            <Button variant="outline" onclick={loadMore} disabled={loadingMore}>
              {loadingMore ? "Loading…" : "Load more"}
            </Button>
          </div>
        {/if}
      </div>
    {/if}
  </div>
{/if}
