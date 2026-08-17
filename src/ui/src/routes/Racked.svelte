<script lang="ts">
  import { onMount } from "svelte";
  import { getRacked } from "../lib/api";
  import type { RackedReport } from "../lib/api";
  import { Card } from "$lib/components/ui/card";
  import ErrorCard from "../lib/ErrorCard.svelte";
  import CalendarHeatmap from "../lib/CalendarHeatmap.svelte";
  import LiftTrendChart from "../lib/LiftTrendChart.svelte";
  import LiftVolumeBars from "../lib/LiftVolumeBars.svelte";
  import RackedBars from "../lib/RackedBars.svelte";
  import { formatVolume } from "../lib/volume";
  import { formatLongDate } from "../lib/date";
  import { WEEKDAYS } from "../lib/weekday";
  import {
    formatDelta,
    formatHour,
    formatPercent,
    formatSessionLength,
    indexedSeries,
  } from "../lib/racked";

  // Racked — the month or year in review. Every figure on this page is computed
  // by the API (internal/racked) and rendered here as-is; the recap email reads
  // the same report, so the two cannot drift apart.

  type Period = "month" | "year";

  let period = $state<Period>("month");
  let report = $state<RackedReport | null>(null);
  let loading = $state(true);
  let failed = $state(false);

  async function load() {
    loading = true;
    failed = false;
    const { data, error } = await getRacked({ query: { period } });
    if (error || !data) {
      failed = true;
      loading = false;
      return;
    }
    report = data;
    loading = false;
  }

  function show(next: Period) {
    if (next === period) return;
    period = next;
    void load();
  }

  onMount(load);

  const trend = $derived(indexedSeries(report?.series ?? []));
  // One colour per lift, shared with the volume bars below the chart so a lift
  // looks the same in both. Lifts the trend chart could not draw get none.
  const colorFor = $derived(
    new Map(trend.shown.map((s) => [s.exerciseId, s.color])),
  );
  const liftRows = $derived(
    (report?.lifts ?? []).map((l) => ({
      exerciseId: l.exerciseId,
      exerciseName: l.exerciseName,
      volumeLb: l.volumeLb,
      sets: l.sets,
      share: l.share,
      color: colorFor.get(l.exerciseId) ?? null,
    })),
  );
  const volumesByDate = $derived(
    Object.fromEntries((report?.days ?? []).map((d) => [d.date, d.volumeLb])),
  );
  const heatmapSessions = $derived(
    (report?.days ?? []).map((d) => ({ performedOn: d.date, day: "" })),
  );
  // Enough columns to cover the period, plus a little air either side.
  const heatmapWeeks = $derived(period === "year" ? 53 : 6);
  const peakHour = $derived(
    (report?.hours ?? []).reduce(
      (best, count, i, all) => (count > all[best] ? i : best),
      0,
    ),
  );
  const hasSessions = $derived((report?.totals.sessions ?? 0) > 0);
</script>

<div class="flex flex-col gap-6">
  <header class="flex flex-wrap items-baseline justify-between gap-3">
    <div>
      <h2 class="text-2xl font-black text-foreground">Racked</h2>
      <p class="mt-0.5 text-sm text-muted-foreground">
        {report ? report.period.label : "Your training, in review."}
      </p>
    </div>
    <!-- A radiogroup rather than tabs: it selects which data the page shows,
         it does not switch between panels that both exist. -->
    <div class="flex gap-1 rounded-full bg-muted/40 p-1" role="radiogroup" aria-label="Period">
      {#each [{ id: "month", label: "This month" }, { id: "year", label: "This year" }] as opt (opt.id)}
        <button
          type="button"
          role="radio"
          aria-checked={period === opt.id}
          class="rounded-full px-3 py-1 text-xs font-semibold uppercase tracking-[0.15em] transition
            {period === opt.id
            ? 'bg-primary text-primary-foreground'
            : 'text-muted-foreground hover:text-foreground'}"
          onclick={() => show(opt.id as Period)}
        >
          {opt.label}
        </button>
      {/each}
    </div>
  </header>

  {#if loading}
    <Card class="h-40 animate-pulse" aria-hidden="true"></Card>
    <Card class="h-56 animate-pulse" aria-hidden="true"></Card>
  {:else if failed || !report}
    <ErrorCard message="Couldn't load your stats." onRetry={load} />
  {:else if !hasSessions}
    <Card class="p-8 text-center">
      <p class="text-sm text-muted-foreground">
        Nothing logged in {report.period.label} yet. Finish a session and this fills up.
      </p>
    </Card>
  {:else}
    <!-- The headline: tonnage, and what that much weight actually looks like. -->
    <Card class="p-6 text-center">
      <h3 class="text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground">
        Pounds lifted
      </h3>
      <p class="mt-1 text-5xl font-black tabular-nums text-primary">
        {formatVolume(report.totals.volumeLb)}
      </p>
      {#if report.comparison.count > 0}
        <p class="mt-2 text-sm text-muted-foreground">
          That's {report.comparison.count.toLocaleString("en-US")}
          {report.comparison.label}.
        </p>
      {/if}
      {#if report.change}
        <p class="mt-1 text-xs tabular-nums text-muted-foreground">
          {formatDelta(report.change.volumePct ?? 0)} vs the previous {report.period.kind}
        </p>
      {/if}
    </Card>

    {#if report.archetype.name}
      <Card class="p-5 text-center">
        <h3 class="text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground">
          Your lifter type
        </h3>
        <p class="mt-1 text-2xl font-black text-foreground">{report.archetype.name}</p>
        <p class="mt-1 text-sm text-muted-foreground">{report.archetype.description}</p>
      </Card>
    {/if}

    <section class="grid grid-cols-2 gap-3 sm:grid-cols-4">
      <Card class="p-4 text-center">
        <h3 class="text-xs uppercase tracking-[0.2em] text-muted-foreground">Sessions</h3>
        <p class="text-2xl font-black tabular-nums text-foreground">{report.totals.sessions}</p>
      </Card>
      <Card class="p-4 text-center">
        <h3 class="text-xs uppercase tracking-[0.2em] text-muted-foreground">Sets</h3>
        <p class="text-2xl font-black tabular-nums text-foreground">{report.totals.sets}</p>
      </Card>
      <Card class="p-4 text-center">
        <h3 class="text-xs uppercase tracking-[0.2em] text-muted-foreground">Reps</h3>
        <p class="text-2xl font-black tabular-nums text-foreground">{report.totals.reps}</p>
      </Card>
      <Card class="p-4 text-center">
        <h3 class="text-xs uppercase tracking-[0.2em] text-muted-foreground">Week streak</h3>
        <p class="text-2xl font-black tabular-nums text-foreground">
          {report.streak.longestWeeks}
        </p>
      </Card>
    </section>

    <!-- Hero moments. Each is nullable server-side and simply absent when the
         period has no answer — an unfinished session has no duration, a lift
         performed once has no trend. -->
    <section class="grid gap-4 sm:grid-cols-3">
      {#if report.mostImproved}
        <Card class="p-4" data-testid="stat-most-improved">
          <h3 class="text-xs uppercase tracking-[0.2em] text-muted-foreground">Most improved</h3>
          <p class="mt-1 font-bold text-foreground">{report.mostImproved.exerciseName}</p>
          <p class="text-2xl font-black tabular-nums text-primary">
            {formatDelta(report.mostImproved.gainPct)}
          </p>
          <p class="text-xs tabular-nums text-muted-foreground">
            {Math.round(report.mostImproved.fromLb)} → {Math.round(report.mostImproved.toLb)} lb est. max
          </p>
        </Card>
      {/if}
      {#if report.heaviestSet}
        <Card class="p-4" data-testid="stat-heaviest-set">
          <h3 class="text-xs uppercase tracking-[0.2em] text-muted-foreground">Heaviest set</h3>
          <p class="mt-1 font-bold text-foreground">{report.heaviestSet.exerciseName}</p>
          <p class="text-2xl font-black tabular-nums text-foreground">
            {formatVolume(report.heaviestSet.weightLb)} lb × {report.heaviestSet.reps}
          </p>
          <p class="text-xs text-muted-foreground">
            {formatLongDate(report.heaviestSet.performedOn)}
          </p>
        </Card>
      {/if}
      {#if report.fastestSession}
        <Card class="p-4" data-testid="stat-fastest-session">
          <h3 class="text-xs uppercase tracking-[0.2em] text-muted-foreground">Fastest session</h3>
          <p class="mt-1 font-bold text-foreground">{report.fastestSession.programDayName}</p>
          <p class="text-2xl font-black tabular-nums text-foreground">
            {formatSessionLength(report.fastestSession.durationSeconds)}
          </p>
          <p class="text-xs text-muted-foreground">
            {formatLongDate(report.fastestSession.performedOn)}
          </p>
        </Card>
      {/if}
    </section>

    {#if trend.shown.length > 0}
      <Card class="p-4">
        <h3 class="mb-2 text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground">
          Every lift, as change from its first session
        </h3>
        <LiftTrendChart series={trend.shown} />
        {#if trend.hidden > 0}
          <!-- Said out loud rather than silently truncated: a chart that shows
               five of eight lifts without saying so reads as showing all eight. -->
          <p class="mt-2 text-xs text-muted-foreground">
            {trend.hidden} more {trend.hidden === 1 ? "lift is" : "lifts are"} not shown — the
            chart carries five at a time.
          </p>
        {/if}
      </Card>
    {/if}

    {#if liftRows.length > 0}
      <Card class="p-4">
        <h3 class="mb-3 text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground">
          Where the weight went
        </h3>
        <LiftVolumeBars rows={liftRows} />
      </Card>
    {/if}

    <Card class="p-4">
      <h3 class="mb-2 text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground">
        Every training day
      </h3>
      <CalendarHeatmap
        sessions={heatmapSessions}
        volumes={volumesByDate}
        endDate={report.period.end}
        weeks={heatmapWeeks}
      />
    </Card>

    <section class="grid gap-4 sm:grid-cols-2">
      <Card class="p-4">
        <h3 class="mb-2 text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground">
          Most productive day
        </h3>
        {#if report.bestWeekday >= 0}
          <p class="mb-2 text-lg font-black text-foreground">
            {WEEKDAYS[report.bestWeekday]}
          </p>
        {/if}
        <RackedBars
          values={report.weekdays}
          labels={WEEKDAYS.map((d) => d.slice(0, 1))}
          highlight={report.bestWeekday}
          format={(v) => `${formatVolume(v)} lb`}
          caption="Volume lifted by day of the week"
        />
      </Card>

      <Card class="p-4">
        <h3 class="mb-2 text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground">
          When you train
        </h3>
        {#if report.hourLabel}
          <p class="mb-2 text-lg font-black text-foreground">{report.hourLabel}</p>
        {/if}
        <RackedBars
          values={report.hours}
          labels={report.hours.map((_, h) => formatHour(h))}
          highlight={peakHour}
          labelEvery={6}
          format={(v) => `${v} session${v === 1 ? "" : "s"}`}
          caption="Sessions started, by hour of the day"
        />
      </Card>
    </section>

    {#if report.attendance.basis !== "none"}
      <Card class="p-4">
        <h3 class="text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground">
          Attendance
        </h3>
        <p class="mt-1 text-2xl font-black tabular-nums text-foreground">
          {formatPercent(report.attendance.rate)}
        </p>
        <p class="text-xs text-muted-foreground">
          {report.attendance.actual} of {report.attendance.expected} sessions
          {#if report.attendance.basis === "cadence"}
            <!-- Honest about the denominator: with no weekday set on the program
                 there is no schedule to measure against, only an estimate. -->
            · estimated from your program's shape, since its days carry no weekdays
          {/if}
        </p>
      </Card>
    {/if}

    {#if report.prs.length > 0}
      <Card class="p-4">
        <h3 class="mb-2 text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground">
          {report.prs.length} personal record{report.prs.length === 1 ? "" : "s"}
        </h3>
        <ul class="flex flex-col gap-1.5">
          <!-- Keyed by position. Nothing derived from a date is unique here: two
               sessions of one lift in a day can each set a weight record, which
               repeats date + lift + kind exactly. The server sends these in the
               order they should read, and nothing reorders them, so the index is
               both correct and the only thing that cannot collide. -->
          {#each report.prs as pr, i (i)}
            <li class="flex items-baseline justify-between gap-2 text-sm">
              <span class="truncate text-foreground">{pr.exerciseName}</span>
              <span class="shrink-0 tabular-nums text-muted-foreground">
                {formatVolume(pr.weightLb)} lb × {pr.reps}
                {#if pr.kind === "e1rm"}<span class="text-xs"> · est. max</span>{/if}
              </span>
            </li>
          {/each}
        </ul>
      </Card>
    {/if}

    {#if report.milestones.length > 0}
      <Card class="p-4">
        <h3 class="mb-2 text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground">
          Milestones
        </h3>
        <ul class="flex flex-col gap-1.5">
          <!-- Position again, for the same reason as the two lists either side.
               A threshold is only crossed once, so this key happens to be safe
               today — but it is safe by accident of the milestone logic, and one
               rule across all three lists is easier to keep right than two. -->
          {#each report.milestones as m, i (i)}
            <li class="flex items-baseline justify-between gap-2 text-sm">
              <span class="truncate text-foreground">{m.label}</span>
              <span class="shrink-0 text-xs text-muted-foreground">
                {formatLongDate(m.performedOn)}
              </span>
            </li>
          {/each}
        </ul>
      </Card>
    {/if}

    {#if report.deloads.length > 0}
      <Card class="p-4">
        <h3 class="mb-2 text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground">
          Stalls and comebacks
        </h3>
        <ul class="flex flex-col gap-1.5">
          <!-- Position: one lift can stall twice in a day, repeating date + lift. -->
          {#each report.deloads as d, i (i)}
            <li class="flex items-baseline justify-between gap-2 text-sm">
              <span class="truncate text-foreground">
                {d.exerciseName}
                <span class="tabular-nums text-muted-foreground">
                  {Math.round(d.fromLb)} → {Math.round(d.toLb)} lb
                </span>
              </span>
              <span class="shrink-0 text-xs {d.recovered ? 'text-primary' : 'text-muted-foreground'}">
                {d.recovered ? "won it back" : "still climbing"}
              </span>
            </li>
          {/each}
        </ul>
      </Card>
    {/if}
  {/if}
</div>
