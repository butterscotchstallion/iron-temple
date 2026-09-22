<script lang="ts">
  import { onMount } from "svelte";
  import { Card } from "$lib/components/ui/card";
  import Avatar from "../lib/Avatar.svelte";
  import CalendarHeatmap from "../lib/CalendarHeatmap.svelte";
  import ErrorCard from "../lib/ErrorCard.svelte";
  import LiftVolumeBars from "../lib/LiftVolumeBars.svelte";
  import MuscleVolumeBars from "../lib/MuscleVolumeBars.svelte";
  import { formatLongDate } from "../lib/date";
  import { muscleGroupLabel } from "../lib/library";
  import { formatVolume } from "../lib/volume";
  import {
    getLifter,
    getLifterRacked,
    type LifterProfile,
    type RackedReport,
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
  let loading = $state(true);
  let failed = $state(false);
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
    loading = false;
  }

  const name = $derived(profile?.displayName || profile?.username || "");
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
  <div class="flex flex-col gap-4">
    <Card class="h-28 animate-pulse" aria-hidden="true"></Card>
    <Card class="h-40 animate-pulse" aria-hidden="true"></Card>
  </div>
{:else}
  <div class="flex flex-col gap-4">
    <!-- Who -->
    <Card class="flex items-center gap-4 p-6">
      <Avatar user={profile} size={56} />
      <div class="min-w-0">
        <h2 class="truncate text-2xl font-black text-foreground">{name}</h2>
        <p class="truncate text-sm text-muted-foreground">
          {profile.username}
          {#if profile.lastTrainedOn}
            · last trained {formatLongDate(profile.lastTrainedOn)}
          {:else}
            · hasn't trained yet
          {/if}
        </p>
      </div>
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
            What they trained
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
          {name} hasn't logged anything this month.
        </p>
      </Card>
    {/if}
  </div>
{/if}
